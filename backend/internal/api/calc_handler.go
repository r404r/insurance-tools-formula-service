package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/engine"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// CalculationEngine defines the interface for executing formula calculations.
// This is declared in the api package to avoid circular imports with the engine
// package.
type CalculationEngine interface {
	Calculate(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]string) (*engine.CalculationResult, error)
	Validate(graph *domain.FormulaGraph) []engine.ValidationError
}

// CalcHandler implements calculation HTTP endpoints.
type CalcHandler struct {
	Engine   CalculationEngine
	Versions store.VersionRepository
	Formulas store.FormulaRepository
	Tables   store.TableRepository
}

// Calculate executes a single formula calculation.
// POST /api/v1/calculate
func (h *CalcHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	var req CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.FormulaID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "formulaId is required", Code: http.StatusBadRequest})
		return
	}

	version, err := h.resolveVersion(r.Context(), req.FormulaID, req.Version)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula version not found", Code: http.StatusNotFound})
		return
	}

	result, err := h.Engine.Calculate(r.Context(), &version.Graph, req.Inputs)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: "calculation failed: " + err.Error(), Code: http.StatusUnprocessableEntity})
		return
	}

	writeJSON(w, http.StatusOK, CalculateResponse{
		Result:          result.Outputs,
		Intermediates:   result.Intermediates,
		ExecutionTimeMs: float64(result.ExecutionTime.Microseconds()) / 1000.0,
		NodesEvaluated:  result.NodesEvaluated,
		ParallelLevels:  result.ParallelLevels,
	})
}

// BatchCalculate executes a formula calculation for each input set.
// POST /api/v1/calculate/batch
func (h *CalcHandler) BatchCalculate(w http.ResponseWriter, r *http.Request) {
	var req BatchCalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.FormulaID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "formulaId is required", Code: http.StatusBadRequest})
		return
	}
	if len(req.InputSets) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "at least one input set is required", Code: http.StatusBadRequest})
		return
	}

	version, err := h.resolveVersion(r.Context(), req.FormulaID, req.Version)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula version not found", Code: http.StatusNotFound})
		return
	}

	results := make([]CalculateResponse, 0, len(req.InputSets))
	for _, inputs := range req.InputSets {
		result, err := h.Engine.Calculate(r.Context(), &version.Graph, inputs)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
				Error: "calculation failed: " + err.Error(),
				Code:  http.StatusUnprocessableEntity,
			})
			return
		}
		results = append(results, CalculateResponse{
			Result:          result.Outputs,
			Intermediates:   result.Intermediates,
			ExecutionTimeMs: float64(result.ExecutionTime.Microseconds()) / 1000.0,
			NodesEvaluated:  result.NodesEvaluated,
			ParallelLevels:  result.ParallelLevels,
		})
	}

	writeJSON(w, http.StatusOK, BatchCalculateResponse{Results: results})
}

// Validate checks a formula graph for errors without executing it.
// POST /api/v1/calculate/validate
func (h *CalcHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var graph domain.FormulaGraph
	if err := json.NewDecoder(r.Body).Decode(&graph); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	errors := h.Engine.Validate(&graph)
	if len(errors) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":  false,
			"errors": errors,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":  true,
		"errors": []engine.ValidationError{},
	})
}

// resolveVersion retrieves the requested formula version, or the published
// version if no specific version was requested.
func (h *CalcHandler) resolveVersion(ctx context.Context, formulaID string, version *int) (*domain.FormulaVersion, error) {
	if version != nil {
		return h.Versions.GetVersion(ctx, formulaID, *version)
	}
	return h.Versions.GetPublished(ctx, formulaID)
}
