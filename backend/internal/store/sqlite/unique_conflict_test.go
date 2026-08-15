package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestUniqueCreatesReturnConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("username", func(t *testing.T) {
		first := &domain.User{ID: "u1", Username: "alice", Password: "x", Role: domain.RoleViewer, CreatedAt: now}
		second := &domain.User{ID: "u2", Username: "alice", Password: "x", Role: domain.RoleViewer, CreatedAt: now}
		if err := s.Users().Create(ctx, first); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if err := s.Users().Create(ctx, second); !errors.Is(err, storepkg.ErrConflict) {
			t.Fatalf("duplicate username error = %v, want ErrConflict", err)
		}
	})

	t.Run("category slug", func(t *testing.T) {
		first := &domain.Category{ID: "c1", Slug: "casualty", Name: "Casualty", Color: "#000000", CreatedAt: now, UpdatedAt: now}
		second := &domain.Category{ID: "c2", Slug: "casualty", Name: "Other", Color: "#000000", CreatedAt: now, UpdatedAt: now}
		if err := s.Categories().Create(ctx, first); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if err := s.Categories().Create(ctx, second); !errors.Is(err, storepkg.ErrConflict) {
			t.Fatalf("duplicate slug error = %v, want ErrConflict", err)
		}
	})
}
