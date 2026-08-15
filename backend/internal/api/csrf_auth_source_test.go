package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// csrfAuthSourceUserRepo supplies the token-version lookup needed by the
// authentication middleware. The other methods make unexpected handler work
// immediately visible in these router-level tests.
type csrfAuthSourceUserRepo struct {
	versions map[string]int
}

var _ store.UserRepository = (*csrfAuthSourceUserRepo)(nil)

func (r *csrfAuthSourceUserRepo) GetTokenVersion(_ context.Context, id string) (int, error) {
	version, ok := r.versions[id]
	if !ok {
		return 0, sql.ErrNoRows
	}
	return version, nil
}

func (r *csrfAuthSourceUserRepo) Create(context.Context, *domain.User) error { return nil }
func (r *csrfAuthSourceUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return nil, sql.ErrNoRows
}
func (r *csrfAuthSourceUserRepo) GetByUsername(context.Context, string) (*domain.User, error) {
	return nil, sql.ErrNoRows
}
func (r *csrfAuthSourceUserRepo) List(context.Context) ([]*domain.User, error) { return nil, nil }
func (r *csrfAuthSourceUserRepo) UpdateRole(context.Context, string, domain.Role) error {
	return nil
}
func (r *csrfAuthSourceUserRepo) Delete(context.Context, string) error { return nil }

func newCSRFSourceTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()

	jwtMgr := auth.NewJWTManager("csrf-source-test-secret-not-for-production", time.Hour)
	user := &domain.User{
		ID:           "csrf-source-admin",
		Username:     "admin",
		Role:         domain.RoleAdmin,
		TokenVersion: 0,
	}
	token, err := jwtMgr.Generate(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	repo := &csrfAuthSourceUserRepo{versions: map[string]int{user.ID: user.TokenVersion}}
	router := NewRouter(RouterConfig{
		AuthHandler: &AuthHandler{
			Users:        repo,
			JWTMgr:       jwtMgr,
			CookieSecure: false,
		},
		JWTManager:  jwtMgr,
		UserStore:   repo,
		Logger:      zerolog.Nop(),
		CORSOrigins: []string{"http://localhost:5173"},
		CalcLimiter: NewDynamicConcurrencyLimiter(1),
		SeedResetHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	return router, token
}

func TestRouterCSRFUsesCredentialSource(t *testing.T) {
	router, token := newCSRFSourceTestRouter(t)

	tests := []struct {
		name       string
		addCookie  bool
		addBearer  bool
		origin     string
		wantStatus int
	}{
		{
			name:       "bearer-only unsafe request without origin is allowed",
			addBearer:  true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "cookie-only unsafe request without origin is rejected",
			addCookie:  true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cookie unsafe request from an allowed origin is allowed",
			addCookie:  true,
			origin:     "http://localhost:5173",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "cookie plus bearer remains subject to cookie csrf protection",
			addCookie:  true,
			addBearer:  true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reset-seed", nil)
			if tt.addCookie {
				req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: token})
			}
			if tt.addBearer {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestLogoutRejectsCrossOriginCookieRequest(t *testing.T) {
	router, token := newCSRFSourceTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: token})
	req.Header.Set("Origin", "https://attacker.example")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-origin cookie logout status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}
