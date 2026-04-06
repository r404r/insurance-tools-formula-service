package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// validateTableData checks that data is a JSON array of string-map objects,
// which is the format the calculation engine expects.
func validateTableData(data json.RawMessage) error {
	if len(data) == 0 {
		return nil
	}
	var rows []map[string]string
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("data must be a JSON array of string-map objects: %w", err)
	}
	return nil
}

// TableHandler implements lookup table HTTP endpoints.
type TableHandler struct {
	Tables     store.TableRepository
	Categories store.CategoryRepository
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
	if _, err := h.Categories.GetBySlug(r.Context(), string(req.Domain)); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid category", Code: http.StatusBadRequest})
		return
	}
	if err := validateTableData(req.Data); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: http.StatusBadRequest})
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

// Update modifies an existing lookup table's name, tableType, and data.
// PUT /api/v1/tables/{id}
func (h *TableHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}
	if req.Name == "" || req.TableType == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name and tableType are required", Code: http.StatusBadRequest})
		return
	}

	existing, err := h.Tables.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "table not found", Code: http.StatusNotFound})
		return
	}

	existing.Name = req.Name
	existing.TableType = req.TableType
	if req.Data != nil {
		if err := validateTableData(req.Data); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: http.StatusBadRequest})
			return
		}
		existing.Data = req.Data
	}

	if err := h.Tables.Update(r.Context(), existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "table not found", Code: http.StatusNotFound})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to update table", Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// Delete removes a lookup table.
// DELETE /api/v1/tables/{id}
func (h *TableHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Tables.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "table not found", Code: http.StatusNotFound})
		case errors.Is(err, store.ErrTableInUse):
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "table is referenced by formula versions and cannot be deleted", Code: http.StatusConflict})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to delete table", Code: http.StatusInternalServerError})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
