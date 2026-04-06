package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// UserHandler implements user management HTTP endpoints.
type UserHandler struct {
	Users store.UserRepository
}

// List returns all users. Requires admin role.
// GET /api/v1/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.Users.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list users", Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// Delete removes a user. Requires admin role. Cannot delete yourself or the last admin.
// DELETE /api/v1/users/{id}
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.UserID == id {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "cannot delete yourself", Code: http.StatusBadRequest})
		return
	}

	if err := h.Users.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found", Code: http.StatusNotFound})
		case errors.Is(err, store.ErrLastAdmin):
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "cannot delete the last administrator", Code: http.StatusConflict})
		case errors.Is(err, store.ErrHasContent):
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "user has associated formulas and cannot be deleted", Code: http.StatusConflict})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to delete user", Code: http.StatusInternalServerError})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateRole changes a user's role. Requires admin role.
// PATCH /api/v1/users/{id}/role
func (h *UserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Role == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "role is required", Code: http.StatusBadRequest})
		return
	}

	if err := h.Users.UpdateRole(r.Context(), id, req.Role); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found", Code: http.StatusNotFound})
		case errors.Is(err, store.ErrLastAdmin):
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "cannot demote the last administrator", Code: http.StatusConflict})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to update role", Code: http.StatusInternalServerError})
		}
		return
	}

	user, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch updated user", Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, user)
}
