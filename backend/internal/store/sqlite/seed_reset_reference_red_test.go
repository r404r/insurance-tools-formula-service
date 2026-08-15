package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestResetSeedDataRejectsSeedTableReferencedByAnyFormulaVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedUser(t, s, "seed-reset-user", "seed-reset-user")

	seedTable := &domain.LookupTable{
		ID: "seed-reset-table", Name: "Seed table", Domain: "life", TableType: "rating",
		Data: []byte(`[{"key":"A","value":"1"}]`), SeedKey: "table-fixture",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Tables().Create(ctx, seedTable); err != nil {
		t.Fatalf("create seed table: %v", err)
	}
	seedFormula := &domain.Formula{
		ID: "seed-reset-formula", Name: "Seed formula", Domain: "life", SeedKey: "formula-fixture",
		CreatedBy: "seed-reset-user", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Formulas().Create(ctx, seedFormula); err != nil {
		t.Fatalf("create seed formula: %v", err)
	}
	if err := s.Versions().CreateVersion(ctx, &domain.FormulaVersion{
		ID: "seed-reset-version", FormulaID: seedFormula.ID, Version: 1, State: domain.StateDraft,
		Graph: domain.FormulaGraph{Outputs: []string{"out"}}, CreatedBy: "seed-reset-user", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create seed version: %v", err)
	}

	consumer := &domain.Formula{
		ID: "seed-reset-consumer", Name: "Consumer", Domain: "life",
		CreatedBy: "seed-reset-user", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Formulas().Create(ctx, consumer); err != nil {
		t.Fatalf("create consumer formula: %v", err)
	}
	if err := s.Versions().CreateVersion(ctx, &domain.FormulaVersion{
		ID: "seed-reset-consumer-version", FormulaID: consumer.ID, Version: 1, State: domain.StateDraft,
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{{
				ID: "lookup", Type: domain.NodeTableLookup,
				Config: []byte(`{"tableId":"seed-reset-table","keyColumns":["key"],"column":"value"}`),
			}},
			Outputs: []string{"lookup"},
		},
		CreatedBy: "seed-reset-user", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create consumer version: %v", err)
	}

	formulasDeleted, tablesDeleted, err := s.ResetSeedData(ctx)
	if !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("ResetSeedData error = %v, want ErrHasContent", err)
	}
	if formulasDeleted != 0 || tablesDeleted != 0 {
		t.Fatalf("ResetSeedData counts = formulas:%d tables:%d, want 0/0 after atomic rejection", formulasDeleted, tablesDeleted)
	}
	if _, err := s.Formulas().GetByID(ctx, seedFormula.ID); err != nil {
		t.Fatalf("seed formula was removed despite rejected reset: %v", err)
	}
	if _, err := s.Versions().GetVersion(ctx, seedFormula.ID, 1); err != nil {
		t.Fatalf("seed formula version was removed despite rejected reset: %v", err)
	}
	if _, err := s.Tables().GetByID(ctx, seedTable.ID); err != nil {
		t.Fatalf("referenced seed table was removed despite rejected reset: %v", err)
	}
}
