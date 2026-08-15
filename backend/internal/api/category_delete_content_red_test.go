package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// The preflight formula/table reads can race a new reference. Repository
// deletion is authoritative, so its ErrHasContent result must be represented
// as the same 409 "category is still in use" contract as preflight usage.
func TestCategoryDeleteMapsRepositoryHasContentToConflict(t *testing.T) {
	categories := &hasContentOnCategoryDeleteRepo{}
	h := &CategoryHandler{
		Categories: categories,
		Formulas:   newInMemoryFormulaRepo(),
		Tables:     &emptyCategoryTableRepo{},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/categories/life-category", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "life-category")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("category delete status = %d, want %d when repository reports category still in use; body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if categories.deleteCalls != 1 {
		t.Errorf("category repository Delete calls = %d, want 1", categories.deleteCalls)
	}
}

type hasContentOnCategoryDeleteRepo struct {
	deleteCalls int
}

func (*hasContentOnCategoryDeleteRepo) Create(context.Context, *domain.Category) error { return nil }
func (*hasContentOnCategoryDeleteRepo) GetByID(context.Context, string) (*domain.Category, error) {
	return &domain.Category{ID: "life-category", Slug: "life", Name: "Life"}, nil
}
func (*hasContentOnCategoryDeleteRepo) GetBySlug(context.Context, string) (*domain.Category, error) {
	return nil, nil
}
func (*hasContentOnCategoryDeleteRepo) List(context.Context) ([]*domain.Category, error) {
	return nil, nil
}
func (*hasContentOnCategoryDeleteRepo) Update(context.Context, *domain.Category) error { return nil }
func (r *hasContentOnCategoryDeleteRepo) Delete(context.Context, string) error {
	r.deleteCalls++
	return store.ErrHasContent
}

type emptyCategoryTableRepo struct{}

func (*emptyCategoryTableRepo) Create(context.Context, *domain.LookupTable) error { return nil }
func (*emptyCategoryTableRepo) GetByID(context.Context, string) (*domain.LookupTable, error) {
	return nil, nil
}
func (*emptyCategoryTableRepo) List(context.Context, *domain.InsuranceDomain) ([]*domain.LookupTable, error) {
	return []*domain.LookupTable{}, nil
}
func (*emptyCategoryTableRepo) Update(context.Context, *domain.LookupTable) error { return nil }
func (*emptyCategoryTableRepo) Delete(context.Context, string) error              { return nil }
