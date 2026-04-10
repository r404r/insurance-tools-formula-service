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
	"github.com/r404r/insurance-tools/formula-service/backend/internal/parser"
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
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
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

// ExportFormat is the version identifier for the export JSON schema.
const ExportFormat = "1.0"

// ExportRequest is the payload for POST /formulas/export.
type ExportRequest struct {
	IDs []string `json:"ids"`
}

// ExportedFormula is a single formula entry in an export file.
type ExportedFormula struct {
	SourceID      string              `json:"sourceId"`
	SourceVersion int                 `json:"sourceVersion"`
	Name          string              `json:"name"`
	Domain        domain.InsuranceDomain `json:"domain"`
	Description   string              `json:"description"`
	Graph         domain.FormulaGraph `json:"graph"`
}

// ExportBundle is the top-level shape of an export file.
type ExportBundle struct {
	Version    string            `json:"version"`
	ExportedAt time.Time         `json:"exportedAt"`
	Formulas   []ExportedFormula `json:"formulas"`
}

// Export returns the specified formulas as a downloadable JSON file.
// POST /api/v1/formulas/export
func (h *FormulaHandler) Export(w http.ResponseWriter, r *http.Request) {
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ids is required and must be non-empty", Code: http.StatusBadRequest})
		return
	}
	if len(req.IDs) > 500 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "too many ids (max 500)", Code: http.StatusBadRequest})
		return
	}

	bundle := ExportBundle{
		Version:    ExportFormat,
		ExportedAt: time.Now().UTC(),
		Formulas:   make([]ExportedFormula, 0, len(req.IDs)),
	}

	for _, id := range req.IDs {
		formula, err := h.Formulas.GetByID(r.Context(), id)
		if err != nil {
			// Skip missing formulas silently; caller can diff requested vs returned.
			continue
		}
		versions, err := h.Versions.ListVersions(r.Context(), id)
		if err != nil || len(versions) == 0 {
			continue
		}
		// Pick the highest version number.
		var latest *domain.FormulaVersion
		for _, v := range versions {
			if latest == nil || v.Version > latest.Version {
				latest = v
			}
		}
		bundle.Formulas = append(bundle.Formulas, ExportedFormula{
			SourceID:      formula.ID,
			SourceVersion: latest.Version,
			Name:          formula.Name,
			Domain:        formula.Domain,
			Description:   formula.Description,
			Graph:         latest.Graph,
		})
	}

	filename := "formulas-export.json"
	if len(bundle.Formulas) == 1 {
		// Use the formula name (sanitized) for single exports.
		filename = sanitizeFilename(bundle.Formulas[0].Name) + ".json"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	// Expose counts so the client can detect partial exports.
	w.Header().Set("X-Export-Requested", strconv.Itoa(len(req.IDs)))
	w.Header().Set("X-Export-Exported", strconv.Itoa(len(bundle.Formulas)))
	w.Header().Set("Access-Control-Expose-Headers", "X-Export-Requested, X-Export-Exported")
	_ = json.NewEncoder(w).Encode(bundle)
}

// ImportResult describes the outcome of an import request.
type ImportResult struct {
	Imported []ImportedItem `json:"imported"`
	Errors   []ImportError  `json:"errors"`
}

// ImportedItem is a successfully imported formula.
type ImportedItem struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Name  string `json:"name"`
}

// ImportError is a failed formula in an import batch.
type ImportError struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// Import ingests an export bundle and creates new formulas.
// POST /api/v1/formulas/import
func (h *FormulaHandler) Import(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authentication required", Code: http.StatusUnauthorized})
		return
	}

	var bundle ExportBundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid import body: " + err.Error(), Code: http.StatusBadRequest})
		return
	}
	if bundle.Version != "" && bundle.Version != ExportFormat {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "unsupported export format version: " + bundle.Version + " (expected " + ExportFormat + ")",
			Code:  http.StatusBadRequest,
		})
		return
	}
	if len(bundle.Formulas) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "import bundle has no formulas", Code: http.StatusBadRequest})
		return
	}
	if len(bundle.Formulas) > 500 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "too many formulas in bundle (max 500)", Code: http.StatusBadRequest})
		return
	}

	result := ImportResult{
		Imported: []ImportedItem{},
		Errors:   []ImportError{},
	}
	now := time.Now().UTC()

	for i, ef := range bundle.Formulas {
		// Basic validation.
		if ef.Name == "" {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "name is required"})
			continue
		}
		if ef.Domain == "" {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "domain is required"})
			continue
		}
		if _, err := h.Categories.GetBySlug(r.Context(), string(ef.Domain)); err != nil {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "unknown domain: " + string(ef.Domain)})
			continue
		}
		if len(ef.Graph.Nodes) == 0 {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "graph has no nodes"})
			continue
		}
		// Run the full graph validator to reject malformed graphs (cycles,
		// broken edges, duplicate node IDs, missing outputs, invalid configs).
		graphCopy := ef.Graph
		if verrs := parser.ValidateGraph(&graphCopy); len(verrs) > 0 {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "invalid graph: " + verrs[0].Message})
			continue
		}

		newFormulaID := uuid.New().String()
		newFormula := &domain.Formula{
			ID:          newFormulaID,
			Name:        ef.Name,
			Domain:      ef.Domain,
			Description: ef.Description,
			CreatedBy:   claims.UserID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := h.Formulas.Create(r.Context(), newFormula); err != nil {
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: "create formula: " + err.Error()})
			continue
		}

		copiedGraph := deepCopyGraph(ef.Graph)
		newVersion := &domain.FormulaVersion{
			ID:         uuid.New().String(),
			FormulaID:  newFormulaID,
			Version:    1,
			State:      domain.StateDraft,
			Graph:      copiedGraph,
			ChangeNote: "import:" + ef.SourceID,
			CreatedBy:  claims.UserID,
			CreatedAt:  now,
		}
		if err := h.Versions.CreateVersion(r.Context(), newVersion); err != nil {
			errMsg := "create version: " + err.Error()
			// Best-effort rollback of the formula shell; surface rollback failure.
			if delErr := h.Formulas.Delete(r.Context(), newFormulaID); delErr != nil {
				errMsg += "; rollback failed: " + delErr.Error()
			}
			result.Errors = append(result.Errors, ImportError{Index: i, Name: ef.Name, Error: errMsg})
			continue
		}

		result.Imported = append(result.Imported, ImportedItem{Index: i, ID: newFormulaID, Name: ef.Name})
	}

	writeJSON(w, http.StatusOK, result)
}

// sanitizeFilename returns a filename-safe version of the given string.
func sanitizeFilename(name string) string {
	safe := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == 0 {
			safe = append(safe, '_')
			continue
		}
		if r < 0x20 {
			continue
		}
		safe = append(safe, r)
	}
	s := string(safe)
	if s == "" {
		return "formula"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
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
