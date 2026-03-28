package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/api"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/auth"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/config"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/engine"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store/sqlite"
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

	// Open database.
	store, err := sqlite.New(cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	// Run migrations.
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info().Str("driver", cfg.Database.Driver).Msg("database migrated")

	// Initialize subsystems.
	jwtMgr := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	eng := engine.NewEngine(engine.EngineConfig{
		Workers: cfg.Engine.MaxWorkers,
		Precision: engine.PrecisionConfig{
			IntermediatePrecision: cfg.Engine.IntermediatePrecision,
			OutputPrecision:       cfg.Engine.OutputPrecision,
		},
		CacheSize: cfg.Engine.CacheSize,
	})

	// Create handlers.
	authHandler := &api.AuthHandler{
		Users:  store.Users(),
		JWTMgr: jwtMgr,
	}
	formulaHandler := &api.FormulaHandler{
		Formulas: store.Formulas(),
	}
	versionHandler := &api.VersionHandler{
		Versions: store.Versions(),
		Formulas: store.Formulas(),
	}
	calcHandler := &api.CalcHandler{
		Engine:   eng,
		Versions: store.Versions(),
		Formulas: store.Formulas(),
		Tables:   store.Tables(),
	}

	// Build the router.
	router := api.NewRouter(api.RouterConfig{
		AuthHandler:    authHandler,
		FormulaHandler: formulaHandler,
		VersionHandler: versionHandler,
		CalcHandler:    calcHandler,
		JWTManager:     jwtMgr,
		Logger:         logger,
		CORSOrigins:    cfg.Server.CORSOrigins,
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
