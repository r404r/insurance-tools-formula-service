package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// StoreTableResolver resolves lookup table data from the database store.
//
// It caches the parsed rows (the result of json.Unmarshal on table.Data) per
// tableID so that repeated ResolveTable calls against the same table within a
// batch run (which otherwise parse the same JSON 100+ times) reuse a single
// decoded copy. The composite-key map that ResolveTable returns is still
// built fresh on every call, so different (keyColumns, column) combinations
// against the same table share the cached rows but get independent maps.
//
// Invalidation is explicit:
//
//   - `InvalidateAll()` drops every cached table and bumps the generation
//   - `Invalidate(tableID)` drops a single table and bumps the generation
//
// Both are wired through Engine.ClearCache(), which the table HTTP handler
// already calls on Update/Delete. No UpdatedAt / schema migration is needed.
//
// Thread safety:
//   - Reads take an RLock; the cache map is built under Lock.
//   - Concurrent cache misses for the same tableID collapse through
//     singleflight. DoChan is used so that each waiter observes its own
//     context cancellation independently — one canceled client cannot
//     fan-out a failure to other waiters sharing the same flight.
//   - The loader runs with context.Background() so it is not tied to any
//     single caller's deadline. If every waiter cancels, the flight still
//     runs to completion (one wasted DB query, at most).
//   - A monotonic generation counter guards against the "invalidation
//     during in-flight load" race: a loader only writes its result into
//     the cache if the generation it captured at start still matches the
//     current generation. If Invalidate happened while the load was
//     running, the in-flight result is returned to that caller (honoring
//     their request) but is NOT written back, so subsequent calls re-fetch
//     and see the fresh data.
//   - Cached rows are shared by reference and MUST be treated as read-only.
//     The only in-tree caller (engine.preloadTableData) iterates without
//     mutating, which is safe; do not introduce callers that modify the
//     returned slice without first switching to a deep-copy strategy.
type StoreTableResolver struct {
	Tables store.TableRepository

	mu    sync.RWMutex
	cache map[string][]map[string]string
	gen   uint64 // bumped on every invalidation; guarded by mu
	sf    singleflight.Group
}

// ResolveTable loads a lookup table by ID and returns a map of compositeKey -> column value.
// keyColumns specifies which columns form the composite key (joined with "|").
// The table data is expected to be a JSON array of string-map objects.
func (r *StoreTableResolver) ResolveTable(ctx context.Context, tableID string, keyColumns []string, column string) (map[string]string, error) {
	rows, err := r.getRows(ctx, tableID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		// Build composite key from all keyColumns values.
		parts := make([]string, 0, len(keyColumns))
		skip := false
		for _, kc := range keyColumns {
			v, ok := row[kc]
			if !ok {
				skip = true
				break
			}
			parts = append(parts, v)
		}
		if skip {
			continue
		}
		val, ok := row[column]
		if !ok {
			continue
		}
		result[strings.Join(parts, "|")] = val
	}

	return result, nil
}

// getRows returns the cached parsed rows for a table, loading and parsing
// them from the store on cache miss.
//
// Concurrent misses for the same tableID collapse through
// singleflight.DoChan. Each caller waits on a select over its own ctx and
// the shared flight channel, so a single canceled caller does not fan out
// its error to other waiters.
//
// The loader runs under context.Background() rather than the first caller's
// ctx — the flight is shared, so inheriting any particular caller's
// cancellation would defeat the purpose of the detachment above.
func (r *StoreTableResolver) getRows(ctx context.Context, tableID string) ([]map[string]string, error) {
	// Fast path: cache hit under RLock.
	r.mu.RLock()
	if rows, ok := r.cache[tableID]; ok {
		r.mu.RUnlock()
		return rows, nil
	}
	r.mu.RUnlock()

	// Slow path: cache miss. Launch or join a shared flight.
	ch := r.sf.DoChan(tableID, func() (interface{}, error) {
		// Snapshot the cache generation under the same lock that serves
		// a re-check of the cache, so that an invalidation racing with
		// this loader either happens entirely before (we see the newer
		// gen via the re-check) or entirely after (we see the older gen
		// and fail the write-back guard below).
		r.mu.RLock()
		if rows, ok := r.cache[tableID]; ok {
			// Another flight just populated the cache between the fast
			// path above and here. Reuse it and skip the DB load.
			r.mu.RUnlock()
			return rows, nil
		}
		startGen := r.gen
		r.mu.RUnlock()

		// Detach from any caller's context: the flight is shared, so
		// inheriting one caller's deadline/cancellation would poison the
		// rest. The store's own timeouts (if any) still apply.
		table, err := r.Tables.GetByID(context.Background(), tableID)
		if err != nil {
			return nil, fmt.Errorf("table %s not found: %w", tableID, err)
		}
		var rows []map[string]string
		if err := json.Unmarshal(table.Data, &rows); err != nil {
			return nil, fmt.Errorf("table %s: invalid data format: %w", tableID, err)
		}

		// Write-back guard: only publish to the cache if no invalidation
		// happened while we were loading. Otherwise return the rows to
		// our caller (we did the work, they asked for it) but leave the
		// cache empty so the next call re-fetches fresh data.
		r.mu.Lock()
		if r.gen == startGen {
			if r.cache == nil {
				r.cache = make(map[string][]map[string]string)
			}
			r.cache[tableID] = rows
		}
		r.mu.Unlock()
		return rows, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.([]map[string]string), nil
	case <-ctx.Done():
		// Our ctx was canceled while waiting. The flight keeps running
		// for any other waiters — we just bail out ourselves.
		return nil, ctx.Err()
	}
}

// GetRows returns the parsed rows of a table, loaded through the same
// singleflight + cache + invalidation-generation path that powers
// ResolveTable. This is the public counterpart of the internal getRows
// method, exposed so that NodeTableAggregate (task #040) can scan a
// whole table at evaluation time without paying for a redundant
// JSON unmarshal.
//
// The returned slice is shared by reference with the cache and MUST be
// treated as read-only by the caller. The single in-tree consumer
// (engine.evalTableAggregate) only iterates rows to evaluate filters
// and pull a column value, which is safe.
func (r *StoreTableResolver) GetRows(ctx context.Context, tableID string) ([]map[string]string, error) {
	return r.getRows(ctx, tableID)
}

// InvalidateAll drops every cached table and bumps the generation so that
// any in-flight loader's write-back is rejected. Called by Engine.ClearCache().
func (r *StoreTableResolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = nil
	r.gen++
	r.mu.Unlock()
}

// Invalidate drops the cache entry for a single table and bumps the
// generation so that any in-flight loader for this table has its write-back
// rejected. Exposed for callers that know exactly which table changed and
// want to avoid a full flush.
func (r *StoreTableResolver) Invalidate(tableID string) {
	r.mu.Lock()
	delete(r.cache, tableID)
	r.gen++
	r.mu.Unlock()
}
