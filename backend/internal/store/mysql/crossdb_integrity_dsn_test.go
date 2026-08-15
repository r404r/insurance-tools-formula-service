package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// TestCrossDatabaseIntegrityWithDSN is intentionally opt-in so normal unit
// tests need no service. Set MYSQL_TEST_DSN to an isolated disposable DB.
func TestCrossDatabaseIntegrityWithDSN(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN is not set")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	exerciseCrossDatabaseIntegrity(t, s)
}

// TestLegacyCategoryIntegrityMigrationWithDSN starts from the pre-category-FK
// shape. Use a separate empty disposable database via MYSQL_LEGACY_TEST_DSN.
func TestLegacyCategoryIntegrityMigrationWithDSN(t *testing.T) {
	dsn := os.Getenv("MYSQL_LEGACY_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_LEGACY_TEST_DSN is not set")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE TABLE users (id VARCHAR(36) PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, password TEXT NOT NULL, role VARCHAR(50) NOT NULL, created_at VARCHAR(35) NOT NULL)`,
		`CREATE TABLE categories (id VARCHAR(36) PRIMARY KEY, slug VARCHAR(255) NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT NOT NULL, color VARCHAR(50) NOT NULL, sort_order INT NOT NULL DEFAULT 0, created_at VARCHAR(35) NOT NULL, updated_at VARCHAR(35) NOT NULL)`,
		`CREATE TABLE formulas (id VARCHAR(36) PRIMARY KEY, name TEXT NOT NULL, domain VARCHAR(255) NOT NULL, description TEXT NOT NULL, created_by VARCHAR(36) NOT NULL, created_at VARCHAR(35) NOT NULL, updated_at VARCHAR(35) NOT NULL, FOREIGN KEY (created_by) REFERENCES users(id))`,
		`CREATE TABLE lookup_tables (id VARCHAR(36) PRIMARY KEY, name TEXT NOT NULL, domain VARCHAR(255) NOT NULL, table_type VARCHAR(100) NOT NULL, data_json MEDIUMTEXT NOT NULL, created_at VARCHAR(35) NOT NULL)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	legacyNow := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		"legacy-orphan-user", "legacy-orphan-user", "x", "editor", legacyNow.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("create legacy orphan user: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO formulas (id, name, domain, description, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"legacy-orphan-formula", "Orphan formula", "legacy-orphan-domain", "", "legacy-orphan-user", legacyNow.Format(time.RFC3339Nano), legacyNow.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("create legacy orphan formula: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO lookup_tables (id, name, domain, table_type, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"legacy-orphan-table", "Orphan table", "legacy-orphan-domain", "rating", `[]`, legacyNow.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("create legacy orphan table: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate legacy schema with orphan domains: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("repeat legacy migration: %v", err)
	}
	if _, err := s.Categories().GetBySlug(ctx, "legacy-orphan-domain"); err != nil {
		t.Fatalf("migration did not create category for orphan domain: %v", err)
	}
	if _, err := s.Formulas().GetByID(ctx, "legacy-orphan-formula"); err != nil {
		t.Fatalf("migration did not preserve orphan formula: %v", err)
	}
	if _, err := s.Tables().GetByID(ctx, "legacy-orphan-table"); err != nil {
		t.Fatalf("migration did not preserve orphan lookup table: %v", err)
	}
	now := time.Now().UTC()
	if err := s.Categories().Create(ctx, &domain.Category{ID: "legacy-category", Slug: "legacy-life", Name: "Life", Color: "#000", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create migrated category: %v", err)
	}
	if err := s.Users().Create(ctx, &domain.User{ID: "legacy-user", Username: "legacy-user", Password: "x", Role: domain.RoleEditor, CreatedAt: now}); err != nil {
		t.Fatalf("create migrated user: %v", err)
	}
	if err := s.Formulas().Create(ctx, &domain.Formula{ID: "legacy-formula", Name: "Formula", Domain: "legacy-life", CreatedBy: "legacy-user", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create migrated formula: %v", err)
	}
	if err := s.Categories().Delete(ctx, "legacy-category"); !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("delete migrated referenced category = %v, want ErrHasContent", err)
	}
}

// TestResetSeedDataRejectsReferencedSeedTableWithDSN exercises the same
// atomic reset contract against a real, disposable MySQL database.
func TestResetSeedDataRejectsReferencedSeedTableWithDSN(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN is not set")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	prefix := "seed-reset-" + uuid.NewString()[:8] + "-"
	now := time.Now().UTC()
	category := &domain.Category{ID: prefix + "category", Slug: prefix + "life", Name: "Life", Color: "#000", CreatedAt: now, UpdatedAt: now}
	if err := s.Categories().Create(ctx, category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	user := &domain.User{ID: prefix + "user", Username: prefix + "user", Password: "x", Role: domain.RoleEditor, CreatedAt: now}
	if err := s.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	table := &domain.LookupTable{ID: prefix + "table", Name: "Seed table", Domain: domain.InsuranceDomain(category.Slug), TableType: "rating", Data: []byte(`[]`), SeedKey: "fixture", CreatedAt: now, UpdatedAt: now}
	if err := s.Tables().Create(ctx, table); err != nil {
		t.Fatalf("create seed table: %v", err)
	}
	seedFormula := &domain.Formula{ID: prefix + "seed-formula", Name: "Seed formula", Domain: domain.InsuranceDomain(category.Slug), SeedKey: "fixture", CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := s.Formulas().Create(ctx, seedFormula); err != nil {
		t.Fatalf("create seed formula: %v", err)
	}
	if err := s.Versions().CreateVersion(ctx, &domain.FormulaVersion{ID: prefix + "seed-version", FormulaID: seedFormula.ID, Version: 1, State: domain.StateDraft, Graph: domain.FormulaGraph{Nodes: []domain.FormulaNode{{ID: "lookup", Type: domain.NodeTableLookup, Config: []byte(`{"tableId":"` + table.ID + `","keyColumns":["key"],"column":"value"}`)}}, Outputs: []string{"lookup"}}, CreatedBy: user.ID, CreatedAt: now}); err != nil {
		t.Fatalf("create seed formula version: %v", err)
	}
	consumer := &domain.Formula{ID: prefix + "consumer", Name: "Consumer", Domain: domain.InsuranceDomain(category.Slug), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := s.Formulas().Create(ctx, consumer); err != nil {
		t.Fatalf("create consumer formula: %v", err)
	}
	if err := s.Versions().CreateVersion(ctx, &domain.FormulaVersion{ID: prefix + "consumer-version", FormulaID: consumer.ID, Version: 1, State: domain.StateDraft, Graph: domain.FormulaGraph{Nodes: []domain.FormulaNode{{ID: "lookup", Type: domain.NodeTableLookup, Config: []byte(`{"tableId":"` + table.ID + `","keyColumns":["key"],"column":"value"}`)}}, Outputs: []string{"lookup"}}, CreatedBy: user.ID, CreatedAt: now}); err != nil {
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
	if _, err := s.Tables().GetByID(ctx, table.ID); err != nil {
		t.Fatalf("referenced seed table was removed despite rejected reset: %v", err)
	}
	if err := s.Formulas().Delete(ctx, consumer.ID); err != nil {
		t.Fatalf("delete external consumer before retrying reset: %v", err)
	}
	formulasDeleted, tablesDeleted, err = s.ResetSeedData(ctx)
	if err != nil {
		t.Fatalf("ResetSeedData after external reference removal = %v, want success", err)
	}
	if formulasDeleted != 1 || tablesDeleted != 1 {
		t.Fatalf("ResetSeedData retry counts = formulas:%d tables:%d, want 1/1", formulasDeleted, tablesDeleted)
	}
	if _, err := s.Formulas().GetByID(ctx, seedFormula.ID); err == nil {
		t.Fatal("seed formula remains after unblocked reset")
	}
	if _, err := s.Tables().GetByID(ctx, table.ID); err == nil {
		t.Fatal("seed table remains after unblocked reset")
	}
}

func exerciseCrossDatabaseIntegrity(t *testing.T, s *MySQLStore) {
	t.Helper()
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("idempotent migration: %v", err)
	}

	prefix := "c004-" + uuid.NewString()[:8] + "-"
	now := time.Now().UTC()
	category := &domain.Category{ID: prefix + "category", Slug: prefix + "life", Name: "Life", Color: "#000", CreatedAt: now, UpdatedAt: now}
	if err := s.Categories().Create(ctx, category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	for i := 0; i < 2; i++ {
		u := &domain.User{ID: fmt.Sprintf("%sadmin-%d", prefix, i), Username: fmt.Sprintf("%sadmin-%d", prefix, i), Password: "x", Role: domain.RoleAdmin, CreatedAt: now}
		if err := s.Users().Create(ctx, u); err != nil {
			t.Fatalf("create admin %d: %v", i, err)
		}
	}
	if err := s.Formulas().Create(ctx, &domain.Formula{ID: prefix + "formula", Name: "Formula", Domain: domain.InsuranceDomain(category.Slug), CreatedBy: prefix + "admin-0", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create formula: %v", err)
	}
	if err := s.Categories().Delete(ctx, category.ID); !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("delete referenced category = %v, want ErrHasContent", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("%sadmin-%d", prefix, i)
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- s.Users().UpdateRole(ctx, id, domain.RoleEditor) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, guarded int
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, storepkg.ErrLastAdmin) {
			guarded++
			continue
		}
		t.Fatalf("concurrent demotion: %v", err)
	}
	if successes != 1 || guarded != 1 {
		t.Fatalf("demotions: successes=%d guarded=%d, want 1/1", successes, guarded)
	}

	for i := 0; i < 2; i++ {
		u := &domain.User{ID: fmt.Sprintf("%sdelete-%d", prefix, i), Username: fmt.Sprintf("%sdelete-%d", prefix, i), Password: "x", Role: domain.RoleAdmin, CreatedAt: now}
		if err := s.Users().Create(ctx, u); err != nil {
			t.Fatalf("create delete admin %d: %v", i, err)
		}
	}
	users, err := s.Users().List(ctx)
	if err != nil {
		t.Fatalf("list users after demotions: %v", err)
	}
	for _, user := range users {
		if user.Role == domain.RoleAdmin && strings.HasPrefix(user.ID, prefix) && !strings.Contains(user.ID, "delete-") {
			if err := s.Users().UpdateRole(ctx, user.ID, domain.RoleEditor); err != nil {
				t.Fatalf("demote remaining scoped admin: %v", err)
			}
		}
	}
	start = make(chan struct{})
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("%sdelete-%d", prefix, i)
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- s.Users().Delete(ctx, id) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, guarded = 0, 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, storepkg.ErrLastAdmin) {
			guarded++
			continue
		}
		t.Fatalf("concurrent delete: %v", err)
	}
	if successes != 1 || guarded != 1 {
		t.Fatalf("deletes: successes=%d guarded=%d, want 1/1", successes, guarded)
	}
}
