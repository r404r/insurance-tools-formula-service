package domain

import "time"

// VersionState represents the lifecycle state of a formula version
type VersionState string

const (
	StateDraft     VersionState = "draft"
	StatePublished VersionState = "published"
	StateArchived  VersionState = "archived"
)

// FormulaVersion represents a specific version of a formula
type FormulaVersion struct {
	ID         string       `json:"id"`
	FormulaID  string       `json:"formulaId"`
	Version    int          `json:"version"`
	State      VersionState `json:"state"`
	Graph      FormulaGraph `json:"graph"`
	ParentVer  *int         `json:"parentVer,omitempty"`
	ChangeNote string       `json:"changeNote"`
	CreatedBy  string       `json:"createdBy"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// User represents a system user
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // never serialize
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Role defines RBAC roles
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleEditor   Role = "editor"
	RoleReviewer Role = "reviewer"
	RoleViewer   Role = "viewer"
)

// CanEdit returns whether the role can create/edit formulas
func (r Role) CanEdit() bool {
	return r == RoleAdmin || r == RoleEditor
}

// CanPublish returns whether the role can publish/archive versions
func (r Role) CanPublish() bool {
	return r == RoleAdmin || r == RoleReviewer
}

// CanManageUsers returns whether the role can manage users
func (r Role) CanManageUsers() bool {
	return r == RoleAdmin
}

// VersionDiff represents differences between two formula versions
type VersionDiff struct {
	FromVersion int            `json:"fromVersion"`
	ToVersion   int            `json:"toVersion"`
	AddedNodes  []FormulaNode  `json:"addedNodes"`
	RemovedNodes []FormulaNode `json:"removedNodes"`
	ModifiedNodes []NodeDiff   `json:"modifiedNodes"`
	AddedEdges  []FormulaEdge  `json:"addedEdges"`
	RemovedEdges []FormulaEdge `json:"removedEdges"`
}

// NodeDiff represents a change to a single node
type NodeDiff struct {
	NodeID string      `json:"nodeId"`
	Before FormulaNode `json:"before"`
	After  FormulaNode `json:"after"`
}
