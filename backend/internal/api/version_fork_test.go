package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// inMemoryFormulaRepo and inMemoryVersionRepo are minimal stand-ins for
// the production stores. They keep all data in maps so the test does not
// touch SQLite. We only implement the methods the version handler uses.
type inMemoryFormulaRepo struct {
	formulas map[string]*domain.Formula
}

func newInMemoryFormulaRepo() *inMemoryFormulaRepo {
	return &inMemoryFormulaRepo{formulas: map[string]*domain.Formula{}}
}

func (r *inMemoryFormulaRepo) Create(_ context.Context, f *domain.Formula) error {
	r.formulas[f.ID] = f
	return nil
}
func (r *inMemoryFormulaRepo) GetByID(_ context.Context, id string) (*domain.Formula, error) {
	f, ok := r.formulas[id]
	if !ok {
		return nil, store.ErrHasContent
	}
	return f, nil
}
func (r *inMemoryFormulaRepo) List(_ context.Context, _ domain.FormulaFilter) ([]*domain.Formula, int, error) {
	return nil, 0, nil
}
func (r *inMemoryFormulaRepo) Update(_ context.Context, f *domain.Formula) error {
	r.formulas[f.ID] = f
	return nil
}
func (r *inMemoryFormulaRepo) Delete(_ context.Context, id string) error {
	delete(r.formulas, id)
	return nil
}
func (r *inMemoryFormulaRepo) UpdateMeta(_ context.Context, formulaID, updatedBy string, updatedAt time.Time) error {
	f, ok := r.formulas[formulaID]
	if !ok {
		return store.ErrHasContent
	}
	f.UpdatedBy = updatedBy
	f.UpdatedAt = updatedAt
	return nil
}

type inMemoryVersionRepo struct {
	// keyed by "formulaID:version"
	versions map[string]*domain.FormulaVersion
}

func newInMemoryVersionRepo() *inMemoryVersionRepo {
	return &inMemoryVersionRepo{versions: map[string]*domain.FormulaVersion{}}
}

func versionKey(formulaID string, ver int) string {
	return formulaID + ":" + itoa(ver)
}
func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + itoa(n%10)
}

func (r *inMemoryVersionRepo) CreateVersion(_ context.Context, v *domain.FormulaVersion) error {
	r.versions[versionKey(v.FormulaID, v.Version)] = v
	return nil
}
func (r *inMemoryVersionRepo) GetVersion(_ context.Context, formulaID string, version int) (*domain.FormulaVersion, error) {
	v, ok := r.versions[versionKey(formulaID, version)]
	if !ok {
		return nil, store.ErrHasContent
	}
	return v, nil
}
func (r *inMemoryVersionRepo) GetPublished(_ context.Context, formulaID string) (*domain.FormulaVersion, error) {
	for _, v := range r.versions {
		if v.FormulaID == formulaID && v.State == domain.StatePublished {
			return v, nil
		}
	}
	return nil, store.ErrHasContent
}
func (r *inMemoryVersionRepo) ListVersions(_ context.Context, formulaID string) ([]*domain.FormulaVersion, error) {
	var out []*domain.FormulaVersion
	for _, v := range r.versions {
		if v.FormulaID == formulaID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *inMemoryVersionRepo) UpdateState(_ context.Context, formulaID string, version int, state domain.VersionState) error {
	v, ok := r.versions[versionKey(formulaID, version)]
	if !ok {
		return store.ErrHasContent
	}
	v.State = state
	return nil
}

// newForkTestHandler builds a handler with two seed versions:
//
//   v1 = published (a stable but old shape)
//   v2 = archived  (the version we want to fork from)
//
// The handler is fully wired so an HTTP POST goes through the same
// path as production: GetByID → ListVersions → BaseVersion check →
// CreateVersion → UpdateMeta.
func newForkTestHandler(t *testing.T) (*VersionHandler, *inMemoryFormulaRepo, *inMemoryVersionRepo, string) {
	t.Helper()
	formulas := newInMemoryFormulaRepo()
	versions := newInMemoryVersionRepo()

	formulaID := uuid.New().String()
	now := time.Now().UTC()

	if err := formulas.Create(context.Background(), &domain.Formula{
		ID:        formulaID,
		Name:      "Test",
		CreatedBy: "u-creator",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed formula: %v", err)
	}
	if err := versions.CreateVersion(context.Background(), &domain.FormulaVersion{
		ID:         uuid.New().String(),
		FormulaID:  formulaID,
		Version:    1,
		State:      domain.StatePublished,
		Graph:      domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote: "v1",
		CreatedBy:  "u-creator",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if err := versions.CreateVersion(context.Background(), &domain.FormulaVersion{
		ID:         uuid.New().String(),
		FormulaID:  formulaID,
		Version:    2,
		State:      domain.StateArchived,
		Graph:      domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote: "v2 archived",
		CreatedBy:  "u-creator",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("seed v2: %v", err)
	}

	return &VersionHandler{Versions: versions, Formulas: formulas}, formulas, versions, formulaID
}

// doCreate is a small helper that fires a POST through the chi router
// so chi.URLParam works inside the handler.
func doCreate(t *testing.T, h *VersionHandler, formulaID string, body CreateVersionRequest) (*httptest.ResponseRecorder, *domain.FormulaVersion) {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/formulas/"+formulaID+"/versions", bytes.NewReader(bodyBytes))

	// Inject claims so the handler thinks an authenticated user is calling.
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: "u-forker"})

	// Wire chi route so chi.URLParam("id") returns the formulaID.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", formulaID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code == http.StatusCreated {
		var v domain.FormulaVersion
		if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
			t.Fatalf("decode created version: %v", err)
		}
		return rr, &v
	}
	return rr, nil
}

func TestVersionCreate_DefaultParentIsLatest(t *testing.T) {
	// No baseVersion: parent should be the previous max (v2 here).
	h, _, versions, formulaID := newForkTestHandler(t)
	rr, created := doCreate(t, h, formulaID, CreateVersionRequest{
		Graph:      domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote: "default save",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}
	if created.Version != 3 {
		t.Errorf("created.Version = %d, want 3", created.Version)
	}
	if created.ParentVer == nil || *created.ParentVer != 2 {
		t.Errorf("ParentVer = %v, want 2 (default fork from latest)", created.ParentVer)
	}
	// Original archived v2 must still exist and be unchanged.
	v2, err := versions.GetVersion(context.Background(), formulaID, 2)
	if err != nil {
		t.Fatalf("v2 missing: %v", err)
	}
	if v2.State != domain.StateArchived {
		t.Errorf("v2 state = %s, want archived", v2.State)
	}
}

func TestVersionCreate_ForkFromArchived(t *testing.T) {
	// baseVersion=2 (archived): parent must be 2, the original archived
	// version must remain archived and untouched.
	h, _, versions, formulaID := newForkTestHandler(t)
	base := 2
	rr, created := doCreate(t, h, formulaID, CreateVersionRequest{
		Graph:       domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote:  "fork from v2",
		BaseVersion: &base,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}
	if created.Version != 3 {
		t.Errorf("created.Version = %d, want 3", created.Version)
	}
	if created.ParentVer == nil || *created.ParentVer != 2 {
		t.Errorf("ParentVer = %v, want 2 (forked from archived v2)", created.ParentVer)
	}
	if created.State != domain.StateDraft {
		t.Errorf("forked version state = %s, want draft", created.State)
	}
	// v2 must still exist as archived.
	v2, err := versions.GetVersion(context.Background(), formulaID, 2)
	if err != nil {
		t.Fatalf("v2 missing: %v", err)
	}
	if v2.State != domain.StateArchived {
		t.Errorf("v2 state corrupted: got %s, want archived", v2.State)
	}
}

func TestVersionCreate_ForkFromPublished(t *testing.T) {
	// baseVersion=1 (published): parent must be 1.
	h, _, _, formulaID := newForkTestHandler(t)
	base := 1
	rr, created := doCreate(t, h, formulaID, CreateVersionRequest{
		Graph:       domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote:  "fork from v1",
		BaseVersion: &base,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}
	if created.ParentVer == nil || *created.ParentVer != 1 {
		t.Errorf("ParentVer = %v, want 1", created.ParentVer)
	}
	if created.Version != 3 {
		t.Errorf("created.Version = %d, want 3 (next of max=2)", created.Version)
	}
}

func TestVersionCreate_ForkFromMissingBaseReturns404(t *testing.T) {
	h, _, _, formulaID := newForkTestHandler(t)
	base := 99
	rr, _ := doCreate(t, h, formulaID, CreateVersionRequest{
		Graph:       domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote:  "fork from nothing",
		BaseVersion: &base,
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rr.Code)
	}
}

func TestVersionCreate_ForkStampsUpdaterMeta(t *testing.T) {
	// After a fork-from-archived, the formula's updated_by should be
	// the user who did the fork (u-forker), not the original creator.
	h, formulas, _, formulaID := newForkTestHandler(t)
	base := 2
	rr, _ := doCreate(t, h, formulaID, CreateVersionRequest{
		Graph:       domain.FormulaGraph{Outputs: []string{"out"}},
		ChangeNote:  "fork stamps updater",
		BaseVersion: &base,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}
	f, err := formulas.GetByID(context.Background(), formulaID)
	if err != nil {
		t.Fatalf("get formula: %v", err)
	}
	if f.UpdatedBy != "u-forker" {
		t.Errorf("UpdatedBy = %q, want %q", f.UpdatedBy, "u-forker")
	}
}
