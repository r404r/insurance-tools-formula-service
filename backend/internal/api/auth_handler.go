package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// AuthHandler implements authentication and user-related HTTP endpoints.
type AuthHandler struct {
	Users        store.UserRepository
	JWTMgr       *auth.JWTManager
	CookieSecure bool
}

func (h *AuthHandler) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.JWTMgr.Expiry().Seconds()),
	})
}

func (h *AuthHandler) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// Login authenticates a user, sets an httpOnly auth cookie, and returns user info.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "username and password are required", Code: http.StatusBadRequest})
		return
	}

	user, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials", Code: http.StatusUnauthorized})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials", Code: http.StatusUnauthorized})
		return
	}

	token, err := h.JWTMgr.Generate(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to generate token", Code: http.StatusInternalServerError})
		return
	}

	h.setAuthCookie(w, token)
	writeJSON(w, http.StatusOK, LoginResponse{Token: token, User: *user})
}

// Register creates a new user account, sets an httpOnly auth cookie, and returns user info.
// The first registered user is granted the admin role; subsequent users receive the viewer role.
// POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "username and password are required", Code: http.StatusBadRequest})
		return
	}

	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "password must be at least 8 characters", Code: http.StatusBadRequest})
		return
	}

	existing, _ := h.Users.GetByUsername(r.Context(), req.Username)
	if existing != nil {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "username already taken", Code: http.StatusConflict})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to hash password", Code: http.StatusInternalServerError})
		return
	}

	// Determine role: first user becomes admin, all others get viewer.
	// We use an optimistic approach: create with viewer role, then check
	// if this is the first user and upgrade to admin. The unique username
	// constraint prevents duplicates; the worst case in a race is two
	// viewers (no privilege escalation).
	user := &domain.User{
		ID:        uuid.New().String(),
		Username:  req.Username,
		Password:  string(hashed),
		Role:      domain.RoleViewer,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.Users.Create(r.Context(), user); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create user", Code: http.StatusInternalServerError})
		return
	}

	// After successfully creating the user, check if they are the only user.
	// If so, promote to admin. This avoids the race condition where two
	// concurrent registrations both see zero users.
	users, _ := h.Users.List(r.Context())
	if len(users) == 1 && users[0].ID == user.ID {
		_ = h.Users.UpdateRole(r.Context(), user.ID, domain.RoleAdmin)
		user.Role = domain.RoleAdmin
	}

	token, err := h.JWTMgr.Generate(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to generate token", Code: http.StatusInternalServerError})
		return
	}

	h.setAuthCookie(w, token)
	writeJSON(w, http.StatusCreated, LoginResponse{Token: token, User: *user})
}

// Logout clears the auth cookie.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearAuthCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the currently authenticated user.
// GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authentication required", Code: http.StatusUnauthorized})
		return
	}

	user, err := h.Users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found", Code: http.StatusNotFound})
		return
	}

	writeJSON(w, http.StatusOK, user)
}
