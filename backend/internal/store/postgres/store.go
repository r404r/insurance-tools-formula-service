package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// PostgresStore implements store.Store using a PostgreSQL database.
type PostgresStore struct {
	db         *sql.DB
	formulas   *formulaRepo
	versions   *versionRepo
	users      *userRepo
	tables     *tableRepo
	categories *categoryRepo
	settings   *settingsRepo
}

// New opens a PostgreSQL database and returns a Store implementation.
func New(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	s := &PostgresStore{db: db}
	s.formulas = &formulaRepo{db: db}
	s.versions = &versionRepo{db: db}
	s.users = &userRepo{db: db}
	s.tables = &tableRepo{db: db}
	s.categories = &categoryRepo{db: db}
	s.settings = &settingsRepo{db: db}
	return s, nil
}

func (s *PostgresStore) Formulas() store.FormulaRepository    { return s.formulas }
func (s *PostgresStore) Versions() store.VersionRepository    { return s.versions }
func (s *PostgresStore) Users() store.UserRepository          { return s.users }
func (s *PostgresStore) Tables() store.TableRepository        { return s.tables }
func (s *PostgresStore) Categories() store.CategoryRepository { return s.categories }
func (s *PostgresStore) Settings() store.SettingsRepository   { return s.settings }

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// Migrate creates the schema tables if they do not exist.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         TEXT PRIMARY KEY,
			username   TEXT NOT NULL UNIQUE,
			password   TEXT NOT NULL,
			role       TEXT NOT NULL DEFAULT 'viewer',
			created_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS formulas (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			domain      TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_by  TEXT NOT NULL REFERENCES users(id),
			created_at  TIMESTAMPTZ NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS formula_versions (
			id          TEXT PRIMARY KEY,
			formula_id  TEXT NOT NULL REFERENCES formulas(id) ON DELETE CASCADE,
			version     INTEGER NOT NULL,
			state       TEXT NOT NULL DEFAULT 'draft',
			graph_json  TEXT NOT NULL,
			parent_ver  INTEGER,
			change_note TEXT NOT NULL DEFAULT '',
			created_by  TEXT NOT NULL REFERENCES users(id),
			created_at  TIMESTAMPTZ NOT NULL,
			UNIQUE(formula_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS lookup_tables (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			domain     TEXT NOT NULL,
			table_type TEXT NOT NULL,
			data_json  TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			color       TEXT NOT NULL DEFAULT '#6366f1',
			sort_order  INTEGER NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_formulas_domain ON formulas(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_formula_versions_formula ON formula_versions(formula_id)`,
		`CREATE INDEX IF NOT EXISTS idx_formula_versions_state ON formula_versions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_lookup_tables_domain ON lookup_tables(domain)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate %q: %w", stmt[:40], err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Formula repository
// ---------------------------------------------------------------------------

type formulaRepo struct {
	db *sql.DB
}

func (r *formulaRepo) Create(ctx context.Context, f *domain.Formula) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO formulas (id, name, domain, description, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		f.ID, f.Name, f.Domain, f.Description, f.CreatedBy, f.CreatedAt, f.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert formula: %w", err)
	}
	return nil
}

func (r *formulaRepo) GetByID(ctx context.Context, id string) (*domain.Formula, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, domain, description, created_by, created_at, updated_at
		 FROM formulas WHERE id = $1`, id)
	return scanFormula(row)
}

func (r *formulaRepo) List(ctx context.Context, filter domain.FormulaFilter) ([]*domain.Formula, int, error) {
	var whereClauses []string
	var args []interface{}
	argN := 1

	if filter.Domain != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("domain = $%d", argN))
		args = append(args, string(*filter.Domain))
		argN++
	}
	if filter.Search != nil && *filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", argN, argN+1))
		pattern := "%" + *filter.Search + "%"
		args = append(args, pattern, pattern)
		argN += 2
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM formulas " + whereSQL
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count formulas: %w", err)
	}

	query := "SELECT id, name, domain, description, created_by, created_at, updated_at FROM formulas " +
		whereSQL + " ORDER BY updated_at DESC"
	pageArgs := append([]interface{}{}, args...)

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argN)
		pageArgs = append(pageArgs, filter.Limit)
		argN++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argN)
		pageArgs = append(pageArgs, filter.Offset)
		argN++
	}
	_ = argN // consumed

	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list formulas: %w", err)
	}
	defer rows.Close()

	var result []*domain.Formula
	for rows.Next() {
		f, err := scanFormulaRows(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate formulas: %w", err)
	}
	return result, total, nil
}

func (r *formulaRepo) Update(ctx context.Context, f *domain.Formula) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE formulas SET name = $1, domain = $2, description = $3, updated_at = $4 WHERE id = $5`,
		f.Name, f.Domain, f.Description, f.UpdatedAt, f.ID,
	)
	if err != nil {
		return fmt.Errorf("update formula: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *formulaRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM formulas WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete formula: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------------
// Version repository
// ---------------------------------------------------------------------------

type versionRepo struct {
	db *sql.DB
}

func (r *versionRepo) CreateVersion(ctx context.Context, v *domain.FormulaVersion) error {
	graphJSON, err := json.Marshal(v.Graph)
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO formula_versions (id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		v.ID, v.FormulaID, v.Version, v.State, string(graphJSON),
		nullableInt(v.ParentVer), v.ChangeNote, v.CreatedBy, v.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}
	return nil
}

func (r *versionRepo) GetVersion(ctx context.Context, formulaID string, version int) (*domain.FormulaVersion, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = $1 AND version = $2`, formulaID, version)
	return scanVersion(row)
}

func (r *versionRepo) GetPublished(ctx context.Context, formulaID string) (*domain.FormulaVersion, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = $1 AND state = 'published'`, formulaID)
	return scanVersion(row)
}

func (r *versionRepo) ListVersions(ctx context.Context, formulaID string) ([]*domain.FormulaVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = $1 ORDER BY version DESC`, formulaID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var result []*domain.FormulaVersion
	for rows.Next() {
		v, err := scanVersionRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return result, nil
}

func (r *versionRepo) UpdateState(ctx context.Context, formulaID string, version int, state domain.VersionState) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Lock all version rows for this formula to serialize concurrent state transitions.
	if _, err := tx.ExecContext(ctx,
		`SELECT id FROM formula_versions WHERE formula_id = $1 FOR UPDATE`,
		formulaID,
	); err != nil {
		return fmt.Errorf("lock formula versions: %w", err)
	}

	var currentState string
	err = tx.QueryRowContext(ctx,
		`SELECT state FROM formula_versions WHERE formula_id = $1 AND version = $2`,
		formulaID, version,
	).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("get current state: %w", err)
	}

	if !isValidTransition(domain.VersionState(currentState), state) {
		return fmt.Errorf("invalid state transition from %s to %s", currentState, state)
	}

	if state == domain.StatePublished {
		_, err = tx.ExecContext(ctx,
			`UPDATE formula_versions SET state = $1 WHERE formula_id = $2 AND state = $3`,
			domain.StateArchived, formulaID, domain.StatePublished,
		)
		if err != nil {
			return fmt.Errorf("archive existing published version: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE formula_versions SET state = $1 WHERE formula_id = $2 AND version = $3`,
		state, formulaID, version,
	)
	if err != nil {
		return fmt.Errorf("update version state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

func isValidTransition(from, to domain.VersionState) bool {
	switch from {
	case domain.StateDraft:
		return to == domain.StatePublished || to == domain.StateArchived
	case domain.StatePublished:
		return to == domain.StateArchived
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// User repository
// ---------------------------------------------------------------------------

type userRepo struct {
	db *sql.DB
}

func (r *userRepo) Create(ctx context.Context, u *domain.User) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password, role, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.Password, u.Role, u.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, created_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, created_at FROM users WHERE username = $1`, username)
	return scanUser(row)
}

func (r *userRepo) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, password, role, created_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var result []*domain.User
	for rows.Next() {
		u, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return result, nil
}

func (r *userRepo) UpdateRole(ctx context.Context, id string, role domain.Role) error {
	if role != domain.RoleAdmin {
		var currentRole string
		err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, id).Scan(&currentRole)
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		if err != nil {
			return fmt.Errorf("get user role: %w", err)
		}
		if domain.Role(currentRole) == domain.RoleAdmin {
			var adminCount int
			if err := r.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != $1`, id).Scan(&adminCount); err != nil {
				return fmt.Errorf("count admins: %w", err)
			}
			if adminCount == 0 {
				return store.ErrLastAdmin
			}
		}
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = $1 WHERE id = $2`, role, id)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *userRepo) Delete(ctx context.Context, id string) error {
	var currentRole string
	err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, id).Scan(&currentRole)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("get user role: %w", err)
	}
	if domain.Role(currentRole) == domain.RoleAdmin {
		var adminCount int
		if err := r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != $1`, id).Scan(&adminCount); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if adminCount == 0 {
			return store.ErrLastAdmin
		}
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return store.ErrHasContent
		}
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------------
// Table repository
// ---------------------------------------------------------------------------

type tableRepo struct {
	db *sql.DB
}

func (r *tableRepo) Create(ctx context.Context, t *domain.LookupTable) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO lookup_tables (id, name, domain, table_type, data_json, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.Name, t.Domain, t.TableType, string(t.Data), t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert lookup table: %w", err)
	}
	return nil
}

func (r *tableRepo) GetByID(ctx context.Context, id string) (*domain.LookupTable, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, domain, table_type, data_json, created_at FROM lookup_tables WHERE id = $1`, id)
	return scanLookupTable(row)
}

func (r *tableRepo) List(ctx context.Context, domainFilter *domain.InsuranceDomain) ([]*domain.LookupTable, error) {
	var query string
	var args []interface{}

	if domainFilter != nil {
		query = `SELECT id, name, domain, table_type, data_json, created_at FROM lookup_tables WHERE domain = $1 ORDER BY name ASC`
		args = append(args, string(*domainFilter))
	} else {
		query = `SELECT id, name, domain, table_type, data_json, created_at FROM lookup_tables ORDER BY name ASC`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list lookup tables: %w", err)
	}
	defer rows.Close()

	var result []*domain.LookupTable
	for rows.Next() {
		t, err := scanLookupTableRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup tables: %w", err)
	}
	return result, nil
}

func (r *tableRepo) Update(ctx context.Context, t *domain.LookupTable) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE lookup_tables SET name = $1, table_type = $2, data_json = $3 WHERE id = $4`,
		t.Name, t.TableType, string(t.Data), t.ID,
	)
	if err != nil {
		return fmt.Errorf("update lookup table: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *tableRepo) Delete(ctx context.Context, id string) error {
	var refCount int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM formula_versions WHERE graph_json LIKE $1`,
		`%"tableId":"`+id+`"%`,
	).Scan(&refCount); err != nil {
		return fmt.Errorf("check table references: %w", err)
	}
	if refCount > 0 {
		return store.ErrTableInUse
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM lookup_tables WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete lookup table: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------------
// Category repository
// ---------------------------------------------------------------------------

type categoryRepo struct {
	db *sql.DB
}

func (r *categoryRepo) Create(ctx context.Context, c *domain.Category) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO categories (id, slug, name, description, color, sort_order, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.Slug, c.Name, c.Description, c.Color, c.SortOrder, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert category: %w", err)
	}
	return nil
}

func (r *categoryRepo) GetByID(ctx context.Context, id string) (*domain.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, slug, name, description, color, sort_order, created_at, updated_at
		 FROM categories WHERE id = $1`, id)
	return scanCategory(row)
}

func (r *categoryRepo) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, slug, name, description, color, sort_order, created_at, updated_at
		 FROM categories WHERE slug = $1`, slug)
	return scanCategory(row)
}

func (r *categoryRepo) List(ctx context.Context) ([]*domain.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, slug, name, description, color, sort_order, created_at, updated_at
		 FROM categories ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var result []*domain.Category
	for rows.Next() {
		c, err := scanCategoryRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return result, nil
}

func (r *categoryRepo) Update(ctx context.Context, c *domain.Category) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = $1, description = $2, color = $3, sort_order = $4, updated_at = $5 WHERE id = $6`,
		c.Name, c.Description, c.Color, c.SortOrder, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *categoryRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanFormula(s scanner) (*domain.Formula, error) {
	var f domain.Formula
	err := s.Scan(&f.ID, &f.Name, &f.Domain, &f.Description, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan formula: %w", err)
	}
	return &f, nil
}

func scanFormulaRows(rows *sql.Rows) (*domain.Formula, error) {
	return scanFormula(rows)
}

func scanVersion(s scanner) (*domain.FormulaVersion, error) {
	var v domain.FormulaVersion
	var graphJSON string
	var parentVer sql.NullInt64
	err := s.Scan(&v.ID, &v.FormulaID, &v.Version, &v.State, &graphJSON, &parentVer, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan version: %w", err)
	}
	if err := json.Unmarshal([]byte(graphJSON), &v.Graph); err != nil {
		return nil, fmt.Errorf("unmarshal graph: %w", err)
	}
	if parentVer.Valid {
		pv := int(parentVer.Int64)
		v.ParentVer = &pv
	}
	return &v, nil
}

func scanVersionRows(rows *sql.Rows) (*domain.FormulaVersion, error) {
	return scanVersion(rows)
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	err := s.Scan(&u.ID, &u.Username, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func scanUserRows(rows *sql.Rows) (*domain.User, error) {
	return scanUser(rows)
}

func scanLookupTable(s scanner) (*domain.LookupTable, error) {
	var t domain.LookupTable
	var dataJSON string
	err := s.Scan(&t.ID, &t.Name, &t.Domain, &t.TableType, &dataJSON, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan lookup table: %w", err)
	}
	t.Data = json.RawMessage(dataJSON)
	return &t, nil
}

func scanLookupTableRows(rows *sql.Rows) (*domain.LookupTable, error) {
	return scanLookupTable(rows)
}

func scanCategory(s scanner) (*domain.Category, error) {
	var c domain.Category
	err := s.Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.Color, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan category: %w", err)
	}
	return &c, nil
}

func scanCategoryRows(rows *sql.Rows) (*domain.Category, error) {
	return scanCategory(rows)
}

func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
