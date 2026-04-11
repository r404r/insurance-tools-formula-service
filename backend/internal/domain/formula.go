package domain

import (
	"encoding/json"
	"time"
)

// InsuranceDomain is the category slug used to classify formulas and lookup tables.
// Values are now dynamic (managed via the categories table) rather than a fixed enum.
type InsuranceDomain string

// Category represents a user-defined formula category (replaces fixed InsuranceDomain constants).
type Category struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"` // URL-safe identifier, used as filter key
	Name        string    `json:"name"` // Display name (user-editable)
	Description string    `json:"description"`
	Color       string    `json:"color"`     // Hex color for UI badge, e.g. "#6366f1"
	SortOrder   int       `json:"sortOrder"` // Display order
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Formula represents a named calculation formula.
//
// CreatedBy / UpdatedBy carry user UUIDs. The repository's List method
// LEFT JOINs the users table and populates CreatedByName / UpdatedByName
// with the human-readable usernames so that the UI does not need a
// second round-trip. GetByID does NOT populate the *Name fields — they
// are list-only transient fields.
//
// UpdatedBy and UpdatedByName may be empty for legacy rows that were
// created before task #042 added the updated_by column. The frontend
// renders them as "—" in that case.
type Formula struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Domain        InsuranceDomain `json:"domain"`
	Description   string          `json:"description"`
	CreatedBy     string          `json:"createdBy"`
	UpdatedBy     string          `json:"updatedBy,omitempty"`
	CreatedByName string          `json:"createdByName,omitempty"`
	UpdatedByName string          `json:"updatedByName,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// FormulaGraph is the DAG representation of a formula
type FormulaGraph struct {
	Nodes   []FormulaNode `json:"nodes"`
	Edges   []FormulaEdge `json:"edges"`
	Outputs []string      `json:"outputs"`
	Layout  *GraphLayout  `json:"layout,omitempty"`
}

// FormulaNode represents a computation node in the formula DAG
type FormulaNode struct {
	ID          string          `json:"id"`
	Type        NodeType        `json:"type"`
	Config      json.RawMessage `json:"config"`
	Description string          `json:"description,omitempty"`
}

// NodeType enumerates the types of formula nodes
type NodeType string

const (
	NodeVariable    NodeType = "variable"
	NodeConstant    NodeType = "constant"
	NodeOperator    NodeType = "operator"
	NodeFunction    NodeType = "function"
	NodeSubFormula  NodeType = "subFormula"
	NodeTableLookup    NodeType = "tableLookup"
	NodeTableAggregate NodeType = "tableAggregate"
	NodeConditional    NodeType = "conditional"
	NodeAggregate      NodeType = "aggregate"
	NodeLoop           NodeType = "loop"
)

// FormulaEdge connects two nodes in the DAG
type FormulaEdge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	SourcePort string `json:"sourcePort"`
	TargetPort string `json:"targetPort"`
}

// GraphLayout stores react-flow node positions
type GraphLayout struct {
	Positions map[string]Position `json:"positions"`
}

// Position is x,y coordinates for a node
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Node config types

type VariableConfig struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"` // integer, decimal, string, boolean
}

type ConstantConfig struct {
	Value string `json:"value"` // string to preserve precision
}

type OperatorConfig struct {
	Op string `json:"op"` // add, subtract, multiply, divide, power, modulo
}

type FunctionConfig struct {
	Fn   string            `json:"fn"`   // round, floor, ceil, abs, min, max, sqrt, ln, exp
	Args map[string]string `json:"args"` // e.g. {"places": "2"}
}

type SubFormulaConfig struct {
	FormulaID string `json:"formulaId"`
	Version   *int   `json:"version,omitempty"` // nil = use published version
}

type TableLookupConfig struct {
	TableID    string   `json:"tableId"`
	KeyColumns []string `json:"keyColumns"` // columns used as composite lookup key; defaults to ["key"]
	Column     string   `json:"column"`
}

// EffectiveKeyColumns returns KeyColumns, falling back to ["key"] for backward compatibility.
func (c *TableLookupConfig) EffectiveKeyColumns() []string {
	if len(c.KeyColumns) == 0 {
		return []string{"key"}
	}
	return c.KeyColumns
}

// TableAggregateConfig configures a NodeTableAggregate node, which performs
// a SQL-style "SELECT <aggregate>(<expression>) FROM <tableId> WHERE <filters>"
// over a lookup table. v1 only supports a single column name in Expression
// (no DSL); v2 will add column expressions and self-join. See spec
// docs/specs/004-table-aggregate-node.md for the design rationale.
type TableAggregateConfig struct {
	TableID string `json:"tableId"` // required: lookup_tables.id

	// Filters narrow down which rows participate in the aggregate. Each
	// filter compares row[Column] against either a constant Value or a
	// dynamic value pulled from another node via InputPort. When the
	// list is empty, every row in the table is selected.
	Filters          []TableFilter `json:"filters,omitempty"`
	FilterCombinator string        `json:"filterCombinator,omitempty"` // "and" (default) or "or"

	// Aggregate selects the reduction. One of: sum, avg, count, min, max, product.
	Aggregate string `json:"aggregate"`

	// Expression names the column to aggregate. v1 supports only a single
	// column name; future versions may extend to a small DSL.
	Expression string `json:"expression"`
}

// TableFilter is one filter clause inside TableAggregateConfig. The right
// side of the comparison is either a literal Value or a dynamic InputPort
// (mutually exclusive). Negate inverts the term result.
type TableFilter struct {
	Column    string `json:"column"`
	Op        string `json:"op"` // eq, ne, gt, ge, lt, le

	Value     string `json:"value,omitempty"`     // literal right-hand side
	InputPort string `json:"inputPort,omitempty"` // dynamic right-hand side from a connected node

	Negate bool `json:"negate,omitempty"`
}

// ConditionalConfig configures an if-then-else node.
//
// Two formats are supported:
//
//   - Legacy single-comparison form: set Comparator to one of
//     eq/ne/gt/ge/lt/le. The evaluator reads ports "condition" and
//     "conditionRight" for the comparison sides, and "thenValue" /
//     "elseValue" for the branch outputs.
//
//   - Composite form: leave Comparator empty (or unused) and supply
//     Conditions []ConditionTerm + Combinator. The i-th condition
//     reads ports "condition_i" and "conditionRight_i". All conditions
//     are joined with the same Combinator ("and" / "or"). For mixed
//     AND/OR you nest two Conditional nodes — this is intentional
//     so a single node stays simple.
//
// Detection of which form to use is by len(Conditions): when the slice
// is non-empty the composite path is taken; otherwise the legacy fields
// are used. This keeps existing formulas (single-comparison) working
// without any migration.
type ConditionalConfig struct {
	// Legacy single-comparison fields (kept for backward compatibility)
	Comparator string `json:"comparator,omitempty"` // eq, ne, gt, ge, lt, le

	// Composite condition fields (preferred for new formulas)
	Conditions []ConditionTerm `json:"conditions,omitempty"`
	Combinator string          `json:"combinator,omitempty"` // "and" (default) or "or"
}

// ConditionTerm is one comparison inside a composite Conditional. Negate
// inverts the result of this single term (so users can write `NOT (A == B)`
// without an extra node). Term i is wired via input ports `condition_i`
// and `conditionRight_i`.
type ConditionTerm struct {
	Op     string `json:"op"`               // eq, ne, gt, ge, lt, le
	Negate bool   `json:"negate,omitempty"` // if true, the term's truth value is inverted
}

type AggregateConfig struct {
	Fn    string `json:"fn"`    // sum, product, count, avg
	Range string `json:"range"` // expression defining iteration range
}

// LoopConfig configures a loop node that iterates over a bounded integer range,
// calls a sub-formula for each iteration, and aggregates the results.
type LoopConfig struct {
	Mode          string `json:"mode"`                    // must be "range"
	FormulaID     string `json:"formulaId"`               // required: body sub-formula ID
	Version       *int   `json:"version,omitempty"`       // nil = use published version
	Iterator      string `json:"iterator"`                // required: variable name injected each iteration, e.g. "t"
	Aggregation    string `json:"aggregation"`                // sum/product/count/avg/min/max/last/fold
	InclusiveEnd   *bool  `json:"inclusiveEnd,omitempty"`    // default true
	MaxIterations  *int   `json:"maxIterations,omitempty"`   // node-level cap; falls back to engine default
	AccumulatorVar string `json:"accumulatorVar,omitempty"`  // variable name for fold accumulator
	InitValue      string `json:"initValue,omitempty"`       // initial accumulator value (decimal string)
}

// LookupTable stores reference data (mortality tables, rating tables, etc.)
type LookupTable struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Domain    InsuranceDomain `json:"domain"`
	TableType string          `json:"tableType"` // mortality, rating, factor
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
}

// FormulaFilter for listing formulas.
//
// SortBy / SortOrder are validated by the API handler against a strict
// whitelist before being passed to the store, so the store layer can
// translate them to SQL fragments without injection risk. Default
// behavior (empty SortBy or invalid value) is "updatedAt desc" so that
// pre-task-#042 callers see no behavior change.
type FormulaFilter struct {
	Domain    *InsuranceDomain `json:"domain,omitempty"`
	Search    *string          `json:"search,omitempty"`
	Limit     int              `json:"limit"`
	Offset    int              `json:"offset"`
	SortBy    string           `json:"sortBy,omitempty"`    // name | createdAt | updatedAt | createdBy | updatedBy
	SortOrder string           `json:"sortOrder,omitempty"` // asc | desc
}
