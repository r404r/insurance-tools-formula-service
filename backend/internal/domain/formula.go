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

// Formula represents a named calculation formula
type Formula struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Domain      InsuranceDomain `json:"domain"`
	Description string          `json:"description"`
	CreatedBy   string          `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
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
	NodeTableLookup NodeType = "tableLookup"
	NodeConditional NodeType = "conditional"
	NodeAggregate   NodeType = "aggregate"
	NodeLoop        NodeType = "loop"
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

type ConditionalConfig struct {
	Comparator string `json:"comparator"` // eq, ne, gt, ge, lt, le
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

// FormulaFilter for listing formulas
type FormulaFilter struct {
	Domain *InsuranceDomain `json:"domain,omitempty"`
	Search *string          `json:"search,omitempty"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}
