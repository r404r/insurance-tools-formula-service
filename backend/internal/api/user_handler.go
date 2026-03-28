package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

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
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found", Code: http.StatusNotFound})
		return
	}

	user, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch updated user", Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, user)
}
