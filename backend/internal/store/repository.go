package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// Sentinel errors for user operations.
var (
	ErrLastAdmin  = errors.New("cannot remove the last administrator")
	ErrHasContent = errors.New("user has associated content and cannot be deleted")
	ErrTableInUse = errors.New("table is referenced by one or more formula versions")
	ErrConflict   = errors.New("record was modified by another request")
)

// FormulaRepository manages formula metadata persistence.
type FormulaRepository interface {
	Create(ctx context.Context, f *domain.Formula) error
	GetByID(ctx context.Context, id string) (*domain.Formula, error)
	List(ctx context.Context, filter domain.FormulaFilter) ([]*domain.Formula, int, error)
	Update(ctx context.Context, f *domain.Formula) error
	Delete(ctx context.Context, id string) error

	// UpdateMeta sets the formula's updated_by and updated_at columns
	// without touching name / domain / description. Called by the
	// version handler after a successful version save so that the
	// formula list's "Updater" column reflects whoever last changed
	// the underlying graph (not whoever last edited the metadata).
	UpdateMeta(ctx context.Context, formulaID, updatedBy string, updatedAt time.Time) error
}

// FormulaVersionAtomicCreator persists a formula and its initial version in a
// single database transaction. Production repositories implement this optional
// capability for template instantiation.
type FormulaVersionAtomicCreator interface {
	CreateWithInitialVersion(ctx context.Context, f *domain.Formula, v *domain.FormulaVersion) error
}

type SeedResetter interface {
	ResetSeedData(ctx context.Context) (formulasDeleted, tablesDeleted int64, err error)
}

// EnsureSeedLookupTablesUnreferenced rejects a seed reset if any persisted
// formula-version graph still names a seed-owned lookup table. Callers must
// invoke it from the same transaction that performs the deletes so a rejected
// reset leaves every seed record untouched.
func EnsureSeedLookupTablesUnreferenced(ctx context.Context, tx *sql.Tx) error {
	seedTableRows, err := tx.QueryContext(ctx, `SELECT id FROM lookup_tables WHERE seed_key != ''`)
	if err != nil {
		return fmt.Errorf("list seed lookup tables: %w", err)
	}
	defer seedTableRows.Close()

	seedTableIDs := make(map[string]struct{})
	for seedTableRows.Next() {
		var id string
		if err := seedTableRows.Scan(&id); err != nil {
			return fmt.Errorf("scan seed lookup table: %w", err)
		}
		seedTableIDs[id] = struct{}{}
	}
	if err := seedTableRows.Err(); err != nil {
		return fmt.Errorf("iterate seed lookup tables: %w", err)
	}
	if len(seedTableIDs) == 0 {
		return nil
	}

	versionRows, err := tx.QueryContext(ctx, `SELECT formula_versions.graph_json
		FROM formula_versions
		JOIN formulas ON formulas.id = formula_versions.formula_id
		WHERE formulas.seed_key = ''`)
	if err != nil {
		return fmt.Errorf("list formula version graphs: %w", err)
	}
	defer versionRows.Close()
	for versionRows.Next() {
		var graphJSON string
		if err := versionRows.Scan(&graphJSON); err != nil {
			return fmt.Errorf("scan formula version graph: %w", err)
		}
		referenced, err := GraphReferencesAnyTableID(graphJSON, seedTableIDs)
		if err != nil {
			return fmt.Errorf("inspect formula version graph: %w", err)
		}
		if referenced {
			return ErrHasContent
		}
	}
	if err := versionRows.Err(); err != nil {
		return fmt.Errorf("iterate formula version graphs: %w", err)
	}
	return nil
}

// GraphReferencesAnyTableID walks the persisted graph JSON rather than
// relying on whitespace-sensitive SQL LIKE patterns. It finds tableLookup
// and tableAggregate configs as well as future nodes that use a tableId key.
func GraphReferencesAnyTableID(graphJSON string, tableIDs map[string]struct{}) (bool, error) {
	var graph any
	if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
		return false, err
	}
	return valueReferencesAnyTableID(graph, tableIDs), nil
}

func valueReferencesAnyTableID(value any, tableIDs map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		if tableID, ok := typed["tableId"].(string); ok {
			if _, referenced := tableIDs[tableID]; referenced {
				return true
			}
		}
		for _, child := range typed {
			if valueReferencesAnyTableID(child, tableIDs) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if valueReferencesAnyTableID(child, tableIDs) {
				return true
			}
		}
	}
	return false
}

// VersionRepository manages formula version persistence.
type VersionRepository interface {
	CreateVersion(ctx context.Context, v *domain.FormulaVersion) error
	GetVersion(ctx context.Context, formulaID string, version int) (*domain.FormulaVersion, error)
	GetPublished(ctx context.Context, formulaID string) (*domain.FormulaVersion, error)
	ListVersions(ctx context.Context, formulaID string) ([]*domain.FormulaVersion, error)
	UpdateState(ctx context.Context, formulaID string, version int, state domain.VersionState) error
}

// UserRepository manages user persistence.
type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetTokenVersion(ctx context.Context, id string) (int, error)
	List(ctx context.Context) ([]*domain.User, error)
	UpdateRole(ctx context.Context, id string, role domain.Role) error
	Delete(ctx context.Context, id string) error
}

// TableRepository manages lookup table persistence.
type TableRepository interface {
	Create(ctx context.Context, t *domain.LookupTable) error
	GetByID(ctx context.Context, id string) (*domain.LookupTable, error)
	List(ctx context.Context, domain *domain.InsuranceDomain) ([]*domain.LookupTable, error)
	Update(ctx context.Context, t *domain.LookupTable) error
	Delete(ctx context.Context, id string) error
}

// CategoryRepository manages category persistence.
type CategoryRepository interface {
	Create(ctx context.Context, c *domain.Category) error
	GetByID(ctx context.Context, id string) (*domain.Category, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Category, error)
	List(ctx context.Context) ([]*domain.Category, error)
	Update(ctx context.Context, c *domain.Category) error
	Delete(ctx context.Context, id string) error
}

// SettingsRepository manages persistent application settings as key-value pairs.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	All(ctx context.Context) (map[string]string, error)
}

// Store is the top-level storage abstraction aggregating all repositories.
type Store interface {
	Formulas() FormulaRepository
	Versions() VersionRepository
	Users() UserRepository
	Tables() TableRepository
	Categories() CategoryRepository
	Settings() SettingsRepository
	Migrate(ctx context.Context) error
	// Ping verifies the underlying database connection is alive. Used by the
	// /healthz readiness probe (container orchestration + API regression suite).
	Ping(ctx context.Context) error
	Close() error
}
