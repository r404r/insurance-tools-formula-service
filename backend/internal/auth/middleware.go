package auth

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is an unexported type used for context value keys, preventing
// collisions with keys defined in other packages.
type contextKey struct{}

var claimsKey = contextKey{}

// AuthMiddleware returns HTTP middleware that extracts a Bearer token from the
// Authorization header, verifies it, and stores the resulting Claims in the
// request context.
func AuthMiddleware(jwtMgr *JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization header","code":401}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"invalid authorization format","code":401}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtMgr.Verify(parts[1])
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token","code":401}`, http.StatusUnauthorized)
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
