package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/parser"
)

func graphFromFormulaText(t *testing.T, text string) domain.FormulaGraph {
	t.Helper()
	ast, err := parser.NewParser(text).Parse()
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	graph, err := parser.ASTToDAG(ast)
	if err != nil {
		t.Fatalf("serialize %q: %v", text, err)
	}
	return *graph
}

func publishTestVersion(t *testing.T, h *VersionHandler, formulaID string, version int) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(UpdateVersionStateRequest{State: domain.StatePublished})
	if err != nil {
		t.Fatalf("marshal publish request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/formulas/"+formulaID+"/versions/"+itoa(version), bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", formulaID)
	rctx.URLParams.Add("ver", itoa(version))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.UpdateState(rr, req)
	return rr
}

// The parser serializer uses the four-port legacy-conditional shape for
// textual if expressions, while a top-level comparison deliberately has only
// condition / conditionRight. Both shapes must remain persistable.
func TestVersionPersistenceAcceptsParserGeneratedConditionalAndComparisonGraphs(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		ports      int
	}{
		{
			name:       "four port if conditional",
			expression: "if age > 60 then 100 else 50",
			ports:      4,
		},
		{
			name:       "two port standalone comparison",
			expression: "age > 60",
			ports:      2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph := graphFromFormulaText(t, tc.expression)
			if got := len(graph.Edges); got != tc.ports {
				t.Fatalf("serialized %q edge count = %d, want %d", tc.expression, got, tc.ports)
			}

			h, _, _, formulaID := newForkTestHandler(t)
			createResponse, created := doCreate(t, h, formulaID, CreateVersionRequest{Graph: graph})
			if createResponse.Code != http.StatusCreated {
				t.Fatalf("create parser-generated %q status = %d, want %d; body=%s", tc.expression, createResponse.Code, http.StatusCreated, createResponse.Body.String())
			}

			publishResponse := publishTestVersion(t, h, formulaID, created.Version)
			if publishResponse.Code != http.StatusOK {
				t.Fatalf("publish parser-generated %q status = %d, want %d; body=%s", tc.expression, publishResponse.Code, http.StatusOK, publishResponse.Body.String())
			}
		})
	}
}

// Import must run the same parser plus engine validation used by version
// creation before it persists a formula shell. Parser validation alone does
// not recognize a non-empty but non-numeric constant value.
func TestFormulaImportRejectsInvalidNumericConstantBeforeWritingRecords(t *testing.T) {
	formulas := newInMemoryFormulaRepo()
	versions := newInMemoryVersionRepo()
	h := &FormulaHandler{
		Formulas:   formulas,
		Versions:   versions,
		Categories: &templateCategoryRepo{},
	}
	bundle := ExportBundle{
		Version: ExportFormat,
		Formulas: []ExportedFormula{{
			SourceID:      "source-invalid-numeric-constant",
			SourceVersion: 1,
			Name:          "Invalid numeric constant",
			Domain:        domain.InsuranceDomain("life"),
			Graph: domain.FormulaGraph{
				Nodes: []domain.FormulaNode{{
					ID:     "out",
					Type:   domain.NodeConstant,
					Config: json.RawMessage(`{"value":"not-a-number"}`),
				}},
				Outputs: []string{"out"},
			},
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
		t.Fatalf("import status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var result ImportResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if len(result.Imported) != 0 || len(result.Errors) != 1 {
		t.Fatalf("import result = %+v, want one validation error and no imports", result)
	}
	if got := len(formulas.formulas); got != 0 {
		t.Fatalf("formula count = %d, want 0 after invalid graph import", got)
	}
	if got := len(versions.versions); got != 0 {
		t.Fatalf("version count = %d, want 0 after invalid graph import", got)
	}
}
