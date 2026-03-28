package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// FormulaHandler implements formula CRUD HTTP endpoints.
type FormulaHandler struct {
	Formulas store.FormulaRepository
}

// List returns formulas filtered by optional domain and search query parameters.
// GET /api/v1/formulas
func (h *FormulaHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := domain.FormulaFilter{
		Limit:  50,
		Offset: 0,
	}

	if d := r.URL.Query().Get("domain"); d != "" {
		dom := domain.InsuranceDomain(d)
		filter.Domain = &dom
	}
	if s := r.URL.Query().Get("search"); s != "" {
		filter.Search = &s
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			filter.Limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			filter.Offset = v
		}
	}

	formulas, total, err := h.Formulas.List(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list formulas", Code: http.StatusInternalServerError})
		return
	}
	if formulas == nil {
		formulas = []*domain.Formula{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"formulas": formulas,
		"total":    total,
		"limit":    filter.Limit,
		"offset":   filter.Offset,
	})
}

// Create creates a new formula.
// POST /api/v1/formulas
func (h *FormulaHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authentication required", Code: http.StatusUnauthorized})
		return
	}

	var req CreateFormulaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name is required", Code: http.StatusBadRequest})
		return
	}
	if req.Domain == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "domain is required", Code: http.StatusBadRequest})
		return
	}

	now := time.Now().UTC()
	formula := &domain.Formula{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Domain:      req.Domain,
		Description: req.Description,
		CreatedBy:   claims.UserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.Formulas.Create(r.Context(), formula); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create formula", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusCreated, formula)
}

// Get returns a single formula by ID.
// GET /api/v1/formulas/{id}
func (h *FormulaHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	formula, err := h.Formulas.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}
	writeJSON(w, http.StatusOK, formula)
}

// Update modifies a formula's metadata.
// PUT /api/v1/formulas/{id}
func (h *FormulaHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formula, err := h.Formulas.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}

	var req UpdateFormulaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Name != nil {
		formula.Name = *req.Name
	}
	if req.Description != nil {
		formula.Description = *req.Description
	}
	formula.UpdatedAt = time.Now().UTC()

	if err := h.Formulas.Update(r.Context(), formula); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to update formula", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusOK, formula)
}

// Delete removes a formula by ID.
// DELETE /api/v1/formulas/{id}
func (h *FormulaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.Formulas.GetByID(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}

	if err := h.Formulas.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to delete formula", Code: http.StatusInternalServerError})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
