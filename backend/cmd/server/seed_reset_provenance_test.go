package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store/sqlite"
	"github.com/r404r/insurance-tools/formula-service/backend/seed"
)

// TestSeedResetPreservesUserDataWithASeedName is the regression contract for
// reset-seed ownership. A user is allowed to choose the same display name as
// a bundled formula or lookup table. Reset must therefore use persisted seed
// provenance rather than inferring ownership from the mutable display name.
func TestSeedResetPreservesUserDataWithASeedName(t *testing.T) {
	formulaNames, tableNames, err := seed.Names()
	if err != nil {
		t.Fatalf("load seed names: %v", err)
	}

	ctx := context.Background()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	user := &domain.User{
		ID:        "user-created-data-owner",
		Username:  "owner",
		Password:  "not-used-by-this-test",
		Role:      domain.RoleEditor,
		CreatedAt: now,
	}
	if err := s.Users().Create(ctx, user); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := s.Categories().Create(ctx, &domain.Category{
		ID: "user-category", Slug: "user-category", Name: "User Category", Color: "#000", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create user category: %v", err)
	}

	formulaID := "user-formula-with-seed-name"
	if err := s.Formulas().Create(ctx, &domain.Formula{
		ID:        formulaID,
		Name:      firstSeedName(t, formulaNames),
		Domain:    "user-category",
		CreatedBy: user.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create user formula: %v", err)
	}

	tableID := "user-table-with-seed-name"
	if err := s.Tables().Create(ctx, &domain.LookupTable{
		ID:        tableID,
		Name:      firstSeedName(t, tableNames),
		Domain:    "user-category",
		TableType: "user-data",
		Data:      json.RawMessage(`[]`),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create user table: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reset-seed", nil)
	rr := httptest.NewRecorder()
	makeSeedResetHandler(s, zerolog.Nop())(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want %d", rr.Code, http.StatusOK)
	}

	if _, err := s.Formulas().GetByID(ctx, formulaID); err != nil {
		t.Fatalf("reset deleted a user formula merely because its name matches a seed: %v", err)
	}
	if _, err := s.Tables().GetByID(ctx, tableID); err != nil {
		t.Fatalf("reset deleted a user table merely because its name matches a seed: %v", err)
	}
}

func TestSeedResetDeletesOnlyPersistedSeedOwnedData(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC()
	if err := s.Categories().Create(ctx, &domain.Category{ID: "category", Slug: "seed-domain", Name: "Seed", Color: "#000", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := s.Users().Create(ctx, &domain.User{ID: "seed-owner", Username: "seed", Password: "x", Role: domain.RoleAdmin, CreatedAt: now}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	formula := &domain.Formula{ID: "seed-formula", Name: "Renamed Seed Formula", Domain: "seed-domain", CreatedBy: "seed-owner", CreatedAt: now, UpdatedAt: now, SeedKey: "bundled-v1/formula/001"}
	if err := s.Formulas().Create(ctx, formula); err != nil {
		t.Fatalf("create seed formula: %v", err)
	}
	table := &domain.LookupTable{ID: "seed-table", Name: "Renamed Seed Table", Domain: "seed-domain", TableType: "seed", Data: json.RawMessage(`[]`), CreatedAt: now, UpdatedAt: now, SeedKey: "bundled-v1/table/001"}
	if err := s.Tables().Create(ctx, table); err != nil {
		t.Fatalf("create seed table: %v", err)
	}

	rr := httptest.NewRecorder()
	makeSeedResetHandler(s, zerolog.Nop())(rr, httptest.NewRequest(http.MethodPost, "/api/v1/admin/reset-seed", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if _, err := s.Formulas().GetByID(ctx, formula.ID); err == nil {
		t.Fatal("seed-owned formula remains after reset")
	}
	if _, err := s.Tables().GetByID(ctx, table.ID); err == nil {
		t.Fatal("seed-owned table remains after reset")
	}
}

// TestSeedResetDoesNotReportSuccessWhenPersistenceFails guards the reset
// command's operational contract: an incomplete deletion must not look like a
// successful reset to an operator or automation invoking the endpoint.
func TestSeedResetDoesNotReportSuccessWhenPersistenceFails(t *testing.T) {
	formulaNames, _, err := seed.Names()
	if err != nil {
		t.Fatalf("load seed names: %v", err)
	}

	tests := []struct {
		name     string
		formulas *resetFormulaRepo
	}{
		{
			name:     "formula listing fails",
			formulas: &resetFormulaRepo{listErr: errors.New("formula database unavailable")},
		},
		{
			name: "formula delete fails",
			formulas: &resetFormulaRepo{
				formulas:  []*domain.Formula{{ID: "seed-formula", Name: firstSeedName(t, formulaNames), SeedKey: "bundled-v1/formula"}},
				deleteErr: errors.New("formula delete denied"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := resetStore{formulas: tt.formulas, tables: &resetTableRepo{}}
			rr := httptest.NewRecorder()
			makeSeedResetHandler(s, zerolog.Nop())(rr, httptest.NewRequest(http.MethodPost, "/api/v1/admin/reset-seed", nil))
			if rr.Code < http.StatusInternalServerError {
				t.Fatalf("reset status = %d, want a 5xx failure rather than successful partial reset", rr.Code)
			}
		})
	}
}

func firstSeedName(t *testing.T, names map[string]bool) string {
	t.Helper()
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	if len(keys) == 0 {
		t.Fatal("embedded seed set is unexpectedly empty")
	}
	sort.Strings(keys)
	return keys[0]
}

// resetStore embeds the untouched Store surface and supplies only the two
// repositories used by makeSeedResetHandler. The embedded interface is never
// called in these tests.
type resetStore struct {
	store.Store
	formulas store.FormulaRepository
	tables   store.TableRepository
}

func (s resetStore) Formulas() store.FormulaRepository { return s.formulas }
func (s resetStore) Tables() store.TableRepository     { return s.tables }

type resetFormulaRepo struct {
	formulas  []*domain.Formula
	listErr   error
	deleteErr error
}

func (r *resetFormulaRepo) Create(context.Context, *domain.Formula) error { return nil }
func (r *resetFormulaRepo) GetByID(context.Context, string) (*domain.Formula, error) {
	return nil, errors.New("not implemented")
}
func (r *resetFormulaRepo) List(context.Context, domain.FormulaFilter) ([]*domain.Formula, int, error) {
	return r.formulas, len(r.formulas), r.listErr
}
func (r *resetFormulaRepo) Update(context.Context, *domain.Formula) error { return nil }
func (r *resetFormulaRepo) Delete(context.Context, string) error          { return r.deleteErr }
func (r *resetFormulaRepo) UpdateMeta(context.Context, string, string, time.Time) error {
	return nil
}

type resetTableRepo struct{}

func (*resetTableRepo) Create(context.Context, *domain.LookupTable) error { return nil }
func (*resetTableRepo) GetByID(context.Context, string) (*domain.LookupTable, error) {
	return nil, errors.New("not implemented")
}
func (*resetTableRepo) List(context.Context, *domain.InsuranceDomain) ([]*domain.LookupTable, error) {
	return nil, nil
}
func (*resetTableRepo) Update(context.Context, *domain.LookupTable) error { return nil }
func (*resetTableRepo) Delete(context.Context, string) error              { return nil }
