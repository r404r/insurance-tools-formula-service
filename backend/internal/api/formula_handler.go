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
	Formulas   store.FormulaRepository
	Versions   store.VersionRepository
	Categories store.CategoryRepository
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
	if _, err := h.Categories.GetBySlug(r.Context(), string(req.Domain)); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid category", Code: http.StatusBadRequest})
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
	if req.Domain != nil {
		if _, err := h.Categories.GetBySlug(r.Context(), string(*req.Domain)); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid category", Code: http.StatusBadRequest})
			return
		}
		formula.Domain = *req.Domain
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

// CopyFormulaRequest is the optional payload for POST /formulas/:id/copy.
// Both fields are optional pointers so callers can distinguish between
// "omit" (nil → use source default) and "set to empty string".
type CopyFormulaRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// Copy duplicates a formula's latest version into a new formula.
// POST /api/v1/formulas/{id}/copy
func (h *FormulaHandler) Copy(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authentication required", Code: http.StatusUnauthorized})
		return
	}

	sourceID := chi.URLParam(r, "id")
	source, err := h.Formulas.GetByID(r.Context(), sourceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}

	versions, err := h.Versions.ListVersions(r.Context(), sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list versions", Code: http.StatusInternalServerError})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "cannot copy a formula without any version", Code: http.StatusBadRequest})
		return
	}

	// Pick the highest version number (matches the editor's "latest = versions[0]" behavior).
	var latest *domain.FormulaVersion
	for _, v := range versions {
		if latest == nil || v.Version > latest.Version {
			latest = v
		}
	}

	// Decode optional overrides.
	var req CopyFormulaRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
			return
		}
	}

	name := source.Name + " (Copy)"
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	// Description: nil means use source; a pointer to "" means intentionally cleared.
	description := source.Description
	if req.Description != nil {
		description = *req.Description
	}

	now := time.Now().UTC()
	newFormulaID := uuid.New().String()
	newFormula := &domain.Formula{
		ID:          newFormulaID,
		Name:        name,
		Domain:      source.Domain,
		Description: description,
		CreatedBy:   claims.UserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.Formulas.Create(r.Context(), newFormula); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create formula", Code: http.StatusInternalServerError})
		return
	}

	// Deep-copy the graph so the source and new formula don't share slices/maps.
	copiedGraph := deepCopyGraph(latest.Graph)

	newVersion := &domain.FormulaVersion{
		ID:         uuid.New().String(),
		FormulaID:  newFormulaID,
		Version:    1,
		State:      domain.StateDraft,
		Graph:      copiedGraph,
		// Change note is written by the backend and shown in the versions UI.
		// Keep it simple and neutral; the source name is the only dynamic part.
		ChangeNote: "copy:" + source.ID,
		CreatedBy:  claims.UserID,
		CreatedAt:  now,
	}
	if err := h.Versions.CreateVersion(r.Context(), newVersion); err != nil {
		// Best-effort rollback: delete the formula shell we just created so the
		// database is not left with an orphaned formula that has zero versions.
		// If the delete also fails we cannot recover here; caller will see 500
		// and the orphan row must be cleaned up out-of-band.
		if delErr := h.Formulas.Delete(r.Context(), newFormulaID); delErr != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Error: "failed to create version and rollback also failed",
				Code:  http.StatusInternalServerError,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create version", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusCreated, newFormula)
}

// deepCopyGraph returns an independent copy of the given FormulaGraph so that
// mutations on the copy do not affect the source.
func deepCopyGraph(src domain.FormulaGraph) domain.FormulaGraph {
	dst := domain.FormulaGraph{
		Nodes:   make([]domain.FormulaNode, len(src.Nodes)),
		Edges:   make([]domain.FormulaEdge, len(src.Edges)),
		Outputs: append([]string(nil), src.Outputs...),
	}
	for i, n := range src.Nodes {
		dst.Nodes[i] = domain.FormulaNode{
			ID:          n.ID,
			Type:        n.Type,
			Description: n.Description,
			Config:      append(json.RawMessage(nil), n.Config...),
		}
	}
	copy(dst.Edges, src.Edges)
	if src.Layout != nil {
		positions := make(map[string]domain.Position, len(src.Layout.Positions))
		for k, v := range src.Layout.Positions {
			positions[k] = v
		}
		dst.Layout = &domain.GraphLayout{Positions: positions}
	}
	return dst
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
