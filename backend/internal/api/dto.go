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
	Name        *string                 `json:"name,omitempty"`
	Domain      *domain.InsuranceDomain `json:"domain,omitempty"`
	Description *string                 `json:"description,omitempty"`
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
	CacheHit        bool              `json:"cacheHit"`
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

// --- Batch-test DTOs ---

// BatchTestCase is one test case in a batch-test request.
type BatchTestCase struct {
	Label    string            `json:"label,omitempty"`
	Inputs   map[string]string `json:"inputs"`
	Expected map[string]string `json:"expected"`
}

// BatchTestRequest carries a batch of test cases with expected results.
type BatchTestRequest struct {
	FormulaID string          `json:"formulaId"`
	Version   *int            `json:"version,omitempty"`
	Tolerance string          `json:"tolerance,omitempty"` // relative, e.g. "0.01" = 1%
	Cases     []BatchTestCase `json:"cases"`
}

// BatchTestCaseResult holds the outcome for a single test case.
type BatchTestCaseResult struct {
	Index           int               `json:"index"`
	Label           string            `json:"label,omitempty"`
	Pass            bool              `json:"pass"`
	Inputs          map[string]string `json:"inputs"`
	Expected        map[string]string `json:"expected"`
	Actual          map[string]string `json:"actual"`
	Diff            map[string]string `json:"diff,omitempty"`
	ExecutionTimeMs float64           `json:"executionTimeMs"`
	CacheHit        bool              `json:"cacheHit"`
	Error           string            `json:"error,omitempty"`
}

// BatchTestSummary aggregates the batch-test results.
type BatchTestSummary struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	PassRate float64 `json:"passRate"`
	// TotalExecutionTimeMs is the wall-clock duration of the whole batch run,
	// measured in milliseconds. With parallel execution this is generally less
	// than the sum of per-case ExecutionTimeMs.
	TotalExecutionTimeMs float64 `json:"totalExecutionTimeMs"`
}

// BatchTestResponse is the full response for a batch-test run.
type BatchTestResponse struct {
	Summary BatchTestSummary      `json:"summary"`
	Results []BatchTestCaseResult `json:"results"`
}

// --- Table DTOs ---

// CreateTableRequest carries data for creating a lookup table.
type CreateTableRequest struct {
	Name      string                 `json:"name"`
	Domain    domain.InsuranceDomain `json:"domain"`
	TableType string                 `json:"tableType"`
	Data      json.RawMessage        `json:"data"`
}

// UpdateTableRequest carries fields for updating an existing lookup table.
type UpdateTableRequest struct {
	Name      string          `json:"name"`
	TableType string          `json:"tableType"`
	Data      json.RawMessage `json:"data"`
}

// --- Category DTOs ---

// CreateCategoryRequest carries data for creating a new category.
type CreateCategoryRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	SortOrder   int    `json:"sortOrder"`
}

// UpdateCategoryRequest carries optional fields for updating a category.
type UpdateCategoryRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	SortOrder   *int    `json:"sortOrder,omitempty"`
}

// --- Parse DTOs ---

// ParseRequest carries a text formula to be converted to a DAG graph.
type ParseRequest struct {
	Text string `json:"text"`
}

// ParseResponse returns the DAG graph converted from a text formula.
type ParseResponse struct {
	Graph domain.FormulaGraph `json:"graph"`
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

// --- Settings DTOs ---

// SettingsResponse carries the current application settings.
type SettingsResponse struct {
	MaxConcurrentCalcs int `json:"maxConcurrentCalcs"`
}

// UpdateSettingsRequest carries the fields to update; nil fields are unchanged.
type UpdateSettingsRequest struct {
	MaxConcurrentCalcs *int `json:"maxConcurrentCalcs"`
}
