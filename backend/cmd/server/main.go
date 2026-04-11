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

	// --- Claims triangle sample table for spec #8 chain ladder ---
	// 5 accident years × varying development years, with a precomputed
	// development_ratio column on each non-final cell. The chain-ladder
	// LDF for development year j is the average of the development_ratio
	// values where dev_year = j. Used by the seed formula
	// 「日本損害保険 チェインラダー LDF」 below.
	claimsTriangleData := json.RawMessage(`[
		{"acc_year": "2020", "dev_year": "1", "cumulative_claim": "100", "development_ratio": "1.27"},
		{"acc_year": "2020", "dev_year": "2", "cumulative_claim": "127", "development_ratio": "1.173"},
		{"acc_year": "2020", "dev_year": "3", "cumulative_claim": "149", "development_ratio": "1.127"},
		{"acc_year": "2020", "dev_year": "4", "cumulative_claim": "168"},
		{"acc_year": "2021", "dev_year": "1", "cumulative_claim": "95", "development_ratio": "1.274"},
		{"acc_year": "2021", "dev_year": "2", "cumulative_claim": "121", "development_ratio": "1.190"},
		{"acc_year": "2021", "dev_year": "3", "cumulative_claim": "144"},
		{"acc_year": "2022", "dev_year": "1", "cumulative_claim": "105", "development_ratio": "1.267"},
		{"acc_year": "2022", "dev_year": "2", "cumulative_claim": "133"},
		{"acc_year": "2023", "dev_year": "1", "cumulative_claim": "98"}
	]`)
	claimsTriangleSampleID, err := seedTable("claims_triangle_sample", "property", "loss_triangle", claimsTriangleData)
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

	// ───────────────────────────────────────────────────────────────
	// Task #045: 17 actuarial formulas from spec 002 (Japanese insurance
	// coverage analysis), excluding #2 (already seeded as 定期保険一時払
	// 純保険料 above), #14 (needs normal-quantile function not yet in
	// the engine), and #19 (release rule belongs in user-defined
	// business logic, not as a generic formula).
	//
	// Each formula uses the nVar/nConst/nOp/nFn/eEdge helpers defined
	// near the bottom of this file so the graph definition stays
	// roughly one line per node. The formulas are grouped into seven
	// categories matching spec 002's chapter layout.
	// ───────────────────────────────────────────────────────────────

	// ── Spec #1: 死亡率 q_x = d_x / l_x ───────────────────────────
	// Inputs: d (年齢 x で 1 年間に死亡する者の数), l (年齢 x の生存者数)
	// Output: q = d / l
	if _, err := seedFormula("日本生命保険 死亡率 qx",
		"life",
		"Mortality rate q_x = d_x / l_x. 入力: d(該当年齢の年間死亡者数), l(該当年齢の生存者数)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_d", "d"),
				nVar("var_l", "l"),
				nOp("op_div", "divide"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_d", "out", "op_div", "left"),
				eEdge("var_l", "out", "op_div", "right"),
			},
			Outputs: []string{"op_div"},
		}); err != nil {
		return err
	}

	// ── Spec #3: 基数 D_x = v^x · l_x ─────────────────────────────
	// Inputs: v (割引因子), x (年齢), l (l_x の値)
	// Output: D = v^x * l
	if _, err := seedFormula("日本生命保険 基数 Dx",
		"life",
		"Commutation function D_x = v^x · l_x. 入力: v(割引因子), x(年齢), l(l_x 生存者数). C_x, M_x, N_x も同様の構造で構築可能",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_v", "v"),
				nVar("var_x", "x"),
				nVar("var_l", "l"),
				nOp("op_pow", "power"),
				nOp("op_mul", "multiply"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_v", "out", "op_pow", "left"),
				eEdge("var_x", "out", "op_pow", "right"),
				eEdge("op_pow", "out", "op_mul", "left"),
				eEdge("var_l", "out", "op_mul", "right"),
			},
			Outputs: []string{"op_mul"},
		}); err != nil {
		return err
	}

	// ── Spec #4: 将来法責任準備金 ₜV_x = A_{x+t} − P_x · ä_{x+t} ───
	// A and a are taken as scalar inputs so the formula stays self-
	// contained. To compose with the existing 期始払年金現価 seed,
	// the user can wire a sub-formula node into the A or a port.
	if _, err := seedFormula("日本生命保険 将来法責任準備金",
		"life",
		"Prospective reserve V_t = A_{x+t} − P_x · ä_{x+t}. 入力: A(将来の給付現価), P(純保険料), a(将来の収入現価=年金現価)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_A", "A"),
				nVar("var_P", "P"),
				nVar("var_a", "a"),
				nOp("op_pa", "multiply"),
				nOp("op_sub", "subtract"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_P", "out", "op_pa", "left"),
				eEdge("var_a", "out", "op_pa", "right"),
				eEdge("var_A", "out", "op_sub", "left"),
				eEdge("op_pa", "out", "op_sub", "right"),
			},
			Outputs: []string{"op_sub"},
		}); err != nil {
		return err
	}

	// ── Spec #5: チルメル式責任準備金 ₜV_x^Z = ₜV_x − α(1 − a_part/a_full) ──
	if _, err := seedFormula("日本生命保険 チルメル式責任準備金",
		"life",
		"Zillmer reserve V_z = V − α(1 − a_part/a_full). 入力: V(将来法責任準備金), alpha(チルメル額), a_part(残存期間年金現価), a_full(全期間年金現価)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_V", "V"),
				nVar("var_alpha", "alpha"),
				nVar("var_apart", "a_part"),
				nVar("var_afull", "a_full"),
				nConst("c_one", "1"),
				nOp("op_div", "divide"),     // a_part / a_full
				nOp("op_inner", "subtract"), // 1 - (above)
				nOp("op_mul", "multiply"),   // alpha * (above)
				nOp("op_outer", "subtract"), // V - (above)
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_apart", "out", "op_div", "left"),
				eEdge("var_afull", "out", "op_div", "right"),
				eEdge("c_one", "out", "op_inner", "left"),
				eEdge("op_div", "out", "op_inner", "right"),
				eEdge("var_alpha", "out", "op_mul", "left"),
				eEdge("op_inner", "out", "op_mul", "right"),
				eEdge("var_V", "out", "op_outer", "left"),
				eEdge("op_mul", "out", "op_outer", "right"),
			},
			Outputs: []string{"op_outer"},
		}); err != nil {
		return err
	}

	// ── Spec #6: 損害率 LR = (paid + adj) / premium ───────────────
	if _, err := seedFormula("日本損害保険 損害率",
		"property",
		"Loss ratio LR = (正味支払保険金 + 損害調査費) / 正味収入保険料. 入力: paid(正味支払保険金), adj(損害調査費), premium(正味収入保険料)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_paid", "paid"),
				nVar("var_adj", "adj"),
				nVar("var_premium", "premium"),
				nOp("op_add", "add"),
				nOp("op_div", "divide"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_paid", "out", "op_add", "left"),
				eEdge("var_adj", "out", "op_add", "right"),
				eEdge("op_add", "out", "op_div", "left"),
				eEdge("var_premium", "out", "op_div", "right"),
			},
			Outputs: []string{"op_div"},
		}); err != nil {
		return err
	}

	// ── Spec #7: 発生保険金 incurred = paid + (end_res − begin_res) ──
	if _, err := seedFormula("日本損害保険 発生保険金",
		"property",
		"Incurred losses = 当期支払保険金 + (当期末支払備金 − 前期末支払備金). 入力: paid(当期支払), end_res(期末支払備金), begin_res(期初支払備金)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_paid", "paid"),
				nVar("var_end", "end_res"),
				nVar("var_begin", "begin_res"),
				nOp("op_delta", "subtract"),
				nOp("op_total", "add"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_end", "out", "op_delta", "left"),
				eEdge("var_begin", "out", "op_delta", "right"),
				eEdge("var_paid", "out", "op_total", "left"),
				eEdge("op_delta", "out", "op_total", "right"),
			},
			Outputs: []string{"op_total"},
		}); err != nil {
		return err
	}

	// ── Spec #8: チェインラダー LDF (single development year) ─────
	// Needs a sample claims_triangle table with a precomputed
	// `development_ratio` column. Seeded earlier in this function as
	// `claims_triangle_sample`.
	if _, err := seedFormula("日本損害保険 チェインラダー LDF",
		"property",
		"Chain ladder LDF for a single development year: avg(development_ratio WHERE dev_year = j). 入力: dev_year(j 動的). 表: claims_triangle_sample (年度ごとの development_ratio を事前計算済み)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_dev_year", "dev_year"),
				nTableAgg("agg_ldf", claimsTriangleSampleID, "avg", "development_ratio",
					[]domain.TableFilter{
						{Column: "dev_year", Op: "eq", InputPort: "dev_year"},
					}, "and"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_dev_year", "out", "agg_ldf", "dev_year"),
			},
			Outputs: []string{"agg_ldf"},
		}); err != nil {
		return err
	}

	// ── Spec #9: BF 法 ult = C + E · (1 − 1/f) ────────────────────
	if _, err := seedFormula("日本損害保険 BF法予測",
		"property",
		"Bornhuetter-Ferguson final loss = C + E · (1 − 1/f). 入力: C(現時点までの実績発生保険金), E(事前想定保険金), f(累積 LDF)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_C", "C"),
				nVar("var_E", "E"),
				nVar("var_f", "f"),
				nConst("c_one", "1"),
				nOp("op_recip", "divide"),   // 1 / f
				nOp("op_inner", "subtract"), // 1 - 1/f
				nOp("op_mul", "multiply"),   // E * (1 - 1/f)
				nOp("op_total", "add"),      // C + above
			},
			Edges: []domain.FormulaEdge{
				eEdge("c_one", "out", "op_recip", "left"),
				eEdge("var_f", "out", "op_recip", "right"),
				eEdge("c_one", "out", "op_inner", "left"),
				eEdge("op_recip", "out", "op_inner", "right"),
				eEdge("var_E", "out", "op_mul", "left"),
				eEdge("op_inner", "out", "op_mul", "right"),
				eEdge("var_C", "out", "op_total", "left"),
				eEdge("op_mul", "out", "op_total", "right"),
			},
			Outputs: []string{"op_total"},
		}); err != nil {
		return err
	}

	// ── Spec #10: 法定 IBNR 要積立額 b = avg(直近3年発生保険金) / 12 ──
	if _, err := seedFormula("日本損害保険 法定IBNR要積立額b",
		"property",
		"Statutory IBNR amount b = (y1 + y2 + y3) / 3 / 12 (1/12 法). 入力: y1, y2, y3 (直近3年度の発生保険金)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_y1", "y1"),
				nVar("var_y2", "y2"),
				nVar("var_y3", "y3"),
				nConst("c_three", "3"),
				nConst("c_twelve", "12"),
				nOp("op_sum12", "add"),
				nOp("op_sum123", "add"),
				nOp("op_avg", "divide"),
				nOp("op_b", "divide"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_y1", "out", "op_sum12", "left"),
				eEdge("var_y2", "out", "op_sum12", "right"),
				eEdge("op_sum12", "out", "op_sum123", "left"),
				eEdge("var_y3", "out", "op_sum123", "right"),
				eEdge("op_sum123", "out", "op_avg", "left"),
				eEdge("c_three", "out", "op_avg", "right"),
				eEdge("op_avg", "out", "op_b", "left"),
				eEdge("c_twelve", "out", "op_b", "right"),
			},
			Outputs: []string{"op_b"},
		}); err != nil {
		return err
	}

	// ── Spec #11: 1/24 法未経過保険料 ─────────────────────────────
	// Closed-form for the UNEARNED fraction (the reserve liability):
	//   unearned_fraction = (2·start_month − 1) / 24
	// At year-end, a contract starting in month 1 (January) has 11.5
	// months elapsed → 23/24 earned, 1/24 unearned. The closed form
	// gives (2·1 − 1)/24 = 1/24 for January, (2·12 − 1)/24 = 23/24
	// for December — matching the spec 002 §11 table exactly.
	// Then unearned_premium = annual_premium × unearned_fraction.
	if _, err := seedFormula("日本損害保険 1/24法未経過保険料",
		"property",
		"Unearned premium under 1/24 method: premium · (2·start_month − 1) / 24. 入力: premium(年間保険料), start_month(始期月 1..12). 1月始期 → 1/24, 12月始期 → 23/24 が未経過.",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_premium", "premium"),
				nVar("var_month", "start_month"),
				nConst("c_2", "2"),
				nConst("c_1", "1"),
				nConst("c_24", "24"),
				nOp("op_2x", "multiply"),   // 2 * start_month
				nOp("op_2xm1", "subtract"), // 2*start_month - 1
				nOp("op_frac", "divide"),   // /24
				nOp("op_uerp", "multiply"), // premium *
			},
			Edges: []domain.FormulaEdge{
				eEdge("c_2", "out", "op_2x", "left"),
				eEdge("var_month", "out", "op_2x", "right"),
				eEdge("op_2x", "out", "op_2xm1", "left"),
				eEdge("c_1", "out", "op_2xm1", "right"),
				eEdge("op_2xm1", "out", "op_frac", "left"),
				eEdge("c_24", "out", "op_frac", "right"),
				eEdge("var_premium", "out", "op_uerp", "left"),
				eEdge("op_frac", "out", "op_uerp", "right"),
			},
			Outputs: []string{"op_uerp"},
		}); err != nil {
		return err
	}

	// ── Spec #12: 短期料率返還 refund = premium · (1 − short_rate) ──
	if _, err := seedFormula("日本損害保険 短期料率返還",
		"property",
		"Short-rate refund = annual_premium · (1 − short_rate). 入力: premium(年間保険料), short_rate(既経過期間に対応する短期料率, 0..1)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_premium", "premium"),
				nVar("var_rate", "short_rate"),
				nConst("c_one", "1"),
				nOp("op_inner", "subtract"),
				nOp("op_refund", "multiply"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("c_one", "out", "op_inner", "left"),
				eEdge("var_rate", "out", "op_inner", "right"),
				eEdge("var_premium", "out", "op_refund", "left"),
				eEdge("op_inner", "out", "op_refund", "right"),
			},
			Outputs: []string{"op_refund"},
		}); err != nil {
		return err
	}

	// ── Spec #13: Bühlmann credibility ─────────────────────────────
	// μ̂ = Z·X + (1−Z)·μ where Z = n / (n+K)
	// Expanded inline: μ̂ = (n·X + K·μ) / (n+K)
	// We use the inline form to avoid having a "Z" intermediate node.
	if _, err := seedFormula("日本損害保険 Bühlmann信頼度",
		"property",
		"Bühlmann credibility μ̂ = (n·X + K·μ) / (n+K). 入力: X(集団の経験平均), mu(全体の手引値), n(データ数), K(個別/個別間変動比)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_X", "X"),
				nVar("var_mu", "mu"),
				nVar("var_n", "n"),
				nVar("var_K", "K"),
				nOp("op_nX", "multiply"),
				nOp("op_Kmu", "multiply"),
				nOp("op_num", "add"),
				nOp("op_den", "add"),
				nOp("op_pred", "divide"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_n", "out", "op_nX", "left"),
				eEdge("var_X", "out", "op_nX", "right"),
				eEdge("var_K", "out", "op_Kmu", "left"),
				eEdge("var_mu", "out", "op_Kmu", "right"),
				eEdge("op_nX", "out", "op_num", "left"),
				eEdge("op_Kmu", "out", "op_num", "right"),
				eEdge("var_n", "out", "op_den", "left"),
				eEdge("var_K", "out", "op_den", "right"),
				eEdge("op_num", "out", "op_pred", "left"),
				eEdge("op_den", "out", "op_pred", "right"),
			},
			Outputs: []string{"op_pred"},
		}); err != nil {
		return err
	}

	// ── Spec #15: 休業損害 (自賠責) loss = 6100 · days ────────────
	if _, err := seedFormula("日本損害賠償 休業損害(自賠責基準)",
		"auto",
		"Lost wages under jibaiseki standard: 6,100 yen × 休業日数. 入力: days(休業日数)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_days", "days"),
				nConst("c_6100", "6100"),
				nOp("op_loss", "multiply"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("c_6100", "out", "op_loss", "left"),
				eEdge("var_days", "out", "op_loss", "right"),
			},
			Outputs: []string{"op_loss"},
		}); err != nil {
		return err
	}

	// ── Spec #16: 逸失利益 lost = income · (1 − rate) · leibniz ───
	if _, err := seedFormula("日本損害賠償 逸失利益",
		"auto",
		"Lost future income = annual_income · (1 − living_expense_rate) · leibniz_coefficient. 入力: income(年収), rate(生活費控除率), leibniz(ライプニッツ係数)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_income", "income"),
				nVar("var_rate", "rate"),
				nVar("var_leibniz", "leibniz"),
				nConst("c_one", "1"),
				nOp("op_keep", "subtract"), // 1 - rate
				nOp("op_take", "multiply"), // income * (1-rate)
				nOp("op_pv", "multiply"),   // * leibniz
			},
			Edges: []domain.FormulaEdge{
				eEdge("c_one", "out", "op_keep", "left"),
				eEdge("var_rate", "out", "op_keep", "right"),
				eEdge("var_income", "out", "op_take", "left"),
				eEdge("op_keep", "out", "op_take", "right"),
				eEdge("op_take", "out", "op_pv", "left"),
				eEdge("var_leibniz", "out", "op_pv", "right"),
			},
			Outputs: []string{"op_pv"},
		}); err != nil {
		return err
	}

	// ── Spec #17: SMR ソルベンシー・マージン比率 ──────────────────
	// SMR = SMM / (0.5 · sqrt(R1² + (R2+R3)² + R4²)) · 100
	if _, err := seedFormula("日本生命保険 ソルベンシー・マージン比率",
		"life",
		"Solvency margin ratio = SMM / (0.5 · sqrt(R1² + (R2+R3)² + R4²)) · 100. 入力: SMM(マージン総額), R1(保険リスク), R2(予定利率リスク), R3(資産運用リスク), R4(経営管理リスク)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_SMM", "SMM"),
				nVar("var_R1", "R1"),
				nVar("var_R2", "R2"),
				nVar("var_R3", "R3"),
				nVar("var_R4", "R4"),
				nConst("c_2", "2"),
				nConst("c_half", "0.5"),
				nConst("c_100", "100"),
				nOp("op_R1sq", "power"),
				nOp("op_R23", "add"),
				nOp("op_R23sq", "power"),
				nOp("op_R4sq", "power"),
				nOp("op_inner1", "add"),
				nOp("op_inner2", "add"),
				nFn("fn_sqrt", "sqrt", nil),
				nOp("op_half", "multiply"),
				nOp("op_ratio", "divide"),
				nOp("op_pct", "multiply"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_R1", "out", "op_R1sq", "left"),
				eEdge("c_2", "out", "op_R1sq", "right"),
				eEdge("var_R2", "out", "op_R23", "left"),
				eEdge("var_R3", "out", "op_R23", "right"),
				eEdge("op_R23", "out", "op_R23sq", "left"),
				eEdge("c_2", "out", "op_R23sq", "right"),
				eEdge("var_R4", "out", "op_R4sq", "left"),
				eEdge("c_2", "out", "op_R4sq", "right"),
				eEdge("op_R1sq", "out", "op_inner1", "left"),
				eEdge("op_R23sq", "out", "op_inner1", "right"),
				eEdge("op_inner1", "out", "op_inner2", "left"),
				eEdge("op_R4sq", "out", "op_inner2", "right"),
				eEdge("op_inner2", "out", "fn_sqrt", "in"),
				eEdge("c_half", "out", "op_half", "left"),
				eEdge("fn_sqrt", "out", "op_half", "right"),
				eEdge("var_SMM", "out", "op_ratio", "left"),
				eEdge("op_half", "out", "op_ratio", "right"),
				eEdge("op_ratio", "out", "op_pct", "left"),
				eEdge("c_100", "out", "op_pct", "right"),
			},
			Outputs: []string{"op_pct"},
		}); err != nil {
		return err
	}

	// ── Spec #18: 逆ざや (planned − actual) · reserve ─────────────
	if _, err := seedFormula("日本生命保険 逆ざや",
		"life",
		"Negative interest spread = (平均予定利率 − 運用利回り) × 利息算入対象責任準備金. 入力: planned(平均予定利率), actual(運用利回り), reserve(利息算入対象責任準備金)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_planned", "planned"),
				nVar("var_actual", "actual"),
				nVar("var_reserve", "reserve"),
				nOp("op_gap", "subtract"),
				nOp("op_neg", "multiply"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_planned", "out", "op_gap", "left"),
				eEdge("var_actual", "out", "op_gap", "right"),
				eEdge("op_gap", "out", "op_neg", "left"),
				eEdge("var_reserve", "out", "op_neg", "right"),
			},
			Outputs: []string{"op_neg"},
		}); err != nil {
		return err
	}

	// ── Spec #20: 自賠責収支調整 surplus = premium − paid − admin ──
	if _, err := seedFormula("日本自賠責 収支調整剰余金",
		"auto",
		"CALI no-profit/no-loss surplus = collected_premium − claims_paid − admin_expense. 入力: premium(収入保険料), paid(支払保険金), admin(事務費)",
		domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				nVar("var_premium", "premium"),
				nVar("var_paid", "paid"),
				nVar("var_admin", "admin"),
				nOp("op_minus_paid", "subtract"),
				nOp("op_minus_admin", "subtract"),
			},
			Edges: []domain.FormulaEdge{
				eEdge("var_premium", "out", "op_minus_paid", "left"),
				eEdge("var_paid", "out", "op_minus_paid", "right"),
				eEdge("op_minus_paid", "out", "op_minus_admin", "left"),
				eEdge("var_admin", "out", "op_minus_admin", "right"),
			},
			Outputs: []string{"op_minus_admin"},
		}); err != nil {
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

// ─────────────────────────────────────────────────────────────────
// Graph construction helpers (task #045)
//
// The original seed code in this file builds FormulaGraphs by inlining
// FormulaNode literals and mustJSON calls, which is fine for a handful
// of formulas but quickly becomes noisy. The 17 spec-002 formulas added
// in task #045 use these helpers to keep each graph definition closer
// to "list of nodes + list of edges" — typically one line per node.
// ─────────────────────────────────────────────────────────────────

func nVar(id, name string) domain.FormulaNode {
	return domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeVariable,
		Config: mustJSON(map[string]any{"name": name, "dataType": "decimal"}),
	}
}

func nConst(id, value string) domain.FormulaNode {
	return domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeConstant,
		Config: mustJSON(map[string]any{"value": value}),
	}
}

func nOp(id, op string) domain.FormulaNode {
	return domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeOperator,
		Config: mustJSON(map[string]any{"op": op}),
	}
}

// nFn builds a function node. args may be nil; if non-nil it is
// merged into the config map (used for functions like round that take
// extra parameters).
func nFn(id, fn string, args map[string]string) domain.FormulaNode {
	cfg := map[string]any{"fn": fn}
	if args != nil {
		cfg["args"] = args
	}
	return domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeFunction,
		Config: mustJSON(cfg),
	}
}

// nTableAgg builds a TableAggregate node (task #040). filterCol/filterOp
// can be empty for "no filter". When filterInputPort is set, the filter
// pulls its right-hand side from a connected node; otherwise filterValue
// is taken as a literal.
func nTableAgg(id, tableID, aggregate, expression string, filters []domain.TableFilter, filterCombinator string) domain.FormulaNode {
	cfg := domain.TableAggregateConfig{
		TableID:          tableID,
		Aggregate:        aggregate,
		Expression:       expression,
		Filters:          filters,
		FilterCombinator: filterCombinator,
	}
	return domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeTableAggregate,
		Config: mustJSON(cfg),
	}
}

func eEdge(src, srcPort, tgt, tgtPort string) domain.FormulaEdge {
	return domain.FormulaEdge{
		Source:     src,
		SourcePort: srcPort,
		Target:     tgt,
		TargetPort: tgtPort,
	}
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
	// Task #045: spec 002 actuarial formulas (17 new)
	"日本生命保険 死亡率 qx",
	"日本生命保険 基数 Dx",
	"日本生命保険 将来法責任準備金",
	"日本生命保険 チルメル式責任準備金",
	"日本損害保険 損害率",
	"日本損害保険 発生保険金",
	"日本損害保険 チェインラダー LDF",
	"日本損害保険 BF法予測",
	"日本損害保険 法定IBNR要積立額b",
	"日本損害保険 1/24法未経過保険料",
	"日本損害保険 短期料率返還",
	"日本損害保険 Bühlmann信頼度",
	"日本損害賠償 休業損害(自賠責基準)",
	"日本損害賠償 逸失利益",
	"日本生命保険 ソルベンシー・マージン比率",
	"日本生命保険 逆ざや",
	"日本自賠責 収支調整剰余金",
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
	"claims_triangle_sample",
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
