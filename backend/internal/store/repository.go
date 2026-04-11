package store

import (
	"context"
	"errors"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// Sentinel errors for user operations.
var (
	ErrLastAdmin  = errors.New("cannot remove the last administrator")
	ErrHasContent = errors.New("user has associated content and cannot be deleted")
	ErrTableInUse = errors.New("table is referenced by one or more formula versions")
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
	Close() error
}
