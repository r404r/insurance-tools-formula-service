package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

func TestCreateWithInitialVersionRollsBackBothRecords(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "creator", "creator")
	now := time.Now().UTC()
	seedFormula(t, s, "existing-formula", "Existing", "creator", now, "", now)
	if err := s.Versions().CreateVersion(context.Background(), &domain.FormulaVersion{
		ID: "duplicate-version-id", FormulaID: "existing-formula", Version: 1, State: domain.StateDraft,
		Graph: domain.FormulaGraph{Outputs: []string{"out"}}, CreatedBy: "creator", CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed existing version: %v", err)
	}
	formula := &domain.Formula{ID: "formula-atomic", Name: "Atomic", Domain: "life", CreatedBy: "creator", CreatedAt: now, UpdatedAt: now}
	// Deliberately reuse a globally unique version ID after the formula insert.
	// The transaction must leave neither new record visible.
	version := &domain.FormulaVersion{ID: "duplicate-version-id", FormulaID: formula.ID, Version: 1, State: domain.StateDraft, Graph: domain.FormulaGraph{Outputs: []string{"out"}}, CreatedBy: "creator", CreatedAt: now}

	if err := s.formulas.CreateWithInitialVersion(context.Background(), formula, version); err == nil {
		t.Fatal("CreateWithInitialVersion returned nil, want injected version insert failure")
	}
	if _, err := s.Formulas().GetByID(context.Background(), formula.ID); err == nil {
		t.Fatal("formula remains after initial version failure")
	}
}
