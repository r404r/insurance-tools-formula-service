package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// SQLiteStore implements store.Store using a SQLite database.
type SQLiteStore struct {
	db         *sql.DB
	formulas   *formulaRepo
	versions   *versionRepo
	users      *userRepo
	tables     *tableRepo
	categories *categoryRepo
	settings   *settingsRepo
}

// New opens a SQLite database and returns a Store implementation.
func New(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Verify the connection is alive.
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// SQLite with WAL mode supports concurrent reads but serialises writes.
	// Capping the pool prevents "database is locked" errors under high load.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	s := &SQLiteStore{db: db}
	s.formulas = &formulaRepo{db: db}
	s.versions = &versionRepo{db: db}
	s.users = &userRepo{db: db}
	s.tables = &tableRepo{db: db}
	s.categories = &categoryRepo{db: db}
	s.settings = &settingsRepo{db: db}
	return s, nil
}

func (s *SQLiteStore) Formulas() store.FormulaRepository    { return s.formulas }
func (s *SQLiteStore) Versions() store.VersionRepository    { return s.versions }
func (s *SQLiteStore) Users() store.UserRepository          { return s.users }
func (s *SQLiteStore) Tables() store.TableRepository        { return s.tables }
func (s *SQLiteStore) Categories() store.CategoryRepository { return s.categories }
func (s *SQLiteStore) Settings() store.SettingsRepository   { return s.settings }

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Migrate creates the schema tables if they do not exist.
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         TEXT PRIMARY KEY,
			username   TEXT NOT NULL UNIQUE,
			password   TEXT NOT NULL,
			role       TEXT NOT NULL DEFAULT 'viewer',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS formulas (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			domain      TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_by  TEXT NOT NULL REFERENCES users(id),
			updated_by  TEXT REFERENCES users(id),
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
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
			created_at  TEXT NOT NULL,
			UNIQUE(formula_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS lookup_tables (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			domain     TEXT NOT NULL,
			table_type TEXT NOT NULL,
			data_json  TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			color       TEXT NOT NULL DEFAULT '#6366f1',
			sort_order  INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_formulas_domain ON formulas(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_formula_versions_formula ON formula_versions(formula_id)`,
		`CREATE INDEX IF NOT EXISTS idx_formula_versions_state ON formula_versions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_lookup_tables_domain ON lookup_tables(domain)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TEXT NOT NULL
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

	// Idempotent column additions for old databases that pre-date a
	// schema bump. SQLite reports a "duplicate column name" error if
	// the column already exists; we tolerate that and re-raise any
	// other error. New installs hit the CREATE TABLE path above and
	// the ALTER becomes a no-op error that we swallow.
	alters := []struct {
		stmt string
		desc string
	}{
		{
			stmt: `ALTER TABLE formulas ADD COLUMN updated_by TEXT REFERENCES users(id)`,
			desc: "add formulas.updated_by (task #042)",
		},
	}
	for _, a := range alters {
		if _, err := tx.ExecContext(ctx, a.stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migrate %s: %w", a.desc, err)
			}
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
		f.CreatedAt.Format(time.RFC3339Nano),
		f.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert formula: %w", err)
	}
	return nil
}

func (r *formulaRepo) GetByID(ctx context.Context, id string) (*domain.Formula, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, domain, description, created_by, updated_by, created_at, updated_at
		 FROM formulas WHERE id = ?`, id)
	return scanFormula(row)
}

// formulaSortColumns maps the validated public sort field to the actual
// SQL column expression. The handler whitelists the sortBy value before
// it ever reaches this map, but we keep the map authoritative for the
// store layer too — never concatenate raw user input into the ORDER BY.
var formulaSortColumns = map[string]string{
	"name":      "f.name",
	"createdAt": "f.created_at",
	"updatedAt": "f.updated_at",
	"createdBy": "u1.username",
	"updatedBy": "u2.username",
}

func (r *formulaRepo) List(ctx context.Context, filter domain.FormulaFilter) ([]*domain.Formula, int, error) {
	var whereClauses []string
	var args []interface{}

	if filter.Domain != nil {
		whereClauses = append(whereClauses, "f.domain = ?")
		args = append(args, string(*filter.Domain))
	}
	if filter.Search != nil && *filter.Search != "" {
		whereClauses = append(whereClauses, "(f.name LIKE ? OR f.description LIKE ?)")
		pattern := "%" + *filter.Search + "%"
		args = append(args, pattern, pattern)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total matching rows. The count query does not need the
	// users join because we never filter on username.
	countQuery := "SELECT COUNT(*) FROM formulas f " + whereSQL
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count formulas: %w", err)
	}

	// Resolve the sort column. Defaults to updated_at desc to keep the
	// pre-task-#042 behavior for callers that pass empty sort fields.
	sortCol, ok := formulaSortColumns[filter.SortBy]
	if !ok {
		sortCol = "f.updated_at"
	}
	sortDir := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		sortDir = "ASC"
	}

	// Fetch the requested page. LEFT JOIN users twice (once for the
	// creator, once for the updater) so the same query yields both
	// usernames; updater rows that are NULL stay NULL through the
	// outer join and are scanned into the empty string fields below.
	//
	// NULLs sort last regardless of direction so that legacy rows
	// without an updater do not crowd the head of an ascending page.
	query := `SELECT f.id, f.name, f.domain, f.description,
		f.created_by, f.updated_by, f.created_at, f.updated_at,
		COALESCE(u1.username, ''), COALESCE(u2.username, '')
		FROM formulas f
		LEFT JOIN users u1 ON u1.id = f.created_by
		LEFT JOIN users u2 ON u2.id = f.updated_by
		` + whereSQL + ` ORDER BY (` + sortCol + ` IS NULL), ` + sortCol + ` ` + sortDir
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
		f, err := scanFormulaListRow(rows)
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

// UpdateMeta updates only updated_by and updated_at on a formula row.
// Used by the version handler after a successful version save so the
// list page's "Updater" column reflects whoever last edited the graph,
// not whoever last renamed the formula.
func (r *formulaRepo) UpdateMeta(ctx context.Context, formulaID, updatedBy string, updatedAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE formulas SET updated_by = ?, updated_at = ? WHERE id = ?`,
		updatedBy, updatedAt.Format(time.RFC3339Nano), formulaID,
	)
	if err != nil {
		return fmt.Errorf("update formula meta: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *formulaRepo) Update(ctx context.Context, f *domain.Formula) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE formulas SET name = ?, domain = ?, description = ?, updated_at = ? WHERE id = ?`,
		f.Name, f.Domain, f.Description, f.UpdatedAt.Format(time.RFC3339Nano), f.ID,
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
		v.CreatedAt.Format(time.RFC3339Nano),
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

// UpdateState transitions a version to the given state. When transitioning to
// Published, any existing published version for the same formula is archived
// first, enforcing the one-published-version-per-formula invariant.
func (r *versionRepo) UpdateState(ctx context.Context, formulaID string, version int, state domain.VersionState) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Validate the target version exists and read its current state.
	var currentState string
	err = tx.QueryRowContext(ctx,
		`SELECT state FROM formula_versions WHERE formula_id = ? AND version = ?`,
		formulaID, version,
	).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("get current state: %w", err)
	}

	// Enforce valid state transitions:
	//   draft     -> published | archived
	//   published -> archived
	//   archived  -> (no transitions allowed)
	if !isValidTransition(domain.VersionState(currentState), state) {
		return fmt.Errorf("invalid state transition from %s to %s", currentState, state)
	}

	// If publishing, archive the currently published version first.
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
		u.CreatedAt.Format(time.RFC3339Nano),
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
	// Prevent demoting the last admin.
	if role != domain.RoleAdmin {
		var currentRole string
		err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&currentRole)
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		if domain.Role(currentRole) == domain.RoleAdmin {
			var adminCount int
			_ = r.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != ?`, id).Scan(&adminCount)
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
	// Prevent deleting the last admin.
	var currentRole string
	err := r.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&currentRole)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if domain.Role(currentRole) == domain.RoleAdmin {
		var adminCount int
		_ = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != ?`, id).Scan(&adminCount)
		if adminCount == 0 {
			return store.ErrLastAdmin
		}
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
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
	dataJSON := string(t.Data)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO lookup_tables (id, name, domain, table_type, data_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Domain, t.TableType, dataJSON,
		t.CreatedAt.Format(time.RFC3339Nano),
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
	// Prevent deletion if any formula version references this table.
	var refCount int
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM formula_versions WHERE graph_json LIKE ?`,
		"%\"tableId\":\""+id+"\"%",
	).Scan(&refCount)
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
// Scan helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanFormula(s scanner) (*domain.Formula, error) {
	var f domain.Formula
	var createdAt, updatedAt string
	var updatedBy sql.NullString
	err := s.Scan(&f.ID, &f.Name, &f.Domain, &f.Description, &f.CreatedBy, &updatedBy, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan formula: %w", err)
	}
	if updatedBy.Valid {
		f.UpdatedBy = updatedBy.String
	}
	f.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &f, nil
}

func scanFormulaRows(rows *sql.Rows) (*domain.Formula, error) {
	return scanFormula(rows)
}

// scanFormulaListRow scans the wider List query that includes the
// joined creator/updater usernames in addition to the base columns.
func scanFormulaListRow(rows *sql.Rows) (*domain.Formula, error) {
	var f domain.Formula
	var createdAt, updatedAt string
	var updatedBy sql.NullString
	err := rows.Scan(
		&f.ID, &f.Name, &f.Domain, &f.Description,
		&f.CreatedBy, &updatedBy, &createdAt, &updatedAt,
		&f.CreatedByName, &f.UpdatedByName,
	)
	if err != nil {
		return nil, fmt.Errorf("scan formula list row: %w", err)
	}
	if updatedBy.Valid {
		f.UpdatedBy = updatedBy.String
	}
	f.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &f, nil
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

func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
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
		c.CreatedAt.Format(time.RFC3339Nano),
		c.UpdatedAt.Format(time.RFC3339Nano),
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
		c.Name, c.Description, c.Color, c.SortOrder, c.UpdatedAt.Format(time.RFC3339Nano), c.ID,
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
