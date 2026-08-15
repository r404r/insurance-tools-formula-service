package postgres

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
// tests need no service. Set POSTGRES_TEST_DSN to an isolated disposable DB.
func TestCrossDatabaseIntegrityWithDSN(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	exerciseCrossDatabaseIntegrity(t, s)
}

// TestLegacyCategoryIntegrityMigrationWithDSN starts from the pre-category-FK
// shape. Use a separate empty disposable database via POSTGRES_LEGACY_TEST_DSN.
func TestLegacyCategoryIntegrityMigrationWithDSN(t *testing.T) {
	dsn := os.Getenv("POSTGRES_LEGACY_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_LEGACY_TEST_DSN is not set")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL, role TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE categories (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', color TEXT NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE formulas (id TEXT PRIMARY KEY, name TEXT NOT NULL, domain TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE lookup_tables (id TEXT PRIMARY KEY, name TEXT NOT NULL, domain TEXT NOT NULL, table_type TEXT NOT NULL, data_json TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("repeat legacy migration: %v", err)
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

func exerciseCrossDatabaseIntegrity(t *testing.T, s *PostgresStore) {
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
