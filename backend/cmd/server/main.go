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
		Formulas:   store.Formulas(),
		Categories: store.Categories(),
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
		Tables:     store.Tables(),
		Categories: store.Categories(),
	}
	userHandler := &api.UserHandler{
		Users: store.Users(),
	}
	categoryHandler := api.NewCategoryHandler(store.Categories(), store.Formulas(), store.Tables())
	parseHandler := &api.ParseHandler{}

	// Build the router.
	router := api.NewRouter(api.RouterConfig{
		AuthHandler:     authHandler,
		FormulaHandler:  formulaHandler,
		VersionHandler:  versionHandler,
		CalcHandler:     calcHandler,
		TableHandler:    tableHandler,
		UserHandler:     userHandler,
		CategoryHandler: categoryHandler,
		ParseHandler:    parseHandler,
		JWTManager:      jwtMgr,
		Logger:          logger,
		CORSOrigins:     cfg.Server.CORSOrigins,
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

	// --- Default categories ---
	defaultCategories := []struct {
		Slug  string
		Name  string
		Color string
		Order int
	}{
		{"life", "人寿保险", "#3b82f6", 1},
		{"property", "财产保险", "#10b981", 2},
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
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},      // 1 + interestRate
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "divide"})},   // v = 1 / (1 + interestRate)
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // sumAssured * qx
			{ID: "n8", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // (sumAssured * qx) * v
			{ID: "n9", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "18"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},  // 1 → add.left
			{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"}, // interestRate → add.right
			{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "left"},  // 1 → divide.left
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"}, // (1+rate) → divide.right
			{Source: "n1", Target: "n7", SourcePort: "out", TargetPort: "left"},  // sumAssured → mul.left
			{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "right"}, // qx → mul.right
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},  // sumAssured*qx → mul.left
			{Source: "n6", Target: "n8", SourcePort: "out", TargetPort: "right"}, // v → mul.right
			{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "in"},    // result → round
		},
		Outputs: []string{"n9"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250}, "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 300}, "n6": {X: 450, Y: 300},
				"n7": {X: 250, Y: 100}, "n8": {X: 550, Y: 150},
				"n9": {X: 700, Y: 150},
			},
		},
	}
	if err := seedFormula("寿险净保费计算", "life",
		"Net premium = sumAssured × qx × v, where v = 1/(1+i). 输入: sumAssured(保额), qx(死亡率), interestRate(预定利率)",
		lifeGraph); err != nil {
		return err
	}

	// --- Japan Life Insurance: Equivalence Principle Premium ---
	// Formula: basePremium = deathBenefit * expectedDeaths / policyCount
	japanEquivalenceGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "deathBenefit", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "expectedDeaths", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "policyCount", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "divide"})},
			{ID: "n6", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "2"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
			{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
			{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
			{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "in"},
		},
		Outputs: []string{"n6"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 60, Y: 60},
				"n2": {X: 60, Y: 180},
				"n3": {X: 60, Y: 300},
				"n4": {X: 300, Y: 120},
				"n5": {X: 540, Y: 180},
				"n6": {X: 780, Y: 180},
			},
		},
	}
	if err := seedFormula("日本生命保険 収支相等純保険料", "life",
		"Pure premium approximation under the equivalence principle. 输入: deathBenefit(保険金額), expectedDeaths(想定死亡件数), policyCount(契約件数)",
		japanEquivalenceGraph); err != nil {
		return err
	}

	// --- Japan Life Insurance: Gross Premium Decomposition ---
	// Formula: grossPremium = netPremium + acquisitionExpense + collectionExpense + maintenanceExpense
	japanGrossPremiumGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "netPremium", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "acquisitionExpense", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "collectionExpense", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "maintenanceExpense", "dataType": "decimal"})},
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
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
				"n1": {X: 60, Y: 60},
				"n2": {X: 60, Y: 180},
				"n3": {X: 60, Y: 300},
				"n4": {X: 60, Y: 420},
				"n5": {X: 320, Y: 120},
				"n6": {X: 560, Y: 210},
				"n7": {X: 800, Y: 255},
				"n8": {X: 1040, Y: 255},
			},
		},
	}
	if err := seedFormula("日本生命保険 粗保険料分解", "life",
		"Gross premium decomposition based on net premium plus expense loadings. 输入: netPremium(純保険料), acquisitionExpense(新契約費), collectionExpense(集金費), maintenanceExpense(維持費)",
		japanGrossPremiumGraph); err != nil {
		return err
	}

	// --- Japan Life Insurance: Reserve Roll-Forward Approximation ---
	// Formula: reserveEnd = reserveBegin * (1 + assumedInterestRate) + levelPremium - expectedBenefit - maintenanceExpense
	japanReserveGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "reserveBegin", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "assumedInterestRate", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "levelPremium", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "expectedBenefit", "dataType": "decimal"})},
			{ID: "n5", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "maintenanceExpense", "dataType": "decimal"})},
			{ID: "n6", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "n8", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "n9", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "n10", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "n11", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "n12", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "2"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
			{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "right"},
			{Source: "n1", Target: "n8", SourcePort: "out", TargetPort: "left"},
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "right"},
			{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "left"},
			{Source: "n3", Target: "n9", SourcePort: "out", TargetPort: "right"},
			{Source: "n9", Target: "n10", SourcePort: "out", TargetPort: "left"},
			{Source: "n4", Target: "n10", SourcePort: "out", TargetPort: "right"},
			{Source: "n10", Target: "n11", SourcePort: "out", TargetPort: "left"},
			{Source: "n5", Target: "n11", SourcePort: "out", TargetPort: "right"},
			{Source: "n11", Target: "n12", SourcePort: "out", TargetPort: "in"},
		},
		Outputs: []string{"n12"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1":  {X: 60, Y: 60},
				"n2":  {X: 60, Y: 180},
				"n3":  {X: 60, Y: 300},
				"n4":  {X: 60, Y: 420},
				"n5":  {X: 60, Y: 540},
				"n6":  {X: 300, Y: 180},
				"n7":  {X: 540, Y: 180},
				"n8":  {X: 780, Y: 120},
				"n9":  {X: 1020, Y: 210},
				"n10": {X: 1260, Y: 255},
				"n11": {X: 1500, Y: 300},
				"n12": {X: 1740, Y: 300},
			},
		},
	}
	if err := seedFormula("日本生命保険 責任準備金ロールフォワード", "life",
		"Reserve roll-forward approximation using level premium accumulation. 输入: reserveBegin(期初責任準備金), assumedInterestRate(予定利率), levelPremium(平準保険料), expectedBenefit(想定保険金), maintenanceExpense(維持費)",
		japanReserveGraph); err != nil {
		return err
	}

	// --- Japan Life Insurance: Surrender Value Approximation ---
	// Formula: surrenderValue = max(netPremiumReserve - deathBenefit * surrenderChargeRate, 0)
	japanSurrenderGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "n1", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "netPremiumReserve", "dataType": "decimal"})},
			{ID: "n2", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "deathBenefit", "dataType": "decimal"})},
			{ID: "n3", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "surrenderChargeRate", "dataType": "decimal"})},
			{ID: "n4", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "0"})},
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "n7", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "max", "args": map[string]string{}})},
			{ID: "n8", Type: domain.NodeFunction, Config: mustJSON(map[string]any{"fn": "round", "args": map[string]string{"places": "2"}})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "left"},
			{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			{Source: "n1", Target: "n6", SourcePort: "out", TargetPort: "left"},
			{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"},
			{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
			{Source: "n4", Target: "n7", SourcePort: "out", TargetPort: "right"},
			{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
		},
		Outputs: []string{"n8"},
		Layout: &domain.GraphLayout{
			Positions: map[string]domain.Position{
				"n1": {X: 60, Y: 60},
				"n2": {X: 60, Y: 180},
				"n3": {X: 60, Y: 300},
				"n4": {X: 300, Y: 300},
				"n5": {X: 360, Y: 210},
				"n6": {X: 600, Y: 135},
				"n7": {X: 840, Y: 180},
				"n8": {X: 1080, Y: 180},
			},
		},
	}
	if err := seedFormula("日本生命保険 解約返戻金近似", "life",
		"Surrender value approximation using reserve less a surrender charge amount, floored at zero. 输入: netPremiumReserve(純保険料式保険料積立金), deathBenefit(保険金額), surrenderChargeRate(控除率)",
		japanSurrenderGraph); err != nil {
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
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})}, // 1 - discount
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // baseRate * riskScore
			{ID: "n8", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // (baseRate * riskScore) * sumInsured
			{ID: "n9", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // * (1 - discount)
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
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250}, "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 350}, "n6": {X: 450, Y: 350},
				"n7": {X: 250, Y: 100}, "n8": {X: 450, Y: 150},
				"n9": {X: 600, Y: 200}, "n10": {X: 750, Y: 200},
			},
		},
	}
	if err := seedFormula("财产险保费计算", "property",
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
			{ID: "n5", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // basePremium * vehicleFactor
			{ID: "n6", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // * driverFactor
			{ID: "n7", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})}, // * ncdDiscount
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
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 150},
				"n3": {X: 50, Y: 250}, "n4": {X: 50, Y: 350},
				"n5": {X: 250, Y: 100}, "n6": {X: 450, Y: 175},
				"n7": {X: 600, Y: 200}, "n8": {X: 750, Y: 200},
			},
		},
	}
	if err := seedFormula("车险商业保费计算", "auto",
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
