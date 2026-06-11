package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// contextKey is an unexported type used for context value keys, preventing
// collisions with keys defined in other packages.
type contextKey struct{}

var claimsKey = contextKey{}

const AuthCookieName = "auth_token"

// AuthMiddleware returns HTTP middleware that extracts a JWT from either the
// "auth_token" httpOnly cookie (preferred) or the Authorization Bearer header
// (fallback for API clients), verifies it, checks the token version against
// the DB to catch invalidated tokens, and stores the resulting Claims in the
// request context.
//
// Error responses:
//   - 401: missing/invalid token, version mismatch, or user deleted (ErrNoRows)
//   - 500: transient DB error while checking token version
func AuthMiddleware(jwtMgr *JWTManager, users store.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			if cookie, err := r.Cookie(AuthCookieName); err == nil {
				tokenStr = cookie.Value
			} else {
				header := r.Header.Get("Authorization")
				if header == "" {
					http.Error(w, `{"error":"missing authorization","code":401}`, http.StatusUnauthorized)
					return
				}
				parts := strings.SplitN(header, " ", 2)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
					http.Error(w, `{"error":"invalid authorization format","code":401}`, http.StatusUnauthorized)
					return
				}
				tokenStr = parts[1]
			}

			claims, err := jwtMgr.Verify(tokenStr)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token","code":401}`, http.StatusUnauthorized)
				return
			}

			// Verify token_version against the DB to catch tokens invalidated by
			// a role change. We distinguish three cases:
			//   - user deleted (sql.ErrNoRows) → 401
			//   - version mismatch               → 401
			//   - transient DB error             → 500 (don't silently log everyone out)
			dbVersion, err := users.GetTokenVersion(r.Context(), claims.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, `{"error":"token has been invalidated","code":401}`, http.StatusUnauthorized)
				} else {
					http.Error(w, `{"error":"authentication service unavailable","code":500}`, http.StatusInternalServerError)
				}
				return
			}
			if dbVersion != claims.TokenVersion {
				http.Error(w, `{"error":"token has been invalidated","code":401}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission returns HTTP middleware that checks whether the
// authenticated user (from context claims) holds the specified permission.
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"authentication required","code":401}`, http.StatusUnauthorized)
				return
			}
			if !HasPermission(claims.Role, perm) {
				http.Error(w, `{"error":"insufficient permissions","code":403}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetClaims extracts the JWT claims stored in the context by AuthMiddleware.
// Returns nil if no claims are present.
func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}

// WithClaims returns a new context with the given Claims attached. Used by
// tests that need to bypass the HTTP middleware and inject a synthetic
// authenticated user — production code goes through AuthMiddleware which
// uses the same key.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}
