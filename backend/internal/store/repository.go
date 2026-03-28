package store

import (
	"context"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// FormulaRepository manages formula metadata persistence.
type FormulaRepository interface {
	Create(ctx context.Context, f *domain.Formula) error
	GetByID(ctx context.Context, id string) (*domain.Formula, error)
	List(ctx context.Context, filter domain.FormulaFilter) ([]*domain.Formula, int, error)
	Update(ctx context.Context, f *domain.Formula) error
	Delete(ctx context.Context, id string) error
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
}

// TableRepository manages lookup table persistence.
type TableRepository interface {
	Create(ctx context.Context, t *domain.LookupTable) error
	GetByID(ctx context.Context, id string) (*domain.LookupTable, error)
	List(ctx context.Context, domain *domain.InsuranceDomain) ([]*domain.LookupTable, error)
}

// Store is the top-level storage abstraction aggregating all repositories.
type Store interface {
	Formulas() FormulaRepository
	Versions() VersionRepository
	Users() UserRepository
	Tables() TableRepository
	Migrate(ctx context.Context) error
	Close() error
}
