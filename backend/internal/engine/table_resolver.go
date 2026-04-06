package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// StoreTableResolver resolves lookup table data from the database store.
type StoreTableResolver struct {
	Tables store.TableRepository
}

// ResolveTable loads a lookup table by ID and returns a map of compositeKey -> column value.
// keyColumns specifies which columns form the composite key (joined with "|").
// The table data is expected to be a JSON array of string-map objects.
func (r *StoreTableResolver) ResolveTable(ctx context.Context, tableID string, keyColumns []string, column string) (map[string]string, error) {
	table, err := r.Tables.GetByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("table %s not found: %w", tableID, err)
	}

	var rows []map[string]string
	if err := json.Unmarshal(table.Data, &rows); err != nil {
		return nil, fmt.Errorf("table %s: invalid data format: %w", tableID, err)
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
