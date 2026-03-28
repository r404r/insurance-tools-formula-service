package main

import (
	"context"
	"encoding/json"
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

	// Seed default data (admin user + sample formulas).
	if err := seed(context.Background(), store, logger); err != nil {
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

	tableHandler := &api.TableHandler{
		Tables: store.Tables(),
	}
	userHandler := &api.UserHandler{
		Users: store.Users(),
	}

	// Build the router.
	router := api.NewRouter(api.RouterConfig{
		AuthHandler:    authHandler,
		FormulaHandler: formulaHandler,
		VersionHandler: versionHandler,
		CalcHandler:    calcHandler,
		TableHandler:   tableHandler,
		UserHandler:    userHandler,
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

// seed creates the default admin user and sample formulas if they don't exist.
func seed(ctx context.Context, s store.Store, logger zerolog.Logger) error {
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

	// Helper: create formula + version if formula doesn't exist.
	seedFormula := func(name string, dom domain.InsuranceDomain, desc string, graph domain.FormulaGraph) error {
		// Check by listing and looking for the name.
		formulas, _, _ := s.Formulas().List(ctx, domain.FormulaFilter{Limit: 1000})
		for _, f := range formulas {
			if f.Name == name {
				return nil // already exists
			}
		}

		formulaID := uuid.New().String()
		formula := &domain.Formula{
			ID:          formulaID,
			Name:        name,
			Domain:      dom,
			Description: desc,
			CreatedBy:   adminID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.Formulas().Create(ctx, formula); err != nil {
			return fmt.Errorf("create formula %s: %w", name, err)
		}

		version := &domain.FormulaVersion{
			ID:         uuid.New().String(),
			FormulaID:  formulaID,
			Version:    1,
			State:      domain.StatePublished,
			Graph:      graph,
			ChangeNote: "Initial version (seed data)",
			CreatedBy:  adminID,
			CreatedAt:  now,
		}
		if err := s.Versions().CreateVersion(ctx, version); err != nil {
			return fmt.Errorf("create version for %s: %w", name, err)
		}
		logger.Info().Str("name", name).Str("domain", string(dom)).Msg("seed formula created")
		return nil
	}

	// --- Life Insurance: Net Premium Calculation ---
	// Formula: premium = sumAssured * mortalityRate * discountFactor
	// Simplified: premium = sumAssured * qx * v
	// where v = 1 / (1 + interestRate)
	lifeGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "sumAssured", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "qx", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "interestRate", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},       // 1 + interestRate
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "divide"})},    // v = 1 / (1 + interestRate)
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // sumAssured * qx
			{ID: "n8", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // (sumAssured * qx) * v
			{ID: "n9", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "18"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},   // 1 → add.left
			{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},  // interestRate → add.right
			{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "left"},   // 1 → divide.left
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"},  // (1+rate) → divide.right
			{Source: "n1", Target: "n7", SourcePort: "out", TargetPort: "left"},   // sumAssured → mul.left
			{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "right"},  // qx → mul.right
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},   // sumAssured*qx → mul.left
			{Source: "n6", Target: "n8", SourcePort: "out", TargetPort: "right"},  // v → mul.right
			{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "in"},     // result → round
		},
		Outputs: []string{"n9"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50},   "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250},  "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 300}, "n6": {X: 450, Y: 300},
				"n7": {X: 250, Y: 100}, "n8": {X: 550, Y: 150},
				"n9": {X: 700, Y: 150},
			},
		},
	}
	if err := seedFormula("寿险净保费计算", domain.DomainLife,
		"Net premium = sumAssured × qx × v, where v = 1/(1+i). 输入: sumAssured(保额), qx(死亡率), interestRate(预定利率)",
		lifeGraph); err != nil {
		return err
	}

	// --- Property Insurance: Premium Rating ---
	// Formula: premium = baseRate * riskScore * sumInsured * (1 - discount)
	propGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "baseRate", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "riskScore", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "sumInsured", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "discount", "dataType": "decimal"})},
			{ID: "n5", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},  // 1 - discount
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // baseRate * riskScore
			{ID: "n8", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // (baseRate * riskScore) * sumInsured
			{ID: "n9", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // * (1 - discount)
			{ID: "n10", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "2"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "left"},
			{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "right"},
			{Source: "n1", Target: "n7", SourcePort: "out", TargetPort: "left"},
			{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "right"},
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},
			{Source: "n3", Target: "n8", SourcePort: "out", TargetPort: "right"},
			{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "left"},
			{Source: "n6", Target: "n9", SourcePort: "out", TargetPort: "right"},
			{Source: "n9", Target: "n10", SourcePort: "out", TargetPort: "in"},
		},
		Outputs: []string{"n10"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50},   "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250},  "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 350}, "n6": {X: 450, Y: 350},
				"n7": {X: 250, Y: 100}, "n8": {X: 450, Y: 150},
				"n9": {X: 600, Y: 200}, "n10": {X: 750, Y: 200},
			},
		},
	}
	if err := seedFormula("财产险保费计算", domain.DomainProperty,
		"Premium = baseRate × riskScore × sumInsured × (1 - discount). 输入: baseRate(基础费率), riskScore(风险评分), sumInsured(保额), discount(折扣率)",
		propGraph); err != nil {
		return err
	}

	// --- Auto Insurance: Commercial Premium ---
	// Formula: premium = basePremium * vehicleFactor * driverFactor * ncdDiscount
	autoGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "basePremium", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "vehicleFactor", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "driverFactor", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "ncdDiscount", "dataType": "decimal"})},
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // basePremium * vehicleFactor
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // * driverFactor
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},  // * ncdDiscount
			{ID: "n8", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "2"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
			{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "right"},
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "left"},
			{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "right"},
			{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
			{Source: "n4", Target: "n7", SourcePort: "out", TargetPort: "right"},
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
		},
		Outputs: []string{"n8"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50},  "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250}, "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 100}, "n6": {X: 450, Y: 175},
				"n7": {X: 600, Y: 200}, "n8": {X: 750, Y: 200},
			},
		},
	}
	if err := seedFormula("车险商业保费计算", domain.DomainAuto,
		"Premium = basePremium × vehicleFactor × driverFactor × ncdDiscount. 输入: basePremium(基础保费), vehicleFactor(车辆系数), driverFactor(驾驶员系数), ncdDiscount(无赔优惠系数)",
		autoGraph); err != nil {
		return err
	}

	return nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
