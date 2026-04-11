package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/api"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/config"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/engine"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store/mysql"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store/postgres"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store/sqlite"
	"github.com/r404r/insurance-tools/formula-service/backend/seed"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	if err := run(logger); err != nil {
		logger.Fatal().Err(err).Msg("server exited with error")
	}
}

func run(logger zerolog.Logger) error {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Set a default JWT secret for development.
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "dev-secret-change-in-production"
		logger.Warn().Msg("AUTH_JWT_SECRET not set, using insecure default (development only)")
	}

	// Open database — driver selected by DB_DRIVER env var.
	var st store.Store
	switch cfg.Database.Driver {
	case "postgres":
		st, err = postgres.New(cfg.Database.DSN)
	case "mysql":
		st, err = mysql.New(cfg.Database.DSN)
	case "sqlite":
		st, err = sqlite.New(cfg.Database.DSN)
	default:
		return fmt.Errorf("DB_DRIVER %q is not supported; use sqlite, postgres, or mysql", cfg.Database.Driver)
	}
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	store := st
	defer store.Close()

	// Run migrations.
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info().Str("driver", cfg.Database.Driver).Msg("database migrated")

	// Seed default data (admin user + sample formulas).
	if err := bootstrap(context.Background(), store, logger); err != nil {
		return fmt.Errorf("seed data: %w", err)
	}

	// Initialize subsystems.
	jwtMgr := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	tableResolver := &engine.StoreTableResolver{Tables: store.Tables()}
	formulaResolver := &engine.StoreFormulaResolver{Versions: store.Versions()}

	eng := engine.NewEngine(engine.EngineConfig{
		Workers: cfg.Engine.MaxWorkers,
		Precision: engine.PrecisionConfig{
			IntermediatePrecision: cfg.Engine.IntermediatePrecision,
			OutputPrecision:       cfg.Engine.OutputPrecision,
		},
		CacheSize:       cfg.Engine.CacheSize,
		TableResolver:   tableResolver,
		FormulaResolver: formulaResolver,
	})

	// Create handlers.
	authHandler := &api.AuthHandler{
		Users:  store.Users(),
		JWTMgr: jwtMgr,
	}
	formulaHandler := &api.FormulaHandler{
		Formulas:   store.Formulas(),
		Versions:   store.Versions(),
		Categories: store.Categories(),
	}
	versionHandler := &api.VersionHandler{
		Versions: store.Versions(),
		Formulas: store.Formulas(),
		Cache:    eng,
	}
	calcHandler := &api.CalcHandler{
		Engine:   eng,
		Versions: store.Versions(),
		Formulas: store.Formulas(),
		Tables:   store.Tables(),
		// Limiter wired below, after it is constructed.
	}

	tableHandler := &api.TableHandler{
		Tables:     store.Tables(),
		Categories: store.Categories(),
		Cache:      eng,
	}
	userHandler := &api.UserHandler{
		Users: store.Users(),
	}
	categoryHandler := api.NewCategoryHandler(store.Categories(), store.Formulas(), store.Tables())
	parseHandler := &api.ParseHandler{}
	cacheHandler := &api.CacheHandler{Engine: eng}
	templateHandler := &api.TemplateHandler{}

	// Load persisted settings and initialise dynamic concurrency limiter.
	maxCalcs := cfg.Engine.MaxConcurrentCalcs
	if v, err := store.Settings().Get(context.Background(), api.SettingMaxConcurrentCalcs); err == nil {
		if n := parseInt(v, maxCalcs); n >= 0 {
			maxCalcs = n
		}
	}
	calcLimiter := api.NewDynamicConcurrencyLimiter(maxCalcs)
	calcHandler.Limiter = calcLimiter
	settingsHandler := &api.SettingsHandler{
		Settings: store.Settings(),
		Limiter:  calcLimiter,
	}

	// Build the router.
	resetSeedHandler := makeSeedResetHandler(st, logger)

	router := api.NewRouter(api.RouterConfig{
		AuthHandler:      authHandler,
		FormulaHandler:   formulaHandler,
		VersionHandler:   versionHandler,
		CalcHandler:      calcHandler,
		TableHandler:     tableHandler,
		UserHandler:      userHandler,
		CategoryHandler:  categoryHandler,
		ParseHandler:     parseHandler,
		CacheHandler:     cacheHandler,
		SettingsHandler:  settingsHandler,
		TemplateHandler:  templateHandler,
		SeedResetHandler: resetSeedHandler,
		JWTManager:       jwtMgr,
		Logger:           logger,
		CORSOrigins:      cfg.Server.CORSOrigins,
		CalcLimiter:      calcLimiter,
	})

	// Start HTTP server.
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown.
	errCh := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", addr).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info().Str("signal", sig.String()).Msg("shutting down server")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	logger.Info().Msg("server stopped")
	return nil
}

// bootstrap creates the minimum data required for the API to be usable on
// a fresh database: the default admin account (so something can log in)
// and the default insurance categories (so the formula list page renders).
// All other seed content (formulas, lookup tables) is loaded out-of-process
// by the seed-runner CLI; see backend/seed/README.md.
func bootstrap(ctx context.Context, s store.Store, logger zerolog.Logger) error {
	// --- Default admin account ---
	adminID := "00000000-0000-0000-0000-000000000001"
	if _, err := s.Users().GetByUsername(ctx, "admin"); err != nil {
		hashed, err := bcrypt.GenerateFromPassword([]byte("admin99999"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash admin password: %w", err)
		}
		admin := &domain.User{
			ID:        adminID,
			Username:  "admin",
			Password:  string(hashed),
			Role:      domain.RoleAdmin,
			CreatedAt: time.Now().UTC(),
		}
		if err := s.Users().Create(ctx, admin); err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		logger.Info().Msg("default admin account created (admin / admin99999)")
	} else {
		logger.Info().Msg("admin account already exists, skipping seed")
	}

	now := time.Now().UTC()

	// --- Default categories ---
	defaultCategories := []struct {
		Slug  string
		Name  string
		Color string
		Order int
	}{
		{"life", "人寿", "#3b82f6", 1},
		{"property", "财产", "#10b981", 2},
		{"auto", "车险", "#f59e0b", 3},
	}
	for _, dc := range defaultCategories {
		if _, err := s.Categories().GetBySlug(ctx, dc.Slug); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get category %s: %w", dc.Slug, err)
			}
			cat := &domain.Category{
				ID:        uuid.New().String(),
				Slug:      dc.Slug,
				Name:      dc.Name,
				Color:     dc.Color,
				SortOrder: dc.Order,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.Categories().Create(ctx, cat); err != nil {
				return fmt.Errorf("create category %s: %w", dc.Slug, err)
			}
			logger.Info().Str("slug", dc.Slug).Msg("seed category created")
		}
	}

	// All other seed data (formulas + lookup tables) is loaded by the
	// out-of-process seed-runner CLI; see backend/seed/README.md.
	return nil
}

func parseInt(s string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ─────────────────────────────────────────────────────────────────
// makeSeedResetHandler returns an http.HandlerFunc that deletes the
// formulas and tables that match seed bundle names. User-created data is
// not affected. Re-seeding is handled out-of-process by the seed-runner
// CLI (see backend/seed/README.md); this handler does not re-create the
// objects itself, so the response includes a hint reminding the operator
// to run the seed-runner.
//
// The seed name lists were inlined string slices before task #047. They
// are now derived at first use from the JSON bundles in
// backend/seed/{formulas,tables}/.
func makeSeedResetHandler(s store.Store, logger zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seedNames, tableNames, err := seed.Names()
		if err != nil {
			logger.Error().Err(err).Msg("seed reset: failed to load embedded seed names")
			http.Error(w, `{"error":"failed to load seed name list","code":500}`, http.StatusInternalServerError)
			return
		}

		// Delete seed formulas by name match.
		formulas, _, _ := s.Formulas().List(r.Context(), domain.FormulaFilter{Limit: 10000})
		deleted := 0
		for _, f := range formulas {
			if seedNames[f.Name] {
				if err := s.Formulas().Delete(r.Context(), f.ID); err != nil {
					logger.Warn().Err(err).Str("name", f.Name).Msg("failed to delete seed formula")
				} else {
					deleted++
				}
			}
		}

		// Delete seed tables (by name match only — tables don't have CreatedBy).
		tables, _ := s.Tables().List(r.Context(), nil)
		tablesDeleted := 0
		for _, t := range tables {
			if tableNames[t.Name] {
				if err := s.Tables().Delete(r.Context(), t.ID); err != nil {
					logger.Warn().Err(err).Str("name", t.Name).Msg("failed to delete seed table")
				} else {
					tablesDeleted++
				}
			}
		}

		logger.Info().Int("formulas", deleted).Int("tables", tablesDeleted).Msg("seed data deleted (re-run seed-runner to re-create)")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"seed data deleted; run seed-runner to re-create","formulasDeleted":%d,"tablesDeleted":%d}`, deleted, tablesDeleted)
	}
}
