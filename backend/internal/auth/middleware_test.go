package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// mockUserRepo is a test double for store.UserRepository that only needs
// GetTokenVersion; all other methods panic if called unexpectedly.
type mockUserRepo struct {
	versions map[string]int  // id → token_version; missing = user deleted (ErrNoRows)
	dbErr    map[string]bool // id → simulate transient DB error
}

var _ store.UserRepository = (*mockUserRepo)(nil)

func (m *mockUserRepo) GetTokenVersion(_ context.Context, id string) (int, error) {
	if m.dbErr[id] {
		return 0, errors.New("db: connection refused")
	}
	v, ok := m.versions[id]
	if !ok {
		return 0, sql.ErrNoRows
	}
	return v, nil
}

func (m *mockUserRepo) Create(_ context.Context, _ *domain.User) error         { return nil }
func (m *mockUserRepo) GetByID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepo) GetByUsername(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepo) List(_ context.Context) ([]*domain.User, error)                  { return nil, nil }
func (m *mockUserRepo) UpdateRole(_ context.Context, _ string, _ domain.Role) error     { return nil }
func (m *mockUserRepo) Delete(_ context.Context, _ string) error                        { return nil }

func newJWTMgr() *auth.JWTManager {
	return auth.NewJWTManager("test-secret-not-for-production", 24*time.Hour)
}

func genToken(t *testing.T, mgr *auth.JWTManager, u *domain.User) string {
	t.Helper()
	tok, err := mgr.Generate(u)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// TestCookiePriorityOverBearer verifies that the cookie is used when both
// a valid cookie and a Bearer header are present — even if the Bearer token
// belongs to a different (stale) user.
func TestCookiePriorityOverBearer(t *testing.T) {
	mgr := newJWTMgr()
	cookieUser := &domain.User{ID: "cookie-user", Role: domain.RoleViewer, TokenVersion: 0}
	bearerUser := &domain.User{ID: "bearer-user", Role: domain.RoleAdmin, TokenVersion: 0}

	repo := &mockUserRepo{versions: map[string]int{"cookie-user": 0, "bearer-user": 0}}
	mw := auth.AuthMiddleware(mgr, repo)

	var gotID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := auth.GetClaims(r.Context()); c != nil {
			gotID = c.UserID
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: genToken(t, mgr, cookieUser)})
	req.Header.Set("Authorization", "Bearer "+genToken(t, mgr, bearerUser))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotID != "cookie-user" {
		t.Fatalf("expected cookie-user, got %q", gotID)
	}
}

// TestTokenVersionMismatch verifies that a token issued before a role change
// (version 0) is rejected when the DB shows version 1.
func TestTokenVersionMismatch(t *testing.T) {
	mgr := newJWTMgr()
	user := &domain.User{ID: "u1", Role: domain.RoleViewer, TokenVersion: 0}

	repo := &mockUserRepo{versions: map[string]int{"u1": 1}} // DB version bumped
	mw := auth.AuthMiddleware(mgr, repo)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(t, mgr, user))

	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on version mismatch, got %d", rr.Code)
	}
}

// TestDeletedUser verifies that a token for a deleted user gets 401
// (GetTokenVersion returns sql.ErrNoRows → treated as invalidated token).
func TestDeletedUser(t *testing.T) {
	mgr := newJWTMgr()
	user := &domain.User{ID: "gone", Role: domain.RoleViewer, TokenVersion: 0}

	repo := &mockUserRepo{versions: map[string]int{}} // user not in DB
	mw := auth.AuthMiddleware(mgr, repo)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(t, mgr, user))

	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for deleted user, got %d", rr.Code)
	}
}

// TestDBError_Returns500 verifies that a transient DB error returns 500
// instead of 401, so a DB blip does not silently log everyone out.
func TestDBError_Returns500(t *testing.T) {
	mgr := newJWTMgr()
	user := &domain.User{ID: "u1", Role: domain.RoleViewer, TokenVersion: 0}

	repo := &mockUserRepo{
		versions: map[string]int{"u1": 0},
		dbErr:    map[string]bool{"u1": true},
	}
	mw := auth.AuthMiddleware(mgr, repo)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(t, mgr, user))

	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on DB error, got %d", rr.Code)
	}
}

// TestMissingAuth verifies that requests with no credentials get 401.
func TestMissingAuth(t *testing.T) {
	mgr := newJWTMgr()
	repo := &mockUserRepo{versions: map[string]int{}}
	mw := auth.AuthMiddleware(mgr, repo)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no credentials, got %d", rr.Code)
	}
}

// TestValidBearerToken verifies the happy path with a valid Bearer token.
func TestValidBearerToken(t *testing.T) {
	mgr := newJWTMgr()
	user := &domain.User{ID: "u1", Role: domain.RoleViewer, TokenVersion: 0}

	repo := &mockUserRepo{versions: map[string]int{"u1": 0}}
	mw := auth.AuthMiddleware(mgr, repo)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(t, mgr, user))

	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d", rr.Code)
	}
}
