package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// VersionHandler implements formula version management HTTP endpoints.
type VersionHandler struct {
	Versions store.VersionRepository
	Formulas store.FormulaRepository
	Cache    CacheInvalidator // optional; cleared when a version is published
}

// List returns all versions for a given formula.
// GET /api/v1/formulas/{id}/versions
func (h *VersionHandler) List(w http.ResponseWriter, r *http.Request) {
	formulaID := chi.URLParam(r, "id")

	if _, err := h.Formulas.GetByID(r.Context(), formulaID); err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}

	versions, err := h.Versions.ListVersions(r.Context(), formulaID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list versions", Code: http.StatusInternalServerError})
		return
	}
	if versions == nil {
		versions = []*domain.FormulaVersion{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

// Create creates a new draft version of a formula. The version number is
// computed as one greater than the current maximum.
// POST /api/v1/formulas/{id}/versions
func (h *VersionHandler) Create(w http.ResponseWriter, r *http.Request) {
	formulaID := chi.URLParam(r, "id")
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authentication required", Code: http.StatusUnauthorized})
		return
	}

	if _, err := h.Formulas.GetByID(r.Context(), formulaID); err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula not found", Code: http.StatusNotFound})
		return
	}

	var req CreateVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	// Determine the next version number.
	existing, _ := h.Versions.ListVersions(r.Context(), formulaID)
	nextVersion := 1
	for _, v := range existing {
		if v.Version >= nextVersion {
			nextVersion = v.Version + 1
		}
	}

	var parentVer *int
	if nextVersion > 1 {
		prev := nextVersion - 1
		parentVer = &prev
	}

	version := &domain.FormulaVersion{
		ID:         uuid.New().String(),
		FormulaID:  formulaID,
		Version:    nextVersion,
		State:      domain.StateDraft,
		Graph:      req.Graph,
		ParentVer:  parentVer,
		ChangeNote: req.ChangeNote,
		CreatedBy:  claims.UserID,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.Versions.CreateVersion(r.Context(), version); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create version", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusCreated, version)
}

// Get returns a specific version of a formula.
// GET /api/v1/formulas/{id}/versions/{ver}
func (h *VersionHandler) Get(w http.ResponseWriter, r *http.Request) {
	formulaID := chi.URLParam(r, "id")
	verStr := chi.URLParam(r, "ver")

	ver, err := strconv.Atoi(verStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid version number", Code: http.StatusBadRequest})
		return
	}

	version, err := h.Versions.GetVersion(r.Context(), formulaID, ver)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "version not found", Code: http.StatusNotFound})
		return
	}

	writeJSON(w, http.StatusOK, version)
}

// UpdateState transitions a version to a new lifecycle state (published or
// archived). Publishing a version automatically archives any previously
// published version of the same formula.
// PATCH /api/v1/formulas/{id}/versions/{ver}
func (h *VersionHandler) UpdateState(w http.ResponseWriter, r *http.Request) {
	formulaID := chi.URLParam(r, "id")
	verStr := chi.URLParam(r, "ver")

	ver, err := strconv.Atoi(verStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid version number", Code: http.StatusBadRequest})
		return
	}

	var req UpdateVersionStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	version, err := h.Versions.GetVersion(r.Context(), formulaID, ver)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "version not found", Code: http.StatusNotFound})
		return
	}

	// Validate state transitions: draft -> published, published -> archived, draft -> archived.
	if !isValidTransition(version.State, req.State) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid state transition from " + string(version.State) + " to " + string(req.State),
			Code:  http.StatusBadRequest,
		})
		return
	}

	// When publishing, archive the currently published version first.
	if req.State == domain.StatePublished {
		published, _ := h.Versions.GetPublished(r.Context(), formulaID)
		if published != nil {
			if err := h.Versions.UpdateState(r.Context(), formulaID, published.Version, domain.StateArchived); err != nil {
				writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to archive previous version", Code: http.StatusInternalServerError})
				return
			}
		}
	}

	if err := h.Versions.UpdateState(r.Context(), formulaID, ver, req.State); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to update version state", Code: http.StatusInternalServerError})
		return
	}

	// Invalidate cache when a new version is published so callers do not
	// receive stale results computed against the previous graph.
	if req.State == domain.StatePublished && h.Cache != nil {
		h.Cache.ClearCache()
	}

	version.State = req.State
	writeJSON(w, http.StatusOK, version)
}

// Diff computes the difference between two versions of a formula.
// GET /api/v1/formulas/{id}/diff?from=X&to=Y
func (h *VersionHandler) Diff(w http.ResponseWriter, r *http.Request) {
	formulaID := chi.URLParam(r, "id")

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "from and to query parameters are required", Code: http.StatusBadRequest})
		return
	}

	fromVer, err := strconv.Atoi(fromStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid from version", Code: http.StatusBadRequest})
		return
	}
	toVer, err := strconv.Atoi(toStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid to version", Code: http.StatusBadRequest})
		return
	}

	fromVersion, err := h.Versions.GetVersion(r.Context(), formulaID, fromVer)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "from version not found", Code: http.StatusNotFound})
		return
	}
	toVersion, err := h.Versions.GetVersion(r.Context(), formulaID, toVer)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "to version not found", Code: http.StatusNotFound})
		return
	}

	diff := computeDiff(fromVersion, toVersion)
	writeJSON(w, http.StatusOK, diff)
}

// isValidTransition checks whether a version state transition is allowed.
func isValidTransition(from, to domain.VersionState) bool {
	switch from {
	case domain.StateDraft:
		return to == domain.StatePublished || to == domain.StateArchived
	case domain.StatePublished:
		return to == domain.StateArchived
	default:
		return false
	}
}

// computeDiff produces a VersionDiff describing the changes between two formula
// versions by comparing their node and edge sets.
func computeDiff(from, to *domain.FormulaVersion) domain.VersionDiff {
	// Initialize all slices to empty (not nil) so they serialise to JSON []
	// rather than null, which would break frontend array operations.
	diff := domain.VersionDiff{
		FromVersion:   from.Version,
		ToVersion:     to.Version,
		AddedNodes:    []domain.FormulaNode{},
		RemovedNodes:  []domain.FormulaNode{},
		ModifiedNodes: []domain.NodeDiff{},
		AddedEdges:    []domain.FormulaEdge{},
		RemovedEdges:  []domain.FormulaEdge{},
	}

	// Index nodes by ID for both versions.
	fromNodes := make(map[string]domain.FormulaNode, len(from.Graph.Nodes))
	for _, n := range from.Graph.Nodes {
		fromNodes[n.ID] = n
	}
	toNodes := make(map[string]domain.FormulaNode, len(to.Graph.Nodes))
	for _, n := range to.Graph.Nodes {
		toNodes[n.ID] = n
	}

	// Find added and modified nodes.
	for _, n := range to.Graph.Nodes {
		old, exists := fromNodes[n.ID]
		if !exists {
			diff.AddedNodes = append(diff.AddedNodes, n)
		} else if !nodesEqual(old, n) {
			diff.ModifiedNodes = append(diff.ModifiedNodes, domain.NodeDiff{
				NodeID: n.ID,
				Before: old,
				After:  n,
			})
		}
	}

	// Find removed nodes.
	for _, n := range from.Graph.Nodes {
		if _, exists := toNodes[n.ID]; !exists {
			diff.RemovedNodes = append(diff.RemovedNodes, n)
		}
	}

	// Index edges by a composite key.
	type edgeKey struct{ source, target, sourcePort, targetPort string }
	fromEdges := make(map[edgeKey]domain.FormulaEdge, len(from.Graph.Edges))
	for _, e := range from.Graph.Edges {
		fromEdges[edgeKey{e.Source, e.Target, e.SourcePort, e.TargetPort}] = e
	}
	toEdges := make(map[edgeKey]domain.FormulaEdge, len(to.Graph.Edges))
	for _, e := range to.Graph.Edges {
		toEdges[edgeKey{e.Source, e.Target, e.SourcePort, e.TargetPort}] = e
	}

	for k, e := range toEdges {
		if _, exists := fromEdges[k]; !exists {
			diff.AddedEdges = append(diff.AddedEdges, e)
		}
	}
	for k, e := range fromEdges {
		if _, exists := toEdges[k]; !exists {
			diff.RemovedEdges = append(diff.RemovedEdges, e)
		}
	}

	// Sort slices for deterministic output.
	sort.Slice(diff.AddedNodes, func(i, j int) bool { return diff.AddedNodes[i].ID < diff.AddedNodes[j].ID })
	sort.Slice(diff.RemovedNodes, func(i, j int) bool { return diff.RemovedNodes[i].ID < diff.RemovedNodes[j].ID })
	sort.Slice(diff.ModifiedNodes, func(i, j int) bool { return diff.ModifiedNodes[i].NodeID < diff.ModifiedNodes[j].NodeID })

	return diff
}

// nodesEqual compares two FormulaNodes for equality.
func nodesEqual(a, b domain.FormulaNode) bool {
	if a.ID != b.ID || a.Type != b.Type {
		return false
	}
	return string(a.Config) == string(b.Config)
}
