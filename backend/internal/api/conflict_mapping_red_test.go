package api

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// These tests simulate the window between a handler's preflight read and the
// database write. The repository is authoritative for uniqueness and can
// report a conflict even though the preceding read found no row.
func TestWriteConflictsMapToConflictOrNotFoundResponses(t *testing.T) {
	t.Run("register unique conflict is 409", func(t *testing.T) {
		h := &AuthHandler{
			Users:  &writeConflictUserRepo{},
			JWTMgr: auth.NewJWTManager("conflict-mapping-test-secret-at-least-32", time.Hour),
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"alice","password":"password123"}`))
		rr := httptest.NewRecorder()

		h.Register(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("register status = %d, want %d when unique create races", rr.Code, http.StatusConflict)
		}
	})

	t.Run("category unique conflict is 409", func(t *testing.T) {
		h := &CategoryHandler{Categories: &writeConflictCategoryRepo{}}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBufferString(`{"slug":"life","name":"Life"}`))
		rr := httptest.NewRecorder()

		h.Create(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("category create status = %d, want %d when unique create races", rr.Code, http.StatusConflict)
		}
	})

	t.Run("formula delete raced by another request is 404", func(t *testing.T) {
		h := &FormulaHandler{Formulas: &alreadyDeletedFormulaRepo{}}
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/formulas/formula-1", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "formula-1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()

		h.Delete(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("formula delete status = %d, want %d when another request removed it first", rr.Code, http.StatusNotFound)
		}
	})
}

type writeConflictUserRepo struct{}

func (*writeConflictUserRepo) Create(context.Context, *domain.User) error { return store.ErrConflict }
func (*writeConflictUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return nil, sql.ErrNoRows
}
func (*writeConflictUserRepo) GetByUsername(context.Context, string) (*domain.User, error) {
	return nil, sql.ErrNoRows
}
func (*writeConflictUserRepo) GetTokenVersion(context.Context, string) (int, error) {
	return 0, sql.ErrNoRows
}
func (*writeConflictUserRepo) List(context.Context) ([]*domain.User, error)          { return nil, nil }
func (*writeConflictUserRepo) UpdateRole(context.Context, string, domain.Role) error { return nil }
func (*writeConflictUserRepo) Delete(context.Context, string) error                  { return nil }

type writeConflictCategoryRepo struct{}

func (*writeConflictCategoryRepo) Create(context.Context, *domain.Category) error {
	return store.ErrConflict
}
func (*writeConflictCategoryRepo) GetByID(context.Context, string) (*domain.Category, error) {
	return nil, sql.ErrNoRows
}
func (*writeConflictCategoryRepo) GetBySlug(context.Context, string) (*domain.Category, error) {
	return nil, sql.ErrNoRows
}
func (*writeConflictCategoryRepo) List(context.Context) ([]*domain.Category, error) { return nil, nil }
func (*writeConflictCategoryRepo) Update(context.Context, *domain.Category) error   { return nil }
func (*writeConflictCategoryRepo) Delete(context.Context, string) error             { return nil }

type alreadyDeletedFormulaRepo struct{}

func (*alreadyDeletedFormulaRepo) Create(context.Context, *domain.Formula) error { return nil }
func (*alreadyDeletedFormulaRepo) GetByID(context.Context, string) (*domain.Formula, error) {
	return &domain.Formula{ID: "formula-1"}, nil
}
func (*alreadyDeletedFormulaRepo) List(context.Context, domain.FormulaFilter) ([]*domain.Formula, int, error) {
	return nil, 0, nil
}
func (*alreadyDeletedFormulaRepo) Update(context.Context, *domain.Formula) error { return nil }
func (*alreadyDeletedFormulaRepo) Delete(context.Context, string) error          { return sql.ErrNoRows }
func (*alreadyDeletedFormulaRepo) UpdateMeta(context.Context, string, string, time.Time) error {
	return nil
}
