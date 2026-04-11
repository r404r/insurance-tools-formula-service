package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// fakeTableRepo is a minimal store.TableRepository used to exercise the
// resolver's caching logic without pulling in a real SQLite/Postgres backend.
// Only GetByID is meaningful here; the other methods satisfy the interface.
//
// An optional `block` channel lets tests pause a GetByID call mid-flight,
// which is how we exercise the "invalidate during in-flight load" race.
type fakeTableRepo struct {
	mu    sync.Mutex
	data  map[string]*domain.LookupTable
	calls int32 // incremented on every GetByID call
	block chan struct{}
}

func newFakeTableRepo() *fakeTableRepo {
	return &fakeTableRepo{data: map[string]*domain.LookupTable{}}
}

func (r *fakeTableRepo) put(id string, rows []map[string]string) {
	b, _ := json.Marshal(rows)
	r.mu.Lock()
	r.data[id] = &domain.LookupTable{ID: id, Data: b}
	r.mu.Unlock()
}

func (r *fakeTableRepo) GetByID(_ context.Context, id string) (*domain.LookupTable, error) {
	atomic.AddInt32(&r.calls, 1)
	// Snapshot the row BEFORE the optional block, so tests can simulate a
	// "slow DB" where the query has already read the old data but the
	// network is slow. Any concurrent updates to repo.data after this
	// snapshot are invisible to this call, mirroring a real SELECT that
	// already committed its read set.
	r.mu.Lock()
	t, ok := r.data[id]
	if !ok {
		r.mu.Unlock()
		return nil, errors.New("not found")
	}
	cp := *t
	r.mu.Unlock()

	if r.block != nil {
		<-r.block
	}
	return &cp, nil
}

// Remaining TableRepository methods are unused by the resolver; stubs satisfy
// the interface at compile time without exercising any real behaviour.
func (r *fakeTableRepo) Create(_ context.Context, _ *domain.LookupTable) error { return nil }
func (r *fakeTableRepo) List(_ context.Context, _ *domain.InsuranceDomain) ([]*domain.LookupTable, error) {
	return nil, nil
}
func (r *fakeTableRepo) Update(_ context.Context, _ *domain.LookupTable) error { return nil }
func (r *fakeTableRepo) Delete(_ context.Context, _ string) error              { return nil }

func TestStoreTableResolver_CachesParsedRows(t *testing.T) {
	repo := newFakeTableRepo()
	repo.put("qx", []map[string]string{
		{"age": "1", "qx": "0.0001", "mx": "0.00005"},
		{"age": "2", "qx": "0.0002", "mx": "0.00010"},
	})

	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	// First call: cold cache, should hit repo once.
	got, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx")
	if err != nil {
		t.Fatalf("first ResolveTable: %v", err)
	}
	if got["1"] != "0.0001" || got["2"] != "0.0002" {
		t.Fatalf("unexpected rows for qx: %v", got)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 1 {
		t.Fatalf("GetByID calls after first Resolve: got %d, want 1", n)
	}

	// Second call on same (tableID, keyColumns, column): should be served
	// from cache — no additional DB hits.
	got2, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx")
	if err != nil {
		t.Fatalf("second ResolveTable: %v", err)
	}
	if got2["1"] != "0.0001" {
		t.Fatalf("unexpected cached rows: %v", got2)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 1 {
		t.Fatalf("GetByID calls after second Resolve: got %d, want 1 (cache miss?)", n)
	}

	// Third call with a DIFFERENT column on the same table should also be
	// served from cache — raw rows are shared across columns.
	got3, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "mx")
	if err != nil {
		t.Fatalf("third ResolveTable: %v", err)
	}
	if got3["1"] != "0.00005" {
		t.Fatalf("unexpected cached-column rows: %v", got3)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 1 {
		t.Fatalf("GetByID calls after cross-column lookup: got %d, want 1", n)
	}
}

func TestStoreTableResolver_InvalidateAllForcesReload(t *testing.T) {
	repo := newFakeTableRepo()
	repo.put("t", []map[string]string{{"k": "1", "v": "10"}})
	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	if _, err := resolver.ResolveTable(ctx, "t", []string{"k"}, "v"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 1 {
		t.Fatalf("calls after warm: %d, want 1", n)
	}

	// Mutate the underlying table and force a reload via InvalidateAll.
	repo.put("t", []map[string]string{{"k": "1", "v": "999"}})
	resolver.InvalidateAll()

	got, err := resolver.ResolveTable(ctx, "t", []string{"k"}, "v")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got["1"] != "999" {
		t.Fatalf("expected reloaded value 999, got %q", got["1"])
	}
	if n := atomic.LoadInt32(&repo.calls); n != 2 {
		t.Fatalf("calls after invalidate+reload: %d, want 2", n)
	}
}

func TestStoreTableResolver_InvalidateOneLeavesOthers(t *testing.T) {
	repo := newFakeTableRepo()
	repo.put("a", []map[string]string{{"k": "1", "v": "A"}})
	repo.put("b", []map[string]string{{"k": "1", "v": "B"}})
	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	// Warm both.
	if _, err := resolver.ResolveTable(ctx, "a", []string{"k"}, "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveTable(ctx, "b", []string{"k"}, "v"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 2 {
		t.Fatalf("warm calls: %d, want 2", n)
	}

	// Invalidate only "a". Subsequent lookups: a should reload, b should not.
	resolver.Invalidate("a")
	if _, err := resolver.ResolveTable(ctx, "a", []string{"k"}, "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveTable(ctx, "b", []string{"k"}, "v"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&repo.calls); n != 3 {
		t.Fatalf("after selective invalidate: %d calls, want 3 (a reloaded, b cached)", n)
	}
}

// TestStoreTableResolver_ConcurrentMissCollapses fires many goroutines at a
// cold cache for the same tableID and asserts that singleflight collapses
// them into a single DB load.
func TestStoreTableResolver_ConcurrentMissCollapses(t *testing.T) {
	repo := newFakeTableRepo()
	repo.put("qx", []map[string]string{{"age": "1", "qx": "0.0001"}})
	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if _, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx"); err != nil {
				t.Errorf("concurrent ResolveTable: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	// singleflight should collapse concurrent misses. In theory multiple
	// flights can happen if goroutines arrive in distinct waves, so allow up
	// to a small number — but definitely not 64.
	n := atomic.LoadInt32(&repo.calls)
	if n == 0 || n > 4 {
		t.Fatalf("GetByID calls under concurrent miss: %d, want 1..4", n)
	}
}

// TestStoreTableResolver_InvalidateDuringLoadDoesNotResurrectStale reproduces
// the codex-review P1 race: a load is in flight when Invalidate runs. Before
// the generation-counter guard, the loader would cheerfully write its stale
// rows into the cache after the invalidation, leaving every subsequent
// caller stuck on old data until the next manual flush.
//
// With the fix, the loader still honors its own caller (returning whatever
// it read before being unblocked), but the write-back is suppressed, so the
// NEXT call bypasses the cache and fetches the post-update rows.
func TestStoreTableResolver_InvalidateDuringLoadDoesNotResurrectStale(t *testing.T) {
	repo := newFakeTableRepo()
	repo.block = make(chan struct{})
	repo.put("qx", []map[string]string{{"age": "1", "qx": "0.0001"}})

	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	// Kick off the first Resolve. It will block inside GetByID until we
	// close repo.block.
	firstDone := make(chan error, 1)
	var firstGot map[string]string
	go func() {
		got, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx")
		firstGot = got
		firstDone <- err
	}()

	// Give the flight a moment to enter GetByID.
	waitUntil(t, func() bool {
		return atomic.LoadInt32(&repo.calls) >= 1
	}, 500*time.Millisecond, "first Resolve never reached GetByID")

	// Simulate an admin updating the table and triggering ClearCache mid-flight.
	repo.put("qx", []map[string]string{{"age": "1", "qx": "0.9999"}})
	resolver.InvalidateAll()

	// Unblock the in-flight load. It will return the OLD rows it already
	// read (before block) to the first caller, then attempt to write back
	// to the cache — which should be rejected by the generation guard.
	// Closing (not nil-ing) is sufficient: any still-racing GetByID will
	// see the closed channel and proceed immediately.
	close(repo.block)

	if err := <-firstDone; err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if firstGot["1"] != "0.0001" {
		t.Fatalf("first Resolve got %q, want the pre-invalidation rows 0.0001", firstGot["1"])
	}

	// The key assertion: subsequent Resolves must NOT see stale 0.0001. A
	// broken write-back would have re-populated the cache with old rows.
	got2, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx")
	if err != nil {
		t.Fatalf("post-invalidate Resolve: %v", err)
	}
	if got2["1"] != "0.9999" {
		t.Fatalf("post-invalidate Resolve got %q, want fresh 0.9999 — cache still holds stale data", got2["1"])
	}
	if n := atomic.LoadInt32(&repo.calls); n != 2 {
		t.Fatalf("GetByID calls after invalidate race: got %d, want exactly 2 (cache must be empty after guard fires)", n)
	}
}

// TestStoreTableResolver_CancelOneDoesNotPoisonOthers reproduces the codex-review
// P2 issue: under the original Do-based singleflight, if the first caller's
// context was canceled, every other waiter sharing the flight observed the
// same context.Canceled error. DoChan + Background loader isolates them.
func TestStoreTableResolver_CancelOneDoesNotPoisonOthers(t *testing.T) {
	repo := newFakeTableRepo()
	repo.block = make(chan struct{})
	repo.put("t", []map[string]string{{"k": "1", "v": "10"}})
	resolver := &StoreTableResolver{Tables: repo}

	// Caller A with a cancelable context that will cancel mid-flight.
	ctxA, cancelA := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var errA error
	var gotA map[string]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		gotA, errA = resolver.ResolveTable(ctxA, "t", []string{"k"}, "v")
	}()

	// Caller B with a healthy background context. Launches after A so A
	// is the flight leader and B joins as a waiter.
	waitUntil(t, func() bool {
		return atomic.LoadInt32(&repo.calls) >= 1
	}, 500*time.Millisecond, "A never started the flight")

	var errB error
	var gotB map[string]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		gotB, errB = resolver.ResolveTable(context.Background(), "t", []string{"k"}, "v")
	}()

	// Cancel A while the flight is still blocked in GetByID.
	cancelA()

	// Now unblock GetByID so the flight can complete for B.
	// Closing (not nil-ing) is sufficient: any still-racing GetByID will
	// see the closed channel and proceed immediately.
	close(repo.block)

	wg.Wait()

	// A should bail with its own ctx.Err(); B should succeed.
	if !errors.Is(errA, context.Canceled) {
		t.Fatalf("caller A error = %v, want context.Canceled", errA)
	}
	if gotA != nil {
		t.Fatalf("caller A got %v, want nil map on cancellation", gotA)
	}
	if errB != nil {
		t.Fatalf("caller B error = %v, want success (poisoned by A's cancel?)", errB)
	}
	if gotB["1"] != "10" {
		t.Fatalf("caller B got %q, want \"10\"", gotB["1"])
	}
}

// waitUntil polls `cond` until it returns true or `timeout` elapses.
func waitUntil(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waitUntil: %s", msg)
}

// TestStoreTableResolver_ConcurrentReadsAreSafe stress-tests the read path
// with concurrent Resolves (mix of hits and Invalidate calls) under -race.
func TestStoreTableResolver_ConcurrentReadsAreSafe(t *testing.T) {
	repo := newFakeTableRepo()
	repo.put("qx", []map[string]string{
		{"age": "1", "qx": "0.0001"},
		{"age": "2", "qx": "0.0002"},
		{"age": "3", "qx": "0.0003"},
	})
	resolver := &StoreTableResolver{Tables: repo}
	ctx := context.Background()

	const workers = 32
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if _, err := resolver.ResolveTable(ctx, "qx", []string{"age"}, "qx"); err != nil {
					t.Errorf("worker %d: %v", id, err)
					return
				}
				// Every 20 iterations, invalidate to exercise the write path.
				if i%20 == 0 {
					resolver.Invalidate("qx")
				}
			}
		}(w)
	}
	wg.Wait()
}
