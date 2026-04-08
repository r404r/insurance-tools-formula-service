package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// MySQLStore implements store.Store using a MySQL database.
type MySQLStore struct {
	db         *sql.DB
	formulas   *formulaRepo
	versions   *versionRepo
	users      *userRepo
	tables     *tableRepo
	categories *categoryRepo
	settings   *settingsRepo
}

// New opens a MySQL database and returns a Store implementation.
func New(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	s := &MySQLStore{db: db}
	s.formulas = &formulaRepo{db: db}
	s.versions = &versionRepo{db: db}
	s.users = &userRepo{db: db}
	s.tables = &tableRepo{db: db}
	s.categories = &categoryRepo{db: db}
	s.settings = &settingsRepo{db: db}
	return s, nil
}

func (s *MySQLStore) Formulas() store.FormulaRepository    { return s.formulas }
func (s *MySQLStore) Versions() store.VersionRepository    { return s.versions }
func (s *MySQLStore) Users() store.UserRepository          { return s.users }
func (s *MySQLStore) Tables() store.TableRepository        { return s.tables }
func (s *MySQLStore) Categories() store.CategoryRepository { return s.categories }
func (s *MySQLStore) Settings() store.SettingsRepository   { return s.settings }

func (s *MySQLStore) Close() error {
	return s.db.Close()
}

// Migrate creates the schema tables if they do not exist.
func (s *MySQLStore) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         VARCHAR(36)  PRIMARY KEY,
			username   VARCHAR(255) NOT NULL UNIQUE,
			password   TEXT         NOT NULL,
			role       VARCHAR(50)  NOT NULL DEFAULT 'viewer',
			created_at VARCHAR(35)  NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS formulas (
			id          VARCHAR(36)  PRIMARY KEY,
			name        TEXT         NOT NULL,
			domain      VARCHAR(255) NOT NULL,
			description TEXT         NOT NULL,
			created_by  VARCHAR(36)  NOT NULL,
			created_at  VARCHAR(35)  NOT NULL,
			updated_at  VARCHAR(35)  NOT NULL,
			FOREIGN KEY (created_by) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS formula_versions (
			id          VARCHAR(36)  PRIMARY KEY,
			formula_id  VARCHAR(36)  NOT NULL,
			version     INT          NOT NULL,
			state       VARCHAR(50)  NOT NULL DEFAULT 'draft',
			graph_json  MEDIUMTEXT   NOT NULL,
			parent_ver  INT,
			change_note TEXT         NOT NULL,
			created_by  VARCHAR(36)  NOT NULL,
			created_at  VARCHAR(35)  NOT NULL,
			UNIQUE KEY uq_formula_version (formula_id, version),
			FOREIGN KEY (formula_id) REFERENCES formulas(id) ON DELETE CASCADE,
			FOREIGN KEY (created_by) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS lookup_tables (
			id         VARCHAR(36)  PRIMARY KEY,
			name       TEXT         NOT NULL,
			domain     VARCHAR(255) NOT NULL,
			table_type VARCHAR(100) NOT NULL,
			data_json  MEDIUMTEXT   NOT NULL,
			created_at VARCHAR(35)  NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id          VARCHAR(36)  PRIMARY KEY,
			slug        VARCHAR(255) NOT NULL UNIQUE,
			name        TEXT         NOT NULL,
			description TEXT         NOT NULL,
			color       VARCHAR(50)  NOT NULL DEFAULT '#6366f1',
			sort_order  INT          NOT NULL DEFAULT 0,
			created_at  VARCHAR(35)  NOT NULL,
			updated_at  VARCHAR(35)  NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			` + "`key`" + `   VARCHAR(255) PRIMARY KEY,
			value      TEXT         NOT NULL,
			updated_at VARCHAR(35)  NOT NULL
		)`,
		`CREATE INDEX idx_formulas_domain ON formulas(domain)`,
		`CREATE INDEX idx_formula_versions_formula ON formula_versions(formula_id)`,
		`CREATE INDEX idx_formula_versions_state ON formula_versions(state)`,
		`CREATE INDEX idx_lookup_tables_domain ON lookup_tables(domain)`,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			// MySQL returns error 1061 when an index already exists; treat as no-op.
			var myErr *mysql.MySQLError
			if errors.As(err, &myErr) && myErr.Number == 1061 {
				continue
			}
			return fmt.Errorf("migrate: %w", err)
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
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Name, f.Domain, f.Description, f.CreatedBy,
		f.CreatedAt.UTC().Format(time.RFC3339Nano),
		f.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert formula: %w", err)
	}
	return nil
}

func (r *formulaRepo) GetByID(ctx context.Context, id string) (*domain.Formula, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, domain, description, created_by, created_at, updated_at
		 FROM formulas WHERE id = ?`, id)
	return scanFormula(row)
}

func (r *formulaRepo) List(ctx context.Context, filter domain.FormulaFilter) ([]*domain.Formula, int, error) {
	var whereClauses []string
	var args []interface{}

	if filter.Domain != nil {
		whereClauses = append(whereClauses, "domain = ?")
		args = append(args, string(*filter.Domain))
	}
	if filter.Search != nil && *filter.Search != "" {
		// MySQL LIKE is case-insensitive by default for utf8mb4_general_ci collation.
		whereClauses = append(whereClauses, "(name LIKE ? OR description LIKE ?)")
		pattern := "%" + *filter.Search + "%"
		args = append(args, pattern, pattern)
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
		query += " LIMIT ?"
		pageArgs = append(pageArgs, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		pageArgs = append(pageArgs, filter.Offset)
	}

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
		`UPDATE formulas SET name = ?, domain = ?, description = ?, updated_at = ? WHERE id = ?`,
		f.Name, f.Domain, f.Description, f.UpdatedAt.UTC().Format(time.RFC3339Nano), f.ID,
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM formulas WHERE id = ?`, id)
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.FormulaID, v.Version, v.State, string(graphJSON),
		nullableInt(v.ParentVer), v.ChangeNote, v.CreatedBy,
		v.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}
	return nil
}

func (r *versionRepo) GetVersion(ctx context.Context, formulaID string, version int) (*domain.FormulaVersion, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = ? AND version = ?`, formulaID, version)
	return scanVersion(row)
}

func (r *versionRepo) GetPublished(ctx context.Context, formulaID string) (*domain.FormulaVersion, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = ? AND state = 'published'`, formulaID)
	return scanVersion(row)
}

func (r *versionRepo) ListVersions(ctx context.Context, formulaID string) ([]*domain.FormulaVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, formula_id, version, state, graph_json, parent_ver, change_note, created_by, created_at
		 FROM formula_versions WHERE formula_id = ? ORDER BY version DESC`, formulaID)
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
		`SELECT id FROM formula_versions WHERE formula_id = ? FOR UPDATE`,
		formulaID,
	); err != nil {
		return fmt.Errorf("lock formula versions: %w", err)
	}

	var currentState string
	err = tx.QueryRowContext(ctx,
		`SELECT state FROM formula_versions WHERE formula_id = ? AND version = ?`,
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
			`UPDATE formula_versions SET state = ? WHERE formula_id = ? AND state = ?`,
			domain.StateArchived, formulaID, domain.StatePublished,
		)
		if err != nil {
			return fmt.Errorf("archive existing published version: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE formula_versions SET state = ? WHERE formula_id = ? AND version = ?`,
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
		 VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.Password, u.Role,
		u.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, created_at FROM users WHERE username = ?`, username)
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
		err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&currentRole)
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		if err != nil {
			return fmt.Errorf("get user role: %w", err)
		}
		if domain.Role(currentRole) == domain.RoleAdmin {
			var adminCount int
			if err := r.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != ?`, id).Scan(&adminCount); err != nil {
				return fmt.Errorf("count admins: %w", err)
			}
			if adminCount == 0 {
				return store.ErrLastAdmin
			}
		}
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = ? WHERE id = ?`, role, id)
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
	err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&currentRole)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("get user role: %w", err)
	}
	if domain.Role(currentRole) == domain.RoleAdmin {
		var adminCount int
		if err := r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != ?`, id).Scan(&adminCount); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if adminCount == 0 {
			return store.ErrLastAdmin
		}
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		var myErr *mysql.MySQLError
		// 1451: FK constraint fails (child rows exist)
		if errors.As(err, &myErr) && myErr.Number == 1451 {
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
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Domain, t.TableType, string(t.Data),
		t.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert lookup table: %w", err)
	}
	return nil
}

func (r *tableRepo) GetByID(ctx context.Context, id string) (*domain.LookupTable, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, domain, table_type, data_json, created_at FROM lookup_tables WHERE id = ?`, id)
	return scanLookupTable(row)
}

func (r *tableRepo) List(ctx context.Context, domainFilter *domain.InsuranceDomain) ([]*domain.LookupTable, error) {
	var query string
	var args []interface{}

	if domainFilter != nil {
		query = `SELECT id, name, domain, table_type, data_json, created_at FROM lookup_tables WHERE domain = ? ORDER BY name ASC`
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
		`UPDATE lookup_tables SET name = ?, table_type = ?, data_json = ? WHERE id = ?`,
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
		`SELECT COUNT(*) FROM formula_versions WHERE graph_json LIKE ?`,
		`%"tableId":"`+id+`"%`,
	).Scan(&refCount); err != nil {
		return fmt.Errorf("check table references: %w", err)
	}
	if refCount > 0 {
		return store.ErrTableInUse
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM lookup_tables WHERE id = ?`, id)
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Slug, c.Name, c.Description, c.Color, c.SortOrder,
		c.CreatedAt.UTC().Format(time.RFC3339Nano),
		c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert category: %w", err)
	}
	return nil
}

func (r *categoryRepo) GetByID(ctx context.Context, id string) (*domain.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, slug, name, description, color, sort_order, created_at, updated_at
		 FROM categories WHERE id = ?`, id)
	return scanCategory(row)
}

func (r *categoryRepo) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, slug, name, description, color, sort_order, created_at, updated_at
		 FROM categories WHERE slug = ?`, slug)
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
		`UPDATE categories SET name = ?, description = ?, color = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Description, c.Color, c.SortOrder, c.UpdatedAt.UTC().Format(time.RFC3339Nano), c.ID,
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
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
	var createdAt, updatedAt string
	err := s.Scan(&f.ID, &f.Name, &f.Domain, &f.Description, &f.CreatedBy, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan formula: %w", err)
	}
	f.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &f, nil
}

func scanFormulaRows(rows *sql.Rows) (*domain.Formula, error) {
	return scanFormula(rows)
}

func scanVersion(s scanner) (*domain.FormulaVersion, error) {
	var v domain.FormulaVersion
	var graphJSON string
	var parentVer sql.NullInt64
	var createdAt string
	err := s.Scan(&v.ID, &v.FormulaID, &v.Version, &v.State, &graphJSON, &parentVer, &v.ChangeNote, &v.CreatedBy, &createdAt)
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
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &v, nil
}

func scanVersionRows(rows *sql.Rows) (*domain.FormulaVersion, error) {
	return scanVersion(rows)
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	var createdAt string
	err := s.Scan(&u.ID, &u.Username, &u.Password, &u.Role, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &u, nil
}

func scanUserRows(rows *sql.Rows) (*domain.User, error) {
	return scanUser(rows)
}

func scanLookupTable(s scanner) (*domain.LookupTable, error) {
	var t domain.LookupTable
	var dataJSON string
	var createdAt string
	err := s.Scan(&t.ID, &t.Name, &t.Domain, &t.TableType, &dataJSON, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan lookup table: %w", err)
	}
	t.Data = json.RawMessage(dataJSON)
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &t, nil
}

func scanLookupTableRows(rows *sql.Rows) (*domain.LookupTable, error) {
	return scanLookupTable(rows)
}

func scanCategory(s scanner) (*domain.Category, error) {
	var c domain.Category
	var createdAt, updatedAt string
	err := s.Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.Color, &c.SortOrder, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan category: %w", err)
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
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
