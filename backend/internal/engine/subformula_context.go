package engine

import (
	"context"
	"fmt"
)

type subFormulaCallPathKey struct{}

func withSubFormulaCall(ctx context.Context, formulaID string, version int) (context.Context, error) {
	callKey := fmt.Sprintf("%s@%d", formulaID, version)
	path, _ := ctx.Value(subFormulaCallPathKey{}).([]string)
	for _, existing := range path {
		if existing == callKey {
			return nil, fmt.Errorf("cyclic sub-formula reference detected for %s", callKey)
		}
	}

	nextPath := append(append([]string{}, path...), callKey)
	return context.WithValue(ctx, subFormulaCallPathKey{}, nextPath), nil
}
