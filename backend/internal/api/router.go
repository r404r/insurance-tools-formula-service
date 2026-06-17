package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// RouterConfig holds the dependencies needed to construct the API router.
type RouterConfig struct {
	AuthHandler        *AuthHandler
	FormulaHandler     *FormulaHandler
	VersionHandler     *VersionHandler
	CalcHandler        *CalcHandler
	TableHandler       *TableHandler
	UserHandler        *UserHandler
	CategoryHandler    *CategoryHandler
	ParseHandler       *ParseHandler
	CacheHandler       *CacheHandler
	SettingsHandler    *SettingsHandler
	TemplateHandler    *TemplateHandler
	HealthHandler      *HealthHandler
	SeedResetHandler   http.HandlerFunc // optional: POST /admin/reset-seed
	JWTManager         *auth.JWTManager
	UserStore          store.UserRepository // for token_version check in AuthMiddleware
	Logger             zerolog.Logger
	CORSOrigins        []string
	CalcLimiter        *DynamicConcurrencyLimiter
	// TrustProxy controls real-IP resolution for rate limiting.
	// See config.ServerConfig.TrustProxy for the full deployment note.
	TrustProxy bool
}

// NewRouter creates a chi.Mux with all API routes wired up.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware stack.
	// PreserveRawIP must run before RealIP so rate limiters can read the
	// original TCP connection IP regardless of X-Forwarded-For headers.
	r.Use(PreserveRawIP)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(Recovery())
	r.Use(Logger(cfg.Logger))
	r.Use(CORS(cfg.CORSOrigins))

	// Unauthenticated readiness probe. Lives at the root (outside /api/v1) so
	// container healthchecks and the API regression suite can hit a stable
	// path without auth or rate limiting.
	if cfg.HealthHandler != nil {
		r.Get("/healthz", cfg.HealthHandler.Health)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints with IP rate limiting.
		// login: 5 req/min/IP; register: 3 req/min/IP; parse: 30 req/min/IP.
		r.With(IPRateLimit(5, 5, cfg.TrustProxy)).Post("/auth/login", cfg.AuthHandler.Login)
		r.With(IPRateLimit(3, 3, cfg.TrustProxy)).Post("/auth/register", cfg.AuthHandler.Register)
		r.Post("/auth/logout", cfg.AuthHandler.Logout)

		// Parse text formula to graph (stateless, no auth needed).
		r.With(IPRateLimit(30, 30, cfg.TrustProxy)).Post("/parse", cfg.ParseHandler.Parse)

		// Formula templates catalogue (public, no auth required).
		r.Get("/templates", cfg.TemplateHandler.List)

		// All remaining routes require authentication.
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddleware(cfg.JWTManager, cfg.UserStore))
			r.Use(CSRFProtect(cfg.CORSOrigins))

			// Current user.
			r.Get("/auth/me", cfg.AuthHandler.Me)

			// Formula CRUD.
			r.Route("/formulas", func(r chi.Router) {
				r.Get("/", cfg.FormulaHandler.List)

				r.With(auth.RequirePermission(auth.PermFormulaCreate)).
					Post("/", cfg.FormulaHandler.Create)

				// Export: requires view permission (all roles have it today,
				// but the boundary must be explicit for future role changes).
				r.With(auth.RequirePermission(auth.PermFormulaView)).
					Post("/export", cfg.FormulaHandler.Export)

				// Import: requires formula create permission.
				r.With(auth.RequirePermission(auth.PermFormulaCreate)).
					Post("/import", cfg.FormulaHandler.Import)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", cfg.FormulaHandler.Get)

					r.With(auth.RequirePermission(auth.PermFormulaEdit)).
						Put("/", cfg.FormulaHandler.Update)

					r.With(auth.RequirePermission(auth.PermFormulaDelete)).
						Delete("/", cfg.FormulaHandler.Delete)

					r.With(auth.RequirePermission(auth.PermFormulaCreate)).
						Post("/copy", cfg.FormulaHandler.Copy)

					// Versions.
					r.Get("/versions", cfg.VersionHandler.List)

					r.With(auth.RequirePermission(auth.PermFormulaEdit)).
						Post("/versions", cfg.VersionHandler.Create)

					r.Get("/versions/{ver}", cfg.VersionHandler.Get)

					r.With(auth.RequirePermission(auth.PermVersionPublish)).
						Patch("/versions/{ver}", cfg.VersionHandler.UpdateState)

					// Diff.
					r.Get("/diff", cfg.VersionHandler.Diff)
				})
			})

			// Calculation endpoints.
			r.Route("/calculate", func(r chi.Router) {
				r.Use(auth.RequirePermission(auth.PermCalculate))
				// Single-calculation endpoints go through the HTTP middleware
				// limiter: one request == one slot.
				r.Group(func(r chi.Router) {
					r.Use(cfg.CalcLimiter.Middleware())
					r.Post("/", cfg.CalcHandler.Calculate)
					r.Post("/batch", cfg.CalcHandler.BatchCalculate)
					r.Post("/validate", cfg.CalcHandler.Validate)
				})
				// BatchTest runs many calculations per request. It acquires
				// the shared limiter directly for each inner case, so it must
				// NOT also be gated by the HTTP middleware — otherwise the
				// outer request would hold a phantom slot that the inner loop
				// could never observe, and the global budget would be
				// over-counted.
				r.Post("/batch-test", cfg.CalcHandler.BatchTest)
			})

			// Lookup table endpoints.
			r.Route("/tables", func(r chi.Router) {
				r.Get("/", cfg.TableHandler.List)
				r.With(auth.RequirePermission(auth.PermTableManage)).
					Post("/", cfg.TableHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", cfg.TableHandler.Get)
					r.With(auth.RequirePermission(auth.PermTableManage)).
						Put("/", cfg.TableHandler.Update)
					r.With(auth.RequirePermission(auth.PermTableManage)).
						Delete("/", cfg.TableHandler.Delete)
				})
			})

			// Category endpoints (list for all, CUD for admin).
			r.Route("/categories", func(r chi.Router) {
				r.Get("/", cfg.CategoryHandler.List)
				r.With(auth.RequirePermission(auth.PermUserManage)).Post("/", cfg.CategoryHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.With(auth.RequirePermission(auth.PermUserManage)).Put("/", cfg.CategoryHandler.Update)
					r.With(auth.RequirePermission(auth.PermUserManage)).Delete("/", cfg.CategoryHandler.Delete)
				})
			})

			// User management endpoints (admin only).
			r.Route("/users", func(r chi.Router) {
				r.Use(auth.RequirePermission(auth.PermUserManage))
				r.Get("/", cfg.UserHandler.List)
				r.Route("/{id}", func(r chi.Router) {
					r.Patch("/role", cfg.UserHandler.UpdateRole)
					r.Delete("/", cfg.UserHandler.Delete)
				})
			})

			// Cache management endpoints (admin only — both read and clear).
			r.With(auth.RequirePermission(auth.PermUserManage)).
				Route("/cache", func(r chi.Router) {
					r.Get("/", cfg.CacheHandler.Stats)
					r.Delete("/", cfg.CacheHandler.Clear)
				})

			// Settings endpoints (admin only).
			r.With(auth.RequirePermission(auth.PermUserManage)).
				Route("/settings", func(r chi.Router) {
					r.Get("/", cfg.SettingsHandler.Get)
					r.Put("/", cfg.SettingsHandler.Update)
				})

			// Seed data reset (admin only).
			if cfg.SeedResetHandler != nil {
				r.With(auth.RequirePermission(auth.PermUserManage)).
					Post("/admin/reset-seed", cfg.SeedResetHandler)
			}
		})
	})

	return r
}
