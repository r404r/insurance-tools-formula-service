package api

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
)

// RouterConfig holds the dependencies needed to construct the API router.
type RouterConfig struct {
	AuthHandler    *AuthHandler
	FormulaHandler *FormulaHandler
	VersionHandler *VersionHandler
	CalcHandler    *CalcHandler
	JWTManager     *auth.JWTManager
	Logger         zerolog.Logger
	CORSOrigins    []string
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
				r.Post("/", cfg.CalcHandler.Calculate)
				r.Post("/batch", cfg.CalcHandler.BatchCalculate)
				r.Post("/validate", cfg.CalcHandler.Validate)
			})
		})
	})

	return r
}
