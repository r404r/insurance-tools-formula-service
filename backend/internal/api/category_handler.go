package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

var categorySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// CategoryHandler implements category CRUD HTTP endpoints.
type CategoryHandler struct {
	Categories store.CategoryRepository
	Formulas   store.FormulaRepository
	Tables     store.TableRepository
}

// NewCategoryHandler creates a new CategoryHandler.
func NewCategoryHandler(categories store.CategoryRepository, formulas store.FormulaRepository, tables store.TableRepository) *CategoryHandler {
	return &CategoryHandler{
		Categories: categories,
		Formulas:   formulas,
		Tables:     tables,
	}
}

// List returns all categories ordered by sort_order.
// GET /api/v1/categories
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	categories, err := h.Categories.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list categories", Code: http.StatusInternalServerError})
		return
	}
	if categories == nil {
		categories = []*domain.Category{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"categories": categories,
	})
}

// Create creates a new category.
// POST /api/v1/categories
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)

	if req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "slug is required", Code: http.StatusBadRequest})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name is required", Code: http.StatusBadRequest})
		return
	}
	if !categorySlugPattern.MatchString(req.Slug) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "slug must use lowercase letters, numbers, and hyphens", Code: http.StatusBadRequest})
		return
	}

	// Check for duplicate slug.
	if existing, err := h.Categories.GetBySlug(r.Context(), req.Slug); err == nil && existing != nil {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "category with this slug already exists", Code: http.StatusConflict})
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to validate category slug", Code: http.StatusInternalServerError})
		return
	}

	now := time.Now().UTC()
	color := req.Color
	if color == "" {
		color = "#6366f1"
	}

	category := &domain.Category{
		ID:          uuid.New().String(),
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		Color:       color,
		SortOrder:   req.SortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.Categories.Create(r.Context(), category); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create category", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusCreated, category)
}

// Update modifies a category's metadata.
// PUT /api/v1/categories/{id}
func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	category, err := h.Categories.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "category not found", Code: http.StatusNotFound})
		return
	}

	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Name != nil {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName == "" {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name is required", Code: http.StatusBadRequest})
			return
		}
		category.Name = trimmedName
	}
	if req.Description != nil {
		category.Description = strings.TrimSpace(*req.Description)
	}
	if req.Color != nil {
		category.Color = *req.Color
	}
	if req.SortOrder != nil {
		category.SortOrder = *req.SortOrder
	}
	category.UpdatedAt = time.Now().UTC()

	if err := h.Categories.Update(r.Context(), category); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to update category", Code: http.StatusInternalServerError})
		return
	}

	writeJSON(w, http.StatusOK, category)
}

// Delete removes a category by ID.
// DELETE /api/v1/categories/{id}
func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	category, err := h.Categories.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "category not found", Code: http.StatusNotFound})
		return
	}

	domainFilter := domain.InsuranceDomain(category.Slug)
	_, total, err := h.Formulas.List(r.Context(), domain.FormulaFilter{
		Domain: &domainFilter,
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to check category usage", Code: http.StatusInternalServerError})
		return
	}
	if total > 0 {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "category is still in use", Code: http.StatusConflict})
		return
	}

	tables, err := h.Tables.List(r.Context(), &domainFilter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to check category usage", Code: http.StatusInternalServerError})
		return
	}
	if len(tables) > 0 {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "category is still in use", Code: http.StatusConflict})
		return
	}

	if err := h.Categories.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to delete category", Code: http.StatusInternalServerError})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
