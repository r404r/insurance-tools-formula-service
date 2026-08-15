package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

func TestComputeDiffIncludesNodeDescriptionChanges(t *testing.T) {
	from := &domain.FormulaVersion{
		Version: 1,
		Graph: domain.FormulaGraph{Nodes: []domain.FormulaNode{{
			ID: "premium", Type: domain.NodeConstant, Config: json.RawMessage(`{"value":"100"}`), Description: "old description",
		}}},
	}
	to := &domain.FormulaVersion{
		Version: 2,
		Graph: domain.FormulaGraph{Nodes: []domain.FormulaNode{{
			ID: "premium", Type: domain.NodeConstant, Config: json.RawMessage(`{"value":"100"}`), Description: "new description",
		}}},
	}

	diff := computeDiff(from, to)
	if len(diff.ModifiedNodes) != 1 {
		t.Fatalf("modifiedNodes = %d, want 1 for a description-only change", len(diff.ModifiedNodes))
	}
}

func TestComputeDiffIncludesGraphOutputChanges(t *testing.T) {
	from := &domain.FormulaVersion{Version: 1, Graph: domain.FormulaGraph{Outputs: []string{"old-output"}}}
	to := &domain.FormulaVersion{Version: 2, Graph: domain.FormulaGraph{Outputs: []string{"new-output"}}}

	payload, err := json.Marshal(computeDiff(from, to))
	if err != nil {
		t.Fatalf("marshal diff: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode diff: %v", err)
	}
	for _, field := range []string{"addedOutputs", "removedOutputs"} {
		if _, ok := got[field]; !ok {
			t.Errorf("diff JSON missing %q: %s", field, payload)
		}
	}
}

type recordingVersionRepo struct {
	*inMemoryVersionRepo
	updates []struct {
		version int
		state   domain.VersionState
	}
}

func (r *recordingVersionRepo) UpdateState(ctx context.Context, formulaID string, version int, state domain.VersionState) error {
	r.updates = append(r.updates, struct {
		version int
		state   domain.VersionState
	}{version: version, state: state})
	return r.inMemoryVersionRepo.UpdateState(ctx, formulaID, version, state)
}

func TestVersionHandlerPublishDelegatesAtomicTransitionToRepository(t *testing.T) {
	formulas := newInMemoryFormulaRepo()
	versions := &recordingVersionRepo{inMemoryVersionRepo: newInMemoryVersionRepo()}
	formulaID := "formula-1"
	_ = formulas.Create(context.Background(), &domain.Formula{ID: formulaID})
	_ = versions.CreateVersion(context.Background(), &domain.FormulaVersion{
		ID: "v1", FormulaID: formulaID, Version: 1, State: domain.StatePublished, Graph: validVersionTestGraph(),
	})
	_ = versions.CreateVersion(context.Background(), &domain.FormulaVersion{
		ID: "v2", FormulaID: formulaID, Version: 2, State: domain.StateDraft, Graph: validVersionTestGraph(),
	})

	body, _ := json.Marshal(UpdateVersionStateRequest{State: domain.StatePublished})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/formulas/"+formulaID+"/versions/2", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", formulaID)
	rctx.URLParams.Add("ver", "2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	(&VersionHandler{Versions: versions, Formulas: formulas}).UpdateState(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(versions.updates) != 1 || versions.updates[0].version != 2 || versions.updates[0].state != domain.StatePublished {
		t.Fatalf("UpdateState calls = %+v, want one atomic publish call for version 2", versions.updates)
	}
}

func TestVersionCreateRejectsNonExecutableGraph(t *testing.T) {
	h, _, _, formulaID := newForkTestHandler(t)
	rr, _ := doCreate(t, h, formulaID, CreateVersionRequest{Graph: domain.FormulaGraph{}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-executable graph; body=%s", rr.Code, rr.Body.String())
	}
}

func TestVersionPublishRejectsPersistedNonExecutableGraph(t *testing.T) {
	formulas := newInMemoryFormulaRepo()
	versions := newInMemoryVersionRepo()
	formulaID := "invalid-formula"
	_ = formulas.Create(context.Background(), &domain.Formula{ID: formulaID})
	_ = versions.CreateVersion(context.Background(), &domain.FormulaVersion{ID: "invalid-v1", FormulaID: formulaID, Version: 1, State: domain.StateDraft, Graph: domain.FormulaGraph{}})
	body, _ := json.Marshal(UpdateVersionStateRequest{State: domain.StatePublished})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/formulas/"+formulaID+"/versions/1", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", formulaID)
	rctx.URLParams.Add("ver", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	(&VersionHandler{Versions: versions, Formulas: formulas}).UpdateState(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for publishing non-executable graph; body=%s", rr.Code, rr.Body.String())
	}
}
