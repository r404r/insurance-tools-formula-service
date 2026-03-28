package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// TableHandler implements lookup table HTTP endpoints.
type TableHandler struct {
	Tables store.TableRepository
}

// List returns lookup tables, optionally filtered by domain.
// GET /api/v1/tables
func (h *TableHandler) List(w http.ResponseWriter, r *http.Request) {
	var domainFilter *domain.InsuranceDomain
	if d := r.URL.Query().Get("domain"); d != "" {
		dom := domain.InsuranceDomain(d)
		domainFilter = &dom
	}

	tables, err := h.Tables.List(r.Context(), domainFilter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list tables", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// Create creates a new lookup table.
// POST /api/v1/tables
func (h *TableHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Name == "" || req.Domain == "" || req.TableType == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name, domain, and tableType are required", Code: http.StatusBadRequest})
		return
	}

	table := &domain.LookupTable{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Domain:    req.Domain,
		TableType: req.TableType,
		Data:      req.Data,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.Tables.Create(r.Context(), table); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create table", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusCreated, table)
}

// Get returns a single lookup table by ID.
// GET /api/v1/tables/{id}
func (h *TableHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	table, err := h.Tables.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "table not found", Code: http.StatusNotFound})
		return
	}
	writeJSON(w, http.StatusOK, table)
}
