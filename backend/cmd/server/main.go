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
	// Returns the formula ID (empty string if already existed) and any error.
	seedFormula := func(name string, dom domain.InsuranceDomain, desc string, graph domain.FormulaGraph) (string, error) {
		// Check by listing and looking for the name.
		formulas, _, _ := s.Formulas().List(ctx, domain.FormulaFilter{Limit: 1000})
		for _, f := range formulas {
			if f.Name == name {
				return f.ID, nil // already exists
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
			return "", fmt.Errorf("create formula %s: %w", name, err)
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
			return "", fmt.Errorf("create version for %s: %w", name, err)
		}
		// Stamp updated_by/updated_at so the seed formula immediately
		// shows the seeding admin as its updater on the list page,
		// matching every other create-formula-and-version flow added
		// by task #042 (Copy, Import, Save Version).
		if err := s.Formulas().UpdateMeta(ctx, formulaID, adminID, now); err != nil {
			logger.Warn().Err(err).Str("name", name).Msg("seed formula meta stamp failed")
		}
		logger.Info().Str("name", name).Str("domain", string(dom)).Msg("seed formula created")
		return formulaID, nil
	}

	// Helper: create a lookup table if it doesn't exist. Returns the table ID.
	seedTable := func(name string, dom domain.InsuranceDomain, tableType string, data json.RawMessage) (string, error) {
		tables, _ := s.Tables().List(ctx, nil)
		for _, t := range tables {
			if t.Name == name {
				return t.ID, nil // already exists
			}
		}
		tableID := uuid.New().String()
		table := &domain.LookupTable{
			ID:        tableID,
			Name:      name,
			Domain:    dom,
			TableType: tableType,
			Data:      data,
			CreatedAt: now,
		}
		if err := s.Tables().Create(ctx, table); err != nil {
			return "", fmt.Errorf("create table %s: %w", name, err)
		}
		logger.Info().Str("name", name).Msg("seed table created")
		return tableID, nil
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
	if _, err := seedFormula("寿险净保费计算", "life",
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
	if _, err := seedFormula("日本生命保険 収支相等純保険料", "life",
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
	if _, err := seedFormula("日本生命保険 粗保険料分解", "life",
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
	if _, err := seedFormula("日本生命保険 責任準備金ロールフォワード", "life",
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
	if _, err := seedFormula("日本生命保険 解約返戻金近似", "life",
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
	if _, err := seedFormula("财产险保费计算", "property",
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
	if _, err := seedFormula("车险商业保费计算", "auto",
		"Premium = basePremium × vehicleFactor × driverFactor × ncdDiscount. 输入: basePremium(基础保费), vehicleFactor(车辆系数), driverFactor(驾驶员系数), ncdDiscount(无赔优惠系数)",
		autoGraph); err != nil {
		return err
	}

	// =====================================================================
	// Actuarial loop formulas with life table
	// =====================================================================

	// --- Life Table ---
	lifeTableData := mustJSON([]map[string]string{
		{"key": "0", "qx": "0.00290"}, {"key": "1", "qx": "0.00040"},
		{"key": "2", "qx": "0.00030"}, {"key": "3", "qx": "0.00020"},
		{"key": "4", "qx": "0.00020"}, {"key": "5", "qx": "0.00015"},
		{"key": "6", "qx": "0.00014"}, {"key": "7", "qx": "0.00013"},
		{"key": "8", "qx": "0.00012"}, {"key": "9", "qx": "0.00011"},
		{"key": "10", "qx": "0.00010"}, {"key": "11", "qx": "0.00012"},
		{"key": "12", "qx": "0.00016"}, {"key": "13", "qx": "0.00019"},
		{"key": "14", "qx": "0.00024"}, {"key": "15", "qx": "0.00030"},
		{"key": "16", "qx": "0.00033"}, {"key": "17", "qx": "0.00037"},
		{"key": "18", "qx": "0.00041"}, {"key": "19", "qx": "0.00045"},
		{"key": "20", "qx": "0.00050"}, {"key": "21", "qx": "0.00050"},
		{"key": "22", "qx": "0.00050"}, {"key": "23", "qx": "0.00050"},
		{"key": "24", "qx": "0.00050"}, {"key": "25", "qx": "0.00050"},
		{"key": "26", "qx": "0.00052"}, {"key": "27", "qx": "0.00054"},
		{"key": "28", "qx": "0.00056"}, {"key": "29", "qx": "0.00058"},
		{"key": "30", "qx": "0.00060"}, {"key": "31", "qx": "0.00064"},
		{"key": "32", "qx": "0.00067"}, {"key": "33", "qx": "0.00071"},
		{"key": "34", "qx": "0.00076"}, {"key": "35", "qx": "0.00080"},
		{"key": "36", "qx": "0.00087"}, {"key": "37", "qx": "0.00094"},
		{"key": "38", "qx": "0.00102"}, {"key": "39", "qx": "0.00111"},
		{"key": "40", "qx": "0.00120"}, {"key": "41", "qx": "0.00133"},
		{"key": "42", "qx": "0.00147"}, {"key": "43", "qx": "0.00163"},
		{"key": "44", "qx": "0.00181"}, {"key": "45", "qx": "0.00200"},
		{"key": "46", "qx": "0.00224"}, {"key": "47", "qx": "0.00250"},
		{"key": "48", "qx": "0.00280"}, {"key": "49", "qx": "0.00313"},
		{"key": "50", "qx": "0.00350"}, {"key": "51", "qx": "0.00383"},
		{"key": "52", "qx": "0.00419"}, {"key": "53", "qx": "0.00459"},
		{"key": "54", "qx": "0.00502"}, {"key": "55", "qx": "0.00550"},
		{"key": "56", "qx": "0.00607"}, {"key": "57", "qx": "0.00670"},
		{"key": "58", "qx": "0.00739"}, {"key": "59", "qx": "0.00816"},
		{"key": "60", "qx": "0.00900"}, {"key": "61", "qx": "0.00997"},
		{"key": "62", "qx": "0.01104"}, {"key": "63", "qx": "0.01223"},
		{"key": "64", "qx": "0.01354"}, {"key": "65", "qx": "0.01500"},
		{"key": "66", "qx": "0.01661"}, {"key": "67", "qx": "0.01840"},
		{"key": "68", "qx": "0.02038"}, {"key": "69", "qx": "0.02257"},
		{"key": "70", "qx": "0.02500"}, {"key": "71", "qx": "0.02773"},
		{"key": "72", "qx": "0.03077"}, {"key": "73", "qx": "0.03413"},
		{"key": "74", "qx": "0.03786"}, {"key": "75", "qx": "0.04200"},
		{"key": "76", "qx": "0.04652"}, {"key": "77", "qx": "0.05152"},
		{"key": "78", "qx": "0.05706"}, {"key": "79", "qx": "0.06320"},
		{"key": "80", "qx": "0.07000"}, {"key": "81", "qx": "0.07797"},
		{"key": "82", "qx": "0.08684"}, {"key": "83", "qx": "0.09673"},
		{"key": "84", "qx": "0.10774"}, {"key": "85", "qx": "0.12000"},
		{"key": "86", "qx": "0.13291"}, {"key": "87", "qx": "0.14720"},
		{"key": "88", "qx": "0.16304"}, {"key": "89", "qx": "0.18058"},
		{"key": "90", "qx": "0.20000"}, {"key": "91", "qx": "0.22107"},
		{"key": "92", "qx": "0.24436"}, {"key": "93", "qx": "0.27010"},
		{"key": "94", "qx": "0.29855"}, {"key": "95", "qx": "0.33000"},
		{"key": "96", "qx": "0.35860"}, {"key": "97", "qx": "0.38967"},
		{"key": "98", "qx": "0.42344"}, {"key": "99", "qx": "0.46013"},
		{"key": "100", "qx": "0.50000"},
	})
	tableID, err := seedTable("日本標準生命表2007（簡易版）", "life", "mortality", lifeTableData)
	if err != nil {
		return err
	}

	// --- Body formula 1: Survival factor 1-q_{x+k} ---
	body1Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_k", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "k", "dataType": "integer"})},
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "op_xk", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "tbl_qx", Type: domain.NodeTableLookup, Config: mustJSON(map[string]any{"tableId": tableID, "keyColumns": []string{"key"}, "column": "qx"})},
			{ID: "op_sub", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_x", Target: "op_xk", SourcePort: "out", TargetPort: "left"},
			{Source: "var_k", Target: "op_xk", SourcePort: "out", TargetPort: "right"},
			{Source: "op_xk", Target: "tbl_qx", SourcePort: "out", TargetPort: "key"},
			{Source: "c_1", Target: "op_sub", SourcePort: "out", TargetPort: "left"},
			{Source: "tbl_qx", Target: "op_sub", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"op_sub"},
	}
	body1ID, err := seedFormula("生存率因子 1-qx", "life",
		"Survival factor: 1 - q_{x+k}. Loop body for computing survival probabilities. 入力: x(年齢), k(イテレータ)",
		body1Graph)
	if err != nil {
		return err
	}

	// --- Body formula 2: Death benefit PV term  v^t * _{t-1}p_x * q_{x+t-1} ---
	body2Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_t", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "t", "dataType": "integer"})},
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "var_v", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "v", "dataType": "decimal"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "c_0", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "0"})},
			{ID: "c_2", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "2"})},
			{ID: "op_pow", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "power"})},
			{ID: "op_tminus1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "op_x_tm1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "tbl_qx", Type: domain.NodeTableLookup, Config: mustJSON(map[string]any{"tableId": tableID, "keyColumns": []string{"key"}, "column": "qx"})},
			{ID: "op_tminus2", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "loop_px", Type: domain.NodeLoop, Config: mustJSON(map[string]any{"mode": "range", "formulaId": body1ID, "iterator": "k", "aggregation": "product"})},
			{ID: "op_mul1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "op_mul2", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_v", Target: "op_pow", SourcePort: "out", TargetPort: "left"},
			{Source: "var_t", Target: "op_pow", SourcePort: "out", TargetPort: "right"},
			{Source: "var_t", Target: "op_tminus1", SourcePort: "out", TargetPort: "left"},
			{Source: "c_1", Target: "op_tminus1", SourcePort: "out", TargetPort: "right"},
			{Source: "var_x", Target: "op_x_tm1", SourcePort: "out", TargetPort: "left"},
			{Source: "op_tminus1", Target: "op_x_tm1", SourcePort: "out", TargetPort: "right"},
			{Source: "op_x_tm1", Target: "tbl_qx", SourcePort: "out", TargetPort: "key"},
			{Source: "var_t", Target: "op_tminus2", SourcePort: "out", TargetPort: "left"},
			{Source: "c_2", Target: "op_tminus2", SourcePort: "out", TargetPort: "right"},
			{Source: "c_0", Target: "loop_px", SourcePort: "out", TargetPort: "start"},
			{Source: "op_tminus2", Target: "loop_px", SourcePort: "out", TargetPort: "end"},
			{Source: "op_pow", Target: "op_mul1", SourcePort: "out", TargetPort: "left"},
			{Source: "loop_px", Target: "op_mul1", SourcePort: "out", TargetPort: "right"},
			{Source: "op_mul1", Target: "op_mul2", SourcePort: "out", TargetPort: "left"},
			{Source: "tbl_qx", Target: "op_mul2", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"op_mul2"},
	}
	body2ID, err := seedFormula("死亡給付PV項", "life",
		"Death benefit PV term: v^t * _{t-1}p_x * q_{x+t-1}. 入力: t(イテレータ), x(年齢), v(割引因子)",
		body2Graph)
	if err != nil {
		return err
	}

	// --- Body formula 3: Annuity PV term  v^t * _tp_x ---
	body3Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_t", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "t", "dataType": "integer"})},
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "var_v", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "v", "dataType": "decimal"})},
			{ID: "c_0", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "0"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "op_pow", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "power"})},
			{ID: "op_tminus1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "loop_px", Type: domain.NodeLoop, Config: mustJSON(map[string]any{"mode": "range", "formulaId": body1ID, "iterator": "k", "aggregation": "product"})},
			{ID: "op_mul", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_v", Target: "op_pow", SourcePort: "out", TargetPort: "left"},
			{Source: "var_t", Target: "op_pow", SourcePort: "out", TargetPort: "right"},
			{Source: "var_t", Target: "op_tminus1", SourcePort: "out", TargetPort: "left"},
			{Source: "c_1", Target: "op_tminus1", SourcePort: "out", TargetPort: "right"},
			{Source: "c_0", Target: "loop_px", SourcePort: "out", TargetPort: "start"},
			{Source: "op_tminus1", Target: "loop_px", SourcePort: "out", TargetPort: "end"},
			{Source: "op_pow", Target: "op_mul", SourcePort: "out", TargetPort: "left"},
			{Source: "loop_px", Target: "op_mul", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"op_mul"},
	}
	body3ID, err := seedFormula("年金現価項", "life",
		"Annuity PV term: v^t * _tp_x. 入力: t(イテレータ), x(年齢), v(割引因子)",
		body3Graph)
	if err != nil {
		return err
	}

	// --- Body formula 4: Reserve step  (V+P)*(1+i) - S*q_{x+t} ---
	body4Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_V", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "V", "dataType": "decimal"})},
			{ID: "var_t", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "t", "dataType": "integer"})},
			{ID: "var_P", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "P", "dataType": "decimal"})},
			{ID: "var_i", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "i", "dataType": "decimal"})},
			{ID: "var_S", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "S", "dataType": "decimal"})},
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "op_vp", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "op_1i", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "op_grow", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "op_xt", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "add"})},
			{ID: "tbl_qx", Type: domain.NodeTableLookup, Config: mustJSON(map[string]any{"tableId": tableID, "keyColumns": []string{"key"}, "column": "qx"})},
			{ID: "op_sq", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
			{ID: "op_result", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_V", Target: "op_vp", SourcePort: "out", TargetPort: "left"},
			{Source: "var_P", Target: "op_vp", SourcePort: "out", TargetPort: "right"},
			{Source: "c_1", Target: "op_1i", SourcePort: "out", TargetPort: "left"},
			{Source: "var_i", Target: "op_1i", SourcePort: "out", TargetPort: "right"},
			{Source: "op_vp", Target: "op_grow", SourcePort: "out", TargetPort: "left"},
			{Source: "op_1i", Target: "op_grow", SourcePort: "out", TargetPort: "right"},
			{Source: "var_x", Target: "op_xt", SourcePort: "out", TargetPort: "left"},
			{Source: "var_t", Target: "op_xt", SourcePort: "out", TargetPort: "right"},
			{Source: "op_xt", Target: "tbl_qx", SourcePort: "out", TargetPort: "key"},
			{Source: "var_S", Target: "op_sq", SourcePort: "out", TargetPort: "left"},
			{Source: "tbl_qx", Target: "op_sq", SourcePort: "out", TargetPort: "right"},
			{Source: "op_grow", Target: "op_result", SourcePort: "out", TargetPort: "left"},
			{Source: "op_sq", Target: "op_result", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"op_result"},
	}
	body4ID, err := seedFormula("責任準備金ステップ", "life",
		"Reserve recursion step: (V+P)*(1+i) - S*q_{x+t}. 入力: V(前期準備金), t(イテレータ), P(保険料), i(予定利率), S(保険金額), x(年齢)",
		body4Graph)
	if err != nil {
		return err
	}

	// --- Main formula 1: Pure premium  S * Σ v^t * _{t-1}p_x * q_{x+t-1} ---
	main1Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_S", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "S", "dataType": "decimal"})},
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "var_n", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "n", "dataType": "integer"})},
			{ID: "var_v", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "v", "dataType": "decimal"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "loop_sum", Type: domain.NodeLoop, Config: mustJSON(map[string]any{"mode": "range", "formulaId": body2ID, "iterator": "t", "aggregation": "sum"})},
			{ID: "op_result", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "multiply"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "c_1", Target: "loop_sum", SourcePort: "out", TargetPort: "start"},
			{Source: "var_n", Target: "loop_sum", SourcePort: "out", TargetPort: "end"},
			{Source: "var_S", Target: "op_result", SourcePort: "out", TargetPort: "left"},
			{Source: "loop_sum", Target: "op_result", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"op_result"},
	}
	if _, err := seedFormula("定期保険一時払純保険料", "life",
		"Term life single pure premium: S * Σ_{t=1}^{n} v^t * _{t-1}p_x * q_{x+t-1}. 入力: S(保険金額), x(年齢), n(保険期間), v(割引因子)",
		main1Graph); err != nil {
		return err
	}

	// --- Main formula 2: Annuity due  ä_{x:n} = Σ_{t=0}^{n-1} v^t * _tp_x ---
	main2Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "var_n", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "n", "dataType": "integer"})},
			{ID: "var_v", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "v", "dataType": "decimal"})},
			{ID: "c_0", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "0"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "op_nm1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "loop_sum", Type: domain.NodeLoop, Config: mustJSON(map[string]any{"mode": "range", "formulaId": body3ID, "iterator": "t", "aggregation": "sum"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_n", Target: "op_nm1", SourcePort: "out", TargetPort: "left"},
			{Source: "c_1", Target: "op_nm1", SourcePort: "out", TargetPort: "right"},
			{Source: "c_0", Target: "loop_sum", SourcePort: "out", TargetPort: "start"},
			{Source: "op_nm1", Target: "loop_sum", SourcePort: "out", TargetPort: "end"},
		},
		Outputs: []string{"loop_sum"},
	}
	if _, err := seedFormula("期始払年金現価", "life",
		"Annuity-due present value: ä_{x:n} = Σ_{t=0}^{n-1} v^t * _tp_x. 入力: x(年齢), n(保険期間), v(割引因子)",
		main2Graph); err != nil {
		return err
	}

	// --- Main formula 3: Reserve via fold  _tV = fold (V+P)*(1+i) - S*q_{x+t} ---
	main3Graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "var_x", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "x", "dataType": "integer"})},
			{ID: "var_n", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "n", "dataType": "integer"})},
			{ID: "var_P", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "P", "dataType": "decimal"})},
			{ID: "var_i", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "i", "dataType": "decimal"})},
			{ID: "var_S", Type: domain.NodeVariable, Config: mustJSON(map[string]any{"name": "S", "dataType": "decimal"})},
			{ID: "c_0", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "0"})},
			{ID: "c_1", Type: domain.NodeConstant, Config: mustJSON(map[string]any{"value": "1"})},
			{ID: "op_nm1", Type: domain.NodeOperator, Config: mustJSON(map[string]any{"op": "subtract"})},
			{ID: "loop_fold", Type: domain.NodeLoop, Config: mustJSON(map[string]any{"mode": "range", "formulaId": body4ID, "iterator": "t", "aggregation": "fold", "accumulatorVar": "V", "initValue": "0"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "var_n", Target: "op_nm1", SourcePort: "out", TargetPort: "left"},
			{Source: "c_1", Target: "op_nm1", SourcePort: "out", TargetPort: "right"},
			{Source: "c_0", Target: "loop_fold", SourcePort: "out", TargetPort: "start"},
			{Source: "op_nm1", Target: "loop_fold", SourcePort: "out", TargetPort: "end"},
		},
		Outputs: []string{"loop_fold"},
	}
	if _, err := seedFormula("漸化式責任準備金", "life",
		"Recursive reserve: fold_{t=0}^{n-1} (V+P)*(1+i) - S*q_{x+t}. 入力: x(年齢), n(保険期間), P(保険料), i(予定利率), S(保険金額)",
		main3Graph); err != nil {
		return err
	}

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

// seedFormulaNames is the list of formula names created by the seed function.
// Used to identify seed data for reset operations.
var seedFormulaNames = []string{
	"寿险净保费计算",
	"日本生命保険 収支相等純保険料",
	"日本生命保険 粗保険料分解",
	"日本生命保険 責任準備金ロールフォワード",
	"日本生命保険 解約返戻金近似",
	"财产险保费计算",
	"车险商业保费计算",
	// Actuarial loop formulas (seed names)
	"生存率因子 1-qx",
	"死亡給付PV項",
	"年金現価項",
	"責任準備金ステップ",
	"定期保険一時払純保険料",
	"期始払年金現価",
	"漸化式責任準備金",
	// Legacy names (API-created before seed integration)
	"純保険料（一時払）",
	"年金現価",
	"責任準備金",
	// Demo formulas
	"平方和 ∑t² (t=1..n)",
	"阶乘 n! = ∏t (t=1..n)",
	"Square (t²)",
	"Identity (t)",
}

// seedTableNames is the list of table names created by API-based seed scripts.
var seedTableNames = []string{
	"日本標準生命表2007（簡易版）",
}

// makeSeedResetHandler returns an http.HandlerFunc that deletes seed formulas
// and tables, then re-runs the seed function. User-created data is not affected.
func makeSeedResetHandler(s store.Store, logger zerolog.Logger) http.HandlerFunc {
	seedNames := make(map[string]bool, len(seedFormulaNames))
	for _, n := range seedFormulaNames {
		seedNames[n] = true
	}
	tableNames := make(map[string]bool, len(seedTableNames))
	for _, n := range seedTableNames {
		tableNames[n] = true
	}

	// The seed admin account ID is used as provenance check.
	const seedAdminID = "00000000-0000-0000-0000-000000000001"

	return func(w http.ResponseWriter, r *http.Request) {
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

		logger.Info().Int("formulas", deleted).Int("tables", tablesDeleted).Msg("seed data deleted")

		// Re-run seed.
		if err := seed(r.Context(), s, logger); err != nil {
			logger.Error().Err(err).Msg("seed reset failed during re-seed")
			http.Error(w, `{"error":"seed reset failed","code":500}`, http.StatusInternalServerError)
			return
		}

		logger.Info().Msg("seed data reset complete")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"seed data reset","formulasDeleted":%d,"tablesDeleted":%d}`, deleted, tablesDeleted)
	}
}
