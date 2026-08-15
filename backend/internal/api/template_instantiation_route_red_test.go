package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// TestTemplateInstantiationIsOneBackendWorkflow specifies the replacement for
// the gallery's client-side create-formula → create-version sequence. A
// successful request must expose both records together; a bad template must
// expose neither. The production endpoint is deliberately absent at Red.
func TestTemplateInstantiationIsOneBackendWorkflow(t *testing.T) {
	formulas := newInMemoryFormulaRepo()
	versions := newInMemoryVersionRepo()
	jwtMgr := auth.NewJWTManager("template-instantiation-test-secret-32b", time.Hour)
	user := &domain.User{ID: "template-user", Username: "editor", Role: domain.RoleEditor}
	token, err := jwtMgr.Generate(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := NewRouter(RouterConfig{
		AuthHandler: &AuthHandler{Users: &templateUserRepo{}, JWTMgr: jwtMgr, CookieSecure: false},
		FormulaHandler: &FormulaHandler{
			Formulas:   formulas,
			Versions:   versions,
			Categories: &templateCategoryRepo{},
		},
		VersionHandler:  &VersionHandler{Formulas: formulas, Versions: versions},
		TemplateHandler: &TemplateHandler{},
		JWTManager:      jwtMgr,
		UserStore:       &templateUserRepo{},
		Logger:          zerolog.Nop(),
		CORSOrigins:     []string{"http://localhost:5173"},
		CalcLimiter:     NewDynamicConcurrencyLimiter(1),
	})

	body, err := json.Marshal(map[string]string{
		"name":        "Atomic template formula",
		"domain":      "life",
		"description": "created from a template in one transaction",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-life-term-risk/instantiate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("instantiate status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if len(formulas.formulas) != 1 {
		t.Fatalf("formula count = %d, want 1 after successful instantiation", len(formulas.formulas))
	}
	if len(versions.versions) != 1 {
		t.Fatalf("version count = %d, want 1; template creation must not leave a formula orphan", len(versions.versions))
	}
}

func TestTemplateInstantiationRollsBackFormulaWhenInitialVersionFails(t *testing.T) {
	formulas := newInMemoryFormulaRepo()
	versions := &failingTemplateVersionRepo{inMemoryVersionRepo: newInMemoryVersionRepo()}
	jwtMgr := auth.NewJWTManager("template-instantiation-test-secret-32b", time.Hour)
	token, err := jwtMgr.Generate(&domain.User{ID: "template-user", Username: "editor", Role: domain.RoleEditor})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	router := NewRouter(RouterConfig{
		AuthHandler:     &AuthHandler{Users: &templateUserRepo{}, JWTMgr: jwtMgr, CookieSecure: false},
		FormulaHandler:  &FormulaHandler{Formulas: formulas, Versions: versions, Categories: &templateCategoryRepo{}},
		VersionHandler:  &VersionHandler{Formulas: formulas, Versions: versions},
		TemplateHandler: &TemplateHandler{}, JWTManager: jwtMgr, UserStore: &templateUserRepo{},
		Logger: zerolog.Nop(), CORSOrigins: []string{"http://localhost:5173"}, CalcLimiter: NewDynamicConcurrencyLimiter(1),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-life-term-risk/instantiate", bytes.NewBufferString(`{"name":"Atomic template formula","domain":"life"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < http.StatusInternalServerError {
		t.Fatalf("instantiate status = %d, want 5xx when initial version persistence fails", rr.Code)
	}
	if len(formulas.formulas) != 0 {
		t.Fatalf("formula count = %d, want 0 after failed initial version", len(formulas.formulas))
	}
}

type failingTemplateVersionRepo struct{ *inMemoryVersionRepo }

func (*failingTemplateVersionRepo) CreateVersion(context.Context, *domain.FormulaVersion) error {
	return errors.New("injected initial version failure")
}

type templateUserRepo struct{}

func (*templateUserRepo) Create(context.Context, *domain.User) error { return nil }
func (*templateUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return nil, nil
}
func (*templateUserRepo) GetByUsername(context.Context, string) (*domain.User, error) {
	return nil, nil
}
func (*templateUserRepo) GetTokenVersion(context.Context, string) (int, error)  { return 0, nil }
func (*templateUserRepo) List(context.Context) ([]*domain.User, error)          { return nil, nil }
func (*templateUserRepo) UpdateRole(context.Context, string, domain.Role) error { return nil }
func (*templateUserRepo) Delete(context.Context, string) error                  { return nil }

type templateCategoryRepo struct{}

func (*templateCategoryRepo) Create(context.Context, *domain.Category) error { return nil }
func (*templateCategoryRepo) GetByID(context.Context, string) (*domain.Category, error) {
	return nil, nil
}
func (*templateCategoryRepo) GetBySlug(_ context.Context, slug string) (*domain.Category, error) {
	return &domain.Category{ID: "life-category", Slug: slug, Name: "Life"}, nil
}
func (*templateCategoryRepo) List(context.Context) ([]*domain.Category, error) { return nil, nil }
func (*templateCategoryRepo) Update(context.Context, *domain.Category) error   { return nil }
func (*templateCategoryRepo) Delete(context.Context, string) error             { return nil }
