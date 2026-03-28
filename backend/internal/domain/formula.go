package domain

import (
	"encoding/json"
	"time"
)

// Insurance domain types
type InsuranceDomain string

const (
	DomainLife     InsuranceDomain = "life"
	DomainProperty InsuranceDomain = "property"
	DomainAuto     InsuranceDomain = "auto"
)

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
	ID     string          `json:"id"`
	Type   NodeType        `json:"type"`
	Config json.RawMessage `json:"config"`
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
	TableID   string `json:"tableId"`
	LookupKey string `json:"lookupKey"`
	Column    string `json:"column"`
}

type ConditionalConfig struct {
	Comparator string `json:"comparator"` // eq, ne, gt, ge, lt, le
}

type AggregateConfig struct {
	Fn    string `json:"fn"`    // sum, product, count, avg
	Range string `json:"range"` // expression defining iteration range
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
