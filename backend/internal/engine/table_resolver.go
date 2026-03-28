package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// StoreTableResolver resolves lookup table data from the database store.
type StoreTableResolver struct {
	Tables store.TableRepository
}

// ResolveTable loads a lookup table by ID and returns a map of key -> column value.
// The table data is expected to be a JSON array of objects with string keys.
func (r *StoreTableResolver) ResolveTable(ctx context.Context, tableID string, column string) (map[string]string, error) {
	table, err := r.Tables.GetByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("table %s not found: %w", tableID, err)
	}

	// Parse the table data. Expected format:
	// [{"key": "25", "qx": "0.00123", "lx": "99877"}, ...]
	var rows []map[string]string
	if err := json.Unmarshal(table.Data, &rows); err != nil {
		return nil, fmt.Errorf("table %s: invalid data format: %w", tableID, err)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		key, ok := row["key"]
		if !ok {
			continue
		}
		val, ok := row[column]
		if !ok {
			continue
		}
		result[key] = val
	}

	return result, nil
}
