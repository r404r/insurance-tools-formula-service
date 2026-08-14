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
