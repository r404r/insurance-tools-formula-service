package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// newTestStore opens an in-memory SQLite store with the schema applied.
// Each test gets its own isolated database so the order of tests does
// not matter.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

// seedUser inserts a user row directly so we can attribute formulas to
// real UUIDs and exercise the LEFT JOIN that the List query depends on.
func seedUser(t *testing.T, s *SQLiteStore, id, username string) {
	t.Helper()
	user := &domain.User{
		ID:        id,
		Username:  username,
		Password:  "x",
		Role:      domain.RoleEditor,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Users().Create(context.Background(), user); err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
}

// seedFormula inserts a formula. updatedBy may be empty (legacy row);
// the helper passes it through UpdateMeta when set.
func seedFormula(t *testing.T, s *SQLiteStore, id, name, createdBy string, createdAt time.Time, updatedBy string, updatedAt time.Time) {
	t.Helper()
	f := &domain.Formula{
		ID:        id,
		Name:      name,
		Domain:    domain.InsuranceDomain("life"),
		CreatedBy: createdBy,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if err := s.Formulas().Create(context.Background(), f); err != nil {
		t.Fatalf("seed formula %s: %v", name, err)
	}
	if updatedBy != "" {
		if err := s.Formulas().UpdateMeta(context.Background(), id, updatedBy, updatedAt); err != nil {
			t.Fatalf("update meta %s: %v", name, err)
		}
	}
}

func TestFormulaList_DefaultSortIsUpdatedAtDesc(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "alice")

	now := time.Now().UTC()
	// Insert in non-sorted order so a passing test really exercises the SQL.
	seedFormula(t, s, "f1", "Mid", "u1", now.Add(-2*time.Hour), "", now.Add(-1*time.Hour))
	seedFormula(t, s, "f2", "Newest", "u1", now.Add(-3*time.Hour), "", now)
	seedFormula(t, s, "f3", "Oldest", "u1", now.Add(-1*time.Hour), "", now.Add(-2*time.Hour))

	got, total, err := s.Formulas().List(context.Background(), domain.FormulaFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	wantOrder := []string{"Newest", "Mid", "Oldest"}
	for i, f := range got {
		if f.Name != wantOrder[i] {
			t.Errorf("position %d: got %q, want %q", i, f.Name, wantOrder[i])
		}
	}
}

func TestFormulaList_SortByName(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "alice")
	now := time.Now().UTC()
	seedFormula(t, s, "f1", "Charlie", "u1", now, "", now)
	seedFormula(t, s, "f2", "Alpha", "u1", now, "", now)
	seedFormula(t, s, "f3", "Bravo", "u1", now, "", now)

	cases := []struct {
		order string
		want  []string
	}{
		{"asc", []string{"Alpha", "Bravo", "Charlie"}},
		{"desc", []string{"Charlie", "Bravo", "Alpha"}},
	}
	for _, tc := range cases {
		t.Run(tc.order, func(t *testing.T) {
			got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{
				Limit:     10,
				SortBy:    "name",
				SortOrder: tc.order,
			})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			for i, f := range got {
				if f.Name != tc.want[i] {
					t.Errorf("position %d: got %q, want %q", i, f.Name, tc.want[i])
				}
			}
		})
	}
}

func TestFormulaList_SortByUpdaterUsername(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u-alice", "alice")
	seedUser(t, s, "u-bob", "bob")
	seedUser(t, s, "u-carol", "carol")

	now := time.Now().UTC()
	seedFormula(t, s, "f1", "Formula1", "u-alice", now, "u-carol", now)
	seedFormula(t, s, "f2", "Formula2", "u-alice", now, "u-alice", now)
	seedFormula(t, s, "f3", "Formula3", "u-alice", now, "u-bob", now)

	got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{
		Limit:     10,
		SortBy:    "updatedBy",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantUpdaterOrder := []string{"alice", "bob", "carol"}
	for i, f := range got {
		if f.UpdatedByName != wantUpdaterOrder[i] {
			t.Errorf("position %d: got %q, want %q", i, f.UpdatedByName, wantUpdaterOrder[i])
		}
	}
}

func TestFormulaList_NullUpdaterSortsLast(t *testing.T) {
	// Legacy rows where updated_by is NULL should always end up after
	// rows that have a value, regardless of sort direction. This is
	// the "NULLS LAST" emulation in the sqlite store's List query.
	s := newTestStore(t)
	seedUser(t, s, "u-alice", "alice")
	seedUser(t, s, "u-bob", "bob")

	now := time.Now().UTC()
	seedFormula(t, s, "f1", "WithUpdater1", "u-alice", now, "u-alice", now)
	seedFormula(t, s, "f2", "WithUpdater2", "u-alice", now, "u-bob", now)
	seedFormula(t, s, "f3", "NoUpdater", "u-alice", now, "", now)

	for _, order := range []string{"asc", "desc"} {
		t.Run(order, func(t *testing.T) {
			got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{
				Limit:     10,
				SortBy:    "updatedBy",
				SortOrder: order,
			})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got) != 3 {
				t.Fatalf("len(got) = %d, want 3", len(got))
			}
			// The NULL updater row must be last regardless of direction.
			if got[2].Name != "NoUpdater" {
				t.Errorf("NULL updater not at end (sort %s): got order=%q,%q,%q",
					order, got[0].Name, got[1].Name, got[2].Name)
			}
		})
	}
}

func TestFormulaList_PopulatesCreatedByName(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u-alice", "alice")
	seedUser(t, s, "u-bob", "bob")

	now := time.Now().UTC()
	seedFormula(t, s, "f1", "First", "u-alice", now, "", now)
	seedFormula(t, s, "f2", "Second", "u-bob", now, "u-alice", now)

	got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{Limit: 10, SortBy: "name", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got[0].CreatedByName != "alice" {
		t.Errorf("first.CreatedByName = %q, want alice", got[0].CreatedByName)
	}
	if got[0].UpdatedByName != "" {
		t.Errorf("first.UpdatedByName = %q, want empty", got[0].UpdatedByName)
	}
	if got[1].CreatedByName != "bob" {
		t.Errorf("second.CreatedByName = %q, want bob", got[1].CreatedByName)
	}
	if got[1].UpdatedByName != "alice" {
		t.Errorf("second.UpdatedByName = %q, want alice", got[1].UpdatedByName)
	}
}

func TestFormulaList_InvalidSortByFallsBackToDefault(t *testing.T) {
	// The handler validates sortBy upstream, but the store layer also
	// needs to behave defensively: an unknown sortBy should silently
	// fall back to updatedAt desc rather than crash or expose the SQL.
	s := newTestStore(t)
	seedUser(t, s, "u1", "alice")
	now := time.Now().UTC()
	seedFormula(t, s, "f1", "B", "u1", now, "", now.Add(-1*time.Hour))
	seedFormula(t, s, "f2", "A", "u1", now, "", now)

	got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{
		Limit:     10,
		SortBy:    "this_is_not_a_real_column; DROP TABLE formulas",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("list with bad sortBy: %v", err)
	}
	// Default fallback is updated_at desc, so "A" (newer updated_at) first.
	if got[0].Name != "A" {
		t.Errorf("got[0].Name = %q, want A (default fallback should be updatedAt desc)", got[0].Name)
	}
}

func TestFormulaList_UpdateMetaChangesListedUpdater(t *testing.T) {
	// The end-to-end story: a formula is created by alice, then bob
	// saves a new version which calls UpdateMeta. The list page must
	// then show bob as the updater.
	s := newTestStore(t)
	seedUser(t, s, "u-alice", "alice")
	seedUser(t, s, "u-bob", "bob")

	now := time.Now().UTC()
	seedFormula(t, s, "f1", "F1", "u-alice", now, "", now)

	// Initially the updater is empty (legacy / fresh formula).
	got, _, _ := s.Formulas().List(context.Background(), domain.FormulaFilter{Limit: 10})
	if got[0].UpdatedByName != "" {
		t.Fatalf("fresh formula updater = %q, want empty", got[0].UpdatedByName)
	}

	// Bob saves a version → version handler calls UpdateMeta.
	later := now.Add(time.Hour)
	if err := s.Formulas().UpdateMeta(context.Background(), "f1", "u-bob", later); err != nil {
		t.Fatalf("update meta: %v", err)
	}

	got, _, _ = s.Formulas().List(context.Background(), domain.FormulaFilter{Limit: 10})
	if got[0].UpdatedByName != "bob" {
		t.Errorf("after UpdateMeta, updater = %q, want bob", got[0].UpdatedByName)
	}
	if !got[0].UpdatedAt.Equal(later) {
		t.Errorf("after UpdateMeta, updatedAt = %v, want %v", got[0].UpdatedAt, later)
	}
}

func TestFormulaList_MigrateIsIdempotent(t *testing.T) {
	// Running Migrate a second time on a populated database must not
	// fail (the ALTER TABLE swallow-error path in Migrate is what we
	// are exercising here).
	s := newTestStore(t)
	seedUser(t, s, "u1", "alice")
	seedFormula(t, s, "f1", "F", "u1", time.Now().UTC(), "", time.Now().UTC())

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	// Verify data survived.
	got, _, err := s.Formulas().List(context.Background(), domain.FormulaFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list after second migrate: %v", err)
	}
	if len(got) != 1 || got[0].Name != "F" {
		t.Errorf("data lost after second migrate: %+v", got)
	}
}
