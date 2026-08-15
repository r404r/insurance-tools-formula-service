package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// atomicFormulaRepo exposes the production repository's optional atomic
// creation contract, while making any legacy Create → CreateVersion → Delete
// sequence observable. The injected failures model the unsafe sequence where
// the version write fails and the best-effort rollback fails too.
type atomicFormulaRepo struct {
	*inMemoryFormulaRepo

	atomicCalls int
	createCalls int
	deleteCalls int

	atomicErr error
	deleteErr error
}

func (r *atomicFormulaRepo) Create(_ context.Context, f *domain.Formula) error {
	r.createCalls++
	r.formulas[f.ID] = f
	return nil
}

func (r *atomicFormulaRepo) Delete(_ context.Context, _ string) error {
	r.deleteCalls++
	return r.deleteErr
}

func (r *atomicFormulaRepo) CreateWithInitialVersion(_ context.Context, _ *domain.Formula, _ *domain.FormulaVersion) error {
	r.atomicCalls++
	return r.atomicErr
}

type failingAtomicPathVersionRepo struct {
	*inMemoryVersionRepo
	createCalls int
	createErr   error
}

func (r *failingAtomicPathVersionRepo) CreateVersion(_ context.Context, _ *domain.FormulaVersion) error {
	r.createCalls++
	return r.createErr
}

// Copy must delegate both writes to FormulaVersionAtomicCreator when present.
// In particular, a failed initial-version write must never expose a formula
// shell, even if the old handler-level rollback would itself fail.
func TestFormulaCopyUsesAtomicCreatorWithoutFormulaShellOnInitialVersionFailure(t *testing.T) {
	formulas := &atomicFormulaRepo{
		inMemoryFormulaRepo: newInMemoryFormulaRepo(),
		atomicErr:           errors.New("injected atomic initial-version failure"),
		deleteErr:           errors.New("injected legacy rollback failure"),
	}
	versions := &failingAtomicPathVersionRepo{
		inMemoryVersionRepo: newInMemoryVersionRepo(),
		createErr:           errors.New("injected legacy version failure"),
	}
	const sourceID = "source-formula"
	formulas.formulas[sourceID] = &domain.Formula{ID: sourceID, Name: "Source", Domain: "life", Description: "source"}
	versions.versions[versionKey(sourceID, 1)] = &domain.FormulaVersion{
		ID: "source-version", FormulaID: sourceID, Version: 1, State: domain.StateDraft, Graph: validVersionTestGraph(),
	}
	h := &FormulaHandler{Formulas: formulas, Versions: versions, Categories: &templateCategoryRepo{}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/formulas/"+sourceID+"/copy", bytes.NewBufferString(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sourceID)
	req = req.WithContext(context.WithValue(auth.WithClaims(req.Context(), &auth.Claims{UserID: "copier"}), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Copy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("copy status = %d, want %d when atomic create fails; body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
	if formulas.atomicCalls != 1 {
		t.Errorf("CreateWithInitialVersion calls = %d, want 1", formulas.atomicCalls)
	}
	if formulas.createCalls != 0 || versions.createCalls != 0 || formulas.deleteCalls != 0 {
		t.Errorf("legacy write path used: Formula.Create=%d Version.CreateVersion=%d Formula.Delete=%d; want all zero", formulas.createCalls, versions.createCalls, formulas.deleteCalls)
	}
	if got := len(formulas.formulas); got != 1 {
		t.Errorf("formula count = %d, want only the source formula; failed copy must not leave a shell", got)
	}
}

// Import has the same two-record persistence boundary as Copy. The response
// may contain a per-item error, but no formula shell may survive an injected
// initial-version failure when the repository offers atomic creation.
func TestFormulaImportUsesAtomicCreatorWithoutFormulaShellOnInitialVersionFailure(t *testing.T) {
	formulas := &atomicFormulaRepo{
		inMemoryFormulaRepo: newInMemoryFormulaRepo(),
		atomicErr:           errors.New("injected atomic initial-version failure"),
		deleteErr:           errors.New("injected legacy rollback failure"),
	}
	versions := &failingAtomicPathVersionRepo{
		inMemoryVersionRepo: newInMemoryVersionRepo(),
		createErr:           errors.New("injected legacy version failure"),
	}
	h := &FormulaHandler{Formulas: formulas, Versions: versions, Categories: &templateCategoryRepo{}}
	bundle := ExportBundle{
		Version: ExportFormat,
		Formulas: []ExportedFormula{{
			SourceID: "source-formula", SourceVersion: 1, Name: "Imported", Domain: "life", Graph: validVersionTestGraph(),
		}},
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal import bundle: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/formulas/import", bytes.NewReader(body))
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: "importer", Role: domain.RoleEditor}))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("import status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var result ImportResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if len(result.Imported) != 0 || len(result.Errors) != 1 {
		t.Errorf("import result = %+v, want one failed item and no imported formulas", result)
	}
	if formulas.atomicCalls != 1 {
		t.Errorf("CreateWithInitialVersion calls = %d, want 1", formulas.atomicCalls)
	}
	if formulas.createCalls != 0 || versions.createCalls != 0 || formulas.deleteCalls != 0 {
		t.Errorf("legacy write path used: Formula.Create=%d Version.CreateVersion=%d Formula.Delete=%d; want all zero", formulas.createCalls, versions.createCalls, formulas.deleteCalls)
	}
	if got := len(formulas.formulas); got != 0 {
		t.Errorf("formula count = %d, want 0; failed import must not leave a shell", got)
	}
}
