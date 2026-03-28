package engine

import (
	"context"
	"fmt"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// FormulaResolver resolves formula versions for sub-formula execution.
type FormulaResolver interface {
	ResolveFormula(ctx context.Context, formulaID string, version *int) (*domain.FormulaVersion, error)
}

// StoreFormulaResolver resolves formula versions from the configured store.
type StoreFormulaResolver struct {
	Versions store.VersionRepository
}

// ResolveFormula returns either a specific version or the published version.
func (r *StoreFormulaResolver) ResolveFormula(ctx context.Context, formulaID string, version *int) (*domain.FormulaVersion, error) {
	if version != nil {
		resolved, err := r.Versions.GetVersion(ctx, formulaID, *version)
		if err != nil {
			return nil, fmt.Errorf("formula %s version %d not found: %w", formulaID, *version, err)
		}
		return resolved, nil
	}

	resolved, err := r.Versions.GetPublished(ctx, formulaID)
	if err != nil {
		return nil, fmt.Errorf("published formula %s not found: %w", formulaID, err)
	}
	return resolved, nil
}
