package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
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
	SeedResetHandler   http.HandlerFunc // optional: POST /admin/reset-seed
	JWTManager         *auth.JWTManager
	Logger             zerolog.Logger
	CORSOrigins        []string
	CalcLimiter        *DynamicConcurrencyLimiter
}

// NewRouter creates a chi.Mux with all API routes wired up.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware stack.
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(Recovery())
	r.Use(Logger(cfg.Logger))
	r.Use(CORS(cfg.CORSOrigins))

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints.
		r.Post("/auth/login", cfg.AuthHandler.Login)
		r.Post("/auth/register", cfg.AuthHandler.Register)

		// Parse text formula to graph (stateless, no auth needed).
		r.Post("/parse", cfg.ParseHandler.Parse)

		// Formula templates catalogue (public, no auth required).
		r.Get("/templates", cfg.TemplateHandler.List)

		// All remaining routes require authentication.
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddleware(cfg.JWTManager))

			// Current user.
			r.Get("/auth/me", cfg.AuthHandler.Me)

			// Formula CRUD.
			r.Route("/formulas", func(r chi.Router) {
				r.Get("/", cfg.FormulaHandler.List)

				r.With(auth.RequirePermission(auth.PermFormulaCreate)).
					Post("/", cfg.FormulaHandler.Create)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", cfg.FormulaHandler.Get)

					r.With(auth.RequirePermission(auth.PermFormulaEdit)).
						Put("/", cfg.FormulaHandler.Update)

					r.With(auth.RequirePermission(auth.PermFormulaDelete)).
						Delete("/", cfg.FormulaHandler.Delete)

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
				r.Use(cfg.CalcLimiter.Middleware())
				r.Post("/", cfg.CalcHandler.Calculate)
				r.Post("/batch", cfg.CalcHandler.BatchCalculate)
				r.Post("/batch-test", cfg.CalcHandler.BatchTest)
				r.Post("/validate", cfg.CalcHandler.Validate)
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
