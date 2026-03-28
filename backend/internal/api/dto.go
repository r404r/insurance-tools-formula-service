package api

import (
	"encoding/json"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// --- Auth DTOs ---

// LoginRequest carries credentials for authentication.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

// RegisterRequest carries data for new user registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// --- Formula DTOs ---

// CreateFormulaRequest carries data for creating a new formula.
type CreateFormulaRequest struct {
	Name        string                 `json:"name"`
	Domain      domain.InsuranceDomain `json:"domain"`
	Description string                 `json:"description"`
}

// UpdateFormulaRequest carries optional fields for updating formula metadata.
type UpdateFormulaRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// --- Version DTOs ---

// CreateVersionRequest carries data for creating a new formula version.
type CreateVersionRequest struct {
	Graph      domain.FormulaGraph `json:"graph"`
	ChangeNote string              `json:"changeNote"`
}

// UpdateVersionStateRequest carries the target state for a version transition.
type UpdateVersionStateRequest struct {
	State domain.VersionState `json:"state"`
}

// --- Calculation DTOs ---

// CalculateRequest carries parameters for a single formula calculation.
type CalculateRequest struct {
	FormulaID string            `json:"formulaId"`
	Version   *int              `json:"version,omitempty"`
	Inputs    map[string]string `json:"inputs"`
	Precision *int32            `json:"precision,omitempty"`
}

// CalculateResponse holds the result of a single formula calculation.
type CalculateResponse struct {
	Result          map[string]string `json:"result"`
	Intermediates   map[string]string `json:"intermediates"`
	ExecutionTimeMs float64           `json:"executionTimeMs"`
	NodesEvaluated  int               `json:"nodesEvaluated"`
	ParallelLevels  int               `json:"parallelLevels"`
}

// BatchCalculateRequest carries parameters for multiple calculations.
type BatchCalculateRequest struct {
	FormulaID string              `json:"formulaId"`
	Version   *int                `json:"version,omitempty"`
	InputSets []map[string]string `json:"inputSets"`
}

// BatchCalculateResponse holds the results of a batch calculation.
type BatchCalculateResponse struct {
	Results []CalculateResponse `json:"results"`
}

// --- Table DTOs ---

// CreateTableRequest carries data for creating a lookup table.
type CreateTableRequest struct {
	Name      string                 `json:"name"`
	Domain    domain.InsuranceDomain `json:"domain"`
	TableType string                 `json:"tableType"`
	Data      json.RawMessage        `json:"data"`
}

// --- Error DTOs ---

// ErrorResponse is the standard error envelope returned by the API.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// --- User DTOs ---

// UpdateUserRoleRequest carries the target role for a user update.
type UpdateUserRoleRequest struct {
	Role domain.Role `json:"role"`
}
