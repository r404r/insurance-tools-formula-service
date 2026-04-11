package parser

import (
	"encoding/json"
	"fmt"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// ValidationError describes a semantic issue found in a formula graph.
type ValidationError struct {
	NodeID  string `json:"nodeId"`
	Message string `json:"message"`
}

// Error implements the error interface for a single ValidationError.
func (ve ValidationError) Error() string {
	if ve.NodeID == "" {
		return ve.Message
	}
	return fmt.Sprintf("node %s: %s", ve.NodeID, ve.Message)
}

// knownFunctions is the set of built-in function names recognized by the engine.
var knownFunctions = map[string]bool{
	"round": true,
	"floor": true,
	"ceil":  true,
	"abs":   true,
	"min":   true,
	"max":   true,
	"sqrt":  true,
	"ln":    true,
	"exp":   true,
	"sum":   true,
	"avg":   true,
}

// validDataTypes lists the allowed data types for variable nodes.
var validDataTypes = map[string]bool{
	"integer": true,
	"decimal": true,
	"string":  true,
	"boolean": true,
}

// validComparators lists the allowed comparator strings for conditional nodes.
var validComparators = map[string]bool{
	"eq": true,
	"ne": true,
	"gt": true,
	"ge": true,
	"lt": true,
	"le": true,
}

// validBinaryOps lists the allowed operator names for binary operator nodes.
var validBinaryOps = map[string]bool{
	"add":      true,
	"subtract": true,
	"multiply": true,
	"divide":   true,
	"power":    true,
	"modulo":   true,
	"negate":   true,
}

// ValidateGraph performs semantic validation of a FormulaGraph and returns
// all errors found. An empty slice means the graph is valid.
func ValidateGraph(graph *domain.FormulaGraph) []ValidationError {
	var errs []ValidationError

	nodeIDs := make(map[string]bool, len(graph.Nodes))
	for _, n := range graph.Nodes {
		if nodeIDs[n.ID] {
			errs = append(errs, ValidationError{NodeID: n.ID, Message: "duplicate node ID"})
		}
		nodeIDs[n.ID] = true
	}

	// Check output nodes exist.
	if len(graph.Outputs) == 0 {
		errs = append(errs, ValidationError{Message: "graph has no output nodes"})
	}
	for _, outID := range graph.Outputs {
		if !nodeIDs[outID] {
			errs = append(errs, ValidationError{
				NodeID:  outID,
				Message: "output references non-existent node",
			})
		}
	}

	// Validate edges reference existing nodes.
	inDegree := make(map[string]int, len(graph.Nodes))
	outDegree := make(map[string]int, len(graph.Nodes))
	children := make(map[string][]string, len(graph.Nodes))
	targetPorts := make(map[string]map[string]bool)
	for _, e := range graph.Edges {
		if !nodeIDs[e.Source] {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("edge source %q does not reference an existing node", e.Source),
			})
		}
		if !nodeIDs[e.Target] {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("edge target %q does not reference an existing node", e.Target),
			})
		}
		inDegree[e.Target]++
		outDegree[e.Source]++
		children[e.Target] = append(children[e.Target], e.Source)

		// Check for duplicate target ports on the same node.
		if targetPorts[e.Target] == nil {
			targetPorts[e.Target] = make(map[string]bool)
		}
		if e.TargetPort != "" && targetPorts[e.Target][e.TargetPort] {
			errs = append(errs, ValidationError{
				NodeID:  e.Target,
				Message: fmt.Sprintf("duplicate edge on target port %q", e.TargetPort),
			})
		}
		targetPorts[e.Target][e.TargetPort] = true
	}

	// Validate individual nodes by type.
	for _, n := range graph.Nodes {
		switch n.Type {
		case domain.NodeVariable:
			errs = append(errs, validateVariable(n)...)
		case domain.NodeConstant:
			errs = append(errs, validateConstant(n)...)
		case domain.NodeOperator:
			errs = append(errs, validateOperator(n, inDegree[n.ID])...)
		case domain.NodeFunction:
			errs = append(errs, validateFunction(n)...)
		case domain.NodeConditional:
			errs = append(errs, validateConditional(n, inDegree[n.ID])...)
		case domain.NodeTableLookup:
			errs = append(errs, validateTableLookup(n)...)
		case domain.NodeTableAggregate:
			errs = append(errs, validateTableAggregate(n)...)
		case domain.NodeSubFormula:
			errs = append(errs, validateSubFormula(n)...)
		case domain.NodeAggregate:
			// Aggregates are accepted as-is for now.
		case domain.NodeLoop:
			// Loop nodes are validated by the engine layer.
		default:
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("unknown node type %q", n.Type),
			})
		}
	}

	// Detect disconnected nodes (warning-level: nodes with no edges at all
	// that are not listed as outputs).
	outputSet := make(map[string]bool, len(graph.Outputs))
	for _, id := range graph.Outputs {
		outputSet[id] = true
	}
	for _, n := range graph.Nodes {
		if inDegree[n.ID] == 0 && outDegree[n.ID] == 0 && !outputSet[n.ID] {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: "disconnected node: no incoming or outgoing edges",
			})
		}
	}

	// Detect cycles using topological sort (Kahn's algorithm).
	errs = append(errs, detectCycles(graph, nodeIDs, children)...)

	return errs
}

func validateVariable(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.VariableConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid variable config: " + err.Error()})
		return errs
	}
	if cfg.Name == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "variable node has empty name"})
	}
	if cfg.DataType != "" && !validDataTypes[cfg.DataType] {
		errs = append(errs, ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("variable has invalid dataType %q; expected one of integer, decimal, string, boolean", cfg.DataType),
		})
	}
	return errs
}

func validateConstant(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.ConstantConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid constant config: " + err.Error()})
		return errs
	}
	if cfg.Value == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "constant node has empty value"})
	}
	return errs
}

func validateOperator(n domain.FormulaNode, inputCount int) []ValidationError {
	var errs []ValidationError
	var cfg domain.OperatorConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid operator config: " + err.Error()})
		return errs
	}
	if !validBinaryOps[cfg.Op] {
		errs = append(errs, ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("unknown operator %q", cfg.Op),
		})
	}
	if cfg.Op == "negate" {
		if inputCount != 1 {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("unary operator %q expects 1 input but has %d", cfg.Op, inputCount),
			})
		}
	} else {
		if inputCount != 2 {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("binary operator %q expects 2 inputs but has %d", cfg.Op, inputCount),
			})
		}
	}
	return errs
}

func validateFunction(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.FunctionConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid function config: " + err.Error()})
		return errs
	}
	if !knownFunctions[cfg.Fn] {
		errs = append(errs, ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("unknown function %q", cfg.Fn),
		})
	}
	return errs
}

func validateConditional(n domain.FormulaNode, inputCount int) []ValidationError {
	var errs []ValidationError
	var cfg domain.ConditionalConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid conditional config: " + err.Error()})
		return errs
	}

	// Composite path: cfg.Conditions non-empty. Each term contributes a
	// (condition_i, conditionRight_i) pair, plus thenValue/elseValue, so
	// the expected wire count is 2*len(Conditions) + 2. This must mirror
	// the engine-side validator in backend/internal/engine/engine.go so
	// that an exported composite Conditional graph can be re-imported.
	if len(cfg.Conditions) > 0 {
		combinator := cfg.Combinator
		if combinator == "" {
			combinator = "and"
		}
		if combinator != "and" && combinator != "or" {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("unknown conditional combinator %q; expected \"and\" or \"or\"", cfg.Combinator),
			})
		}
		for i, term := range cfg.Conditions {
			if !validComparators[term.Op] {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: fmt.Sprintf("conditional term %d: unknown op %q; expected one of eq, ne, gt, ge, lt, le", i, term.Op),
				})
			}
		}
		expected := 2*len(cfg.Conditions) + 2
		if inputCount != expected {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("composite conditional with %d term(s) expects %d inputs (condition_i, conditionRight_i for each term plus thenValue, elseValue) but has %d", len(cfg.Conditions), expected, inputCount),
			})
		}
		return errs
	}

	// Legacy path: single comparator. Two shapes are accepted depending
	// on whether Comparator is set (pure comparison node — 2 inputs) or
	// blank (legacy if-then-else — 3 inputs). Behavior unchanged.
	if cfg.Comparator != "" {
		if !validComparators[cfg.Comparator] {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("unknown comparator %q; expected one of eq, ne, gt, ge, lt, le", cfg.Comparator),
			})
		}
		if inputCount != 2 {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("comparison node expects 2 inputs but has %d", inputCount),
			})
		}
	} else {
		// if/then/else expects 3 inputs: condition, consequent, alternate
		if inputCount != 3 {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("conditional node expects 3 inputs (condition, consequent, alternate) but has %d", inputCount),
			})
		}
	}
	return errs
}

func validateTableLookup(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.TableLookupConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid table lookup config: " + err.Error()})
		return errs
	}
	if cfg.TableID == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "table lookup has empty tableId"})
	}
	return errs
}

// validateTableAggregate checks the structural sanity of a NodeTableAggregate
// config — non-empty tableId / expression / aggregate, valid filter ops and
// combinator, and the value/inputPort exclusivity of each filter. The wire
// count is intentionally not checked here because the port set is dynamic
// (only filters with InputPort need an incoming edge); the engine layer's
// validateGraph handles per-filter port presence.
func validateTableAggregate(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.TableAggregateConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid table aggregate config: " + err.Error()})
		return errs
	}
	if cfg.TableID == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "table aggregate has empty tableId"})
	}
	if cfg.Expression == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "table aggregate has empty expression (column name)"})
	}
	validAggs := map[string]bool{
		"sum": true, "avg": true, "count": true, "min": true, "max": true, "product": true,
	}
	if !validAggs[cfg.Aggregate] {
		errs = append(errs, ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("table aggregate has unknown aggregate %q (expected sum/avg/count/min/max/product)", cfg.Aggregate),
		})
	}
	if cfg.FilterCombinator != "" && cfg.FilterCombinator != "and" && cfg.FilterCombinator != "or" {
		errs = append(errs, ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("table aggregate has unknown filterCombinator %q (expected and/or)", cfg.FilterCombinator),
		})
	}
	for i, f := range cfg.Filters {
		if f.Column == "" {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("table aggregate filter %d: missing column", i),
			})
		}
		if !validComparators[f.Op] {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("table aggregate filter %d: unknown op %q (expected one of eq, ne, gt, ge, lt, le)", i, f.Op),
			})
		}
		if f.Value != "" && f.InputPort != "" {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: fmt.Sprintf("table aggregate filter %d: cannot set both value and inputPort", i),
			})
		}
	}
	return errs
}

func validateSubFormula(n domain.FormulaNode) []ValidationError {
	var errs []ValidationError
	var cfg domain.SubFormulaConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "invalid sub-formula config: " + err.Error()})
		return errs
	}
	if cfg.FormulaID == "" {
		errs = append(errs, ValidationError{NodeID: n.ID, Message: "sub-formula has empty formulaId"})
	}
	return errs
}

// detectCycles uses Kahn's algorithm for topological sorting. If the
// algorithm cannot process all nodes, the remaining ones form a cycle.
func detectCycles(
	graph *domain.FormulaGraph,
	nodeIDs map[string]bool,
	children map[string][]string,
) []ValidationError {
	// Build in-degree counts based on edges.
	inDeg := make(map[string]int, len(nodeIDs))
	for id := range nodeIDs {
		inDeg[id] = 0
	}
	for _, e := range graph.Edges {
		inDeg[e.Target]++
	}

	// Seed with zero-in-degree nodes.
	queue := make([]string, 0, len(nodeIDs))
	for id, deg := range inDeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Build forward adjacency (source -> targets) for removing edges.
	fwd := make(map[string][]string)
	for _, e := range graph.Edges {
		fwd[e.Source] = append(fwd[e.Source], e.Target)
	}

	visited := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		visited++
		for _, tgt := range fwd[cur] {
			inDeg[tgt]--
			if inDeg[tgt] == 0 {
				queue = append(queue, tgt)
			}
		}
	}

	if visited < len(nodeIDs) {
		// Collect the nodes that are part of cycles.
		var cycleNodes []string
		for id, deg := range inDeg {
			if deg > 0 {
				cycleNodes = append(cycleNodes, id)
			}
		}
		return []ValidationError{{
			Message: fmt.Sprintf("cycle detected involving nodes: %v", cycleNodes),
		}}
	}

	return nil
}
