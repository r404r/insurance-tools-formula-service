package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestCategoryDeleteIsRejectedByDatabaseWhenFormulaReferencesSlug(t *testing.T) {
	ctx := context.Background()
	s, err := New("file:category-integrity?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	category := &domain.Category{
		ID: "category-1", Slug: "life", Name: "Life", Color: "#6366f1", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Categories().Create(ctx, category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := s.Users().Create(ctx, &domain.User{
		ID: "user-1", Username: "alice", Password: "x", Role: domain.RoleEditor, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.Formulas().Create(ctx, &domain.Formula{
		ID: "formula-1", Name: "Formula", Domain: "life", CreatedBy: "user-1", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create formula: %v", err)
	}

	err = s.Categories().Delete(ctx, category.ID)
	if !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("delete category error = %v, want ErrHasContent", err)
	}
	if _, err := s.Categories().GetByID(ctx, category.ID); err != nil {
		t.Fatalf("category was deleted despite live formula reference: %v", err)
	}
}

func TestMigrateAddsCategoryIntegrityToLegacySchema(t *testing.T) {
	ctx := context.Background()
	s, err := New("file:legacy-category-integrity?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	legacy := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL, role TEXT NOT NULL, token_version INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE categories (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', color TEXT NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE formulas (id TEXT PRIMARY KEY, name TEXT NOT NULL, domain TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL REFERENCES users(id), updated_by TEXT REFERENCES users(id), created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE lookup_tables (id TEXT PRIMARY KEY, name TEXT NOT NULL, domain TEXT NOT NULL, table_type TEXT NOT NULL, data_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, stmt := range legacy {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	now := time.Now().UTC()
	if err := s.Categories().Create(ctx, &domain.Category{ID: "c", Slug: "life", Name: "Life", Color: "#000", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := s.Users().Create(ctx, &domain.User{ID: "u", Username: "u", Password: "x", Role: domain.RoleEditor, CreatedAt: now}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.Formulas().Create(ctx, &domain.Formula{ID: "f", Name: "F", Domain: "life", CreatedBy: "u", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create formula: %v", err)
	}
	if err := s.Categories().Delete(ctx, "c"); !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("legacy category delete error = %v, want ErrHasContent", err)
	}
}
