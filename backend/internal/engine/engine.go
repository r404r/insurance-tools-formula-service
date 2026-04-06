package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// Engine is the interface for calculating insurance formulas.
type Engine interface {
	// Calculate evaluates the formula graph with the given string-encoded
	// input values and returns the results.
	Calculate(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]string) (*CalculationResult, error)

	// Validate checks the formula graph for structural errors without
	// executing it.
	Validate(graph *domain.FormulaGraph) []ValidationError
}

// CalculationResult holds the output of a formula evaluation.
type CalculationResult struct {
	// Outputs contains the final output values, keyed by output node ID,
	// encoded as decimal strings.
	Outputs map[string]string `json:"outputs"`

	// Intermediates contains the computed value for every node in the graph,
	// useful for debugging and audit trails. Keyed by node ID.
	Intermediates map[string]string `json:"intermediates"`

	// NodesEvaluated is the total number of nodes that were computed.
	NodesEvaluated int `json:"nodesEvaluated"`

	// ParallelLevels is the number of topological levels in the execution plan.
	ParallelLevels int `json:"parallelLevels"`

	// ExecutionTime is the wall-clock duration of the calculation.
	ExecutionTime time.Duration `json:"executionTime"`
}

// ValidationError describes a structural problem with a formula graph.
type ValidationError struct {
	NodeID  string `json:"nodeId"`
	Message string `json:"message"`
}

// Error implements the error interface for ValidationError.
func (ve ValidationError) Error() string {
	if ve.NodeID != "" {
		return fmt.Sprintf("node %s: %s", ve.NodeID, ve.Message)
	}
	return ve.Message
}

// TableResolver resolves lookup table data by table ID. keyColumns specifies
// which columns form the composite lookup key (values joined with "|").
// Returns a map from composite key to column value (as string-encoded decimal).
type TableResolver interface {
	ResolveTable(ctx context.Context, tableID string, keyColumns []string, column string) (map[string]string, error)
}

// EngineConfig holds configuration for the default engine implementation.
type EngineConfig struct {
	Workers       int
	Precision     PrecisionConfig
	CacheSize     int
	TableResolver TableResolver
	FormulaResolver FormulaResolver
}

// DefaultEngineConfig returns a sensible default configuration.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		Workers:   4,
		Precision: DefaultPrecision(),
		CacheSize: 1000,
	}
}

// defaultEngine is the production implementation of Engine.
type defaultEngine struct {
	executor      *Executor
	cache         *ResultCache
	config        EngineConfig
	tableResolver TableResolver
	formulaResolver FormulaResolver
}

// NewEngine creates a new Engine with the given configuration.
func NewEngine(cfg EngineConfig) Engine {
	engine := &defaultEngine{
		cache:           NewResultCache(cfg.CacheSize),
		config:          cfg,
		tableResolver:   cfg.TableResolver,
		formulaResolver: cfg.FormulaResolver,
	}
	engine.executor = NewExecutor(cfg.Workers, cfg.Precision, engine.executeSubFormula)
	return engine
}

// Calculate implements Engine.Calculate.
func (e *defaultEngine) Calculate(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]string) (*CalculationResult, error) {
	start := time.Now()

	// Parse string inputs to Decimal.
	decInputs, err := parseInputs(inputs)
	if err != nil {
		return nil, fmt.Errorf("parse inputs: %w", err)
	}

	plan, allResults, err := e.calculateGraph(ctx, graph, decInputs)
	if err != nil {
		return nil, err
	}

	// Collect outputs.
	outputValues := collectOutputValues(plan, allResults)
	outputs := make(map[string]string, len(outputValues))
	for outID, val := range outputValues {
			rounded := e.config.Precision.RoundOutput(val)
			outputs[outID] = rounded.String()
	}

	// Collect all intermediates.
	intermediates := make(map[string]string, len(allResults))
	for k, v := range allResults {
		intermediates[k] = v.String()
	}

	return &CalculationResult{
		Outputs:        outputs,
		Intermediates:  intermediates,
		NodesEvaluated: len(allResults),
		ParallelLevels: len(plan.Levels),
		ExecutionTime:  time.Since(start),
	}, nil
}

func (e *defaultEngine) calculateGraph(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]Decimal) (*ExecutionPlan, map[string]Decimal, error) {
	plan, err := BuildPlan(graph)
	if err != nil {
		return nil, nil, fmt.Errorf("build plan: %w", err)
	}

	workingInputs := cloneDecimalMap(inputs)
	if e.tableResolver != nil {
		if err := e.preloadTableData(ctx, graph, workingInputs); err != nil {
			return nil, nil, fmt.Errorf("preload table data: %w", err)
		}
	}

	allResults, err := e.executor.Execute(ctx, plan, workingInputs)
	if err != nil {
		return nil, nil, fmt.Errorf("execute: %w", err)
	}

	return plan, allResults, nil
}

func (e *defaultEngine) executeSubFormula(ctx context.Context, node *domain.FormulaNode, nodeInputs map[string]Decimal, seedInputs map[string]Decimal) (Decimal, error) {
	if e.formulaResolver == nil {
		return e.executor.evaluator.EvaluateNode(node, nodeInputs)
	}

	var cfg domain.SubFormulaConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid subFormula config: %w", node.ID, err)
	}

	if cfg.FormulaID == "" {
		return Zero, fmt.Errorf("node %s: subFormula config missing formulaId", node.ID)
	}

	version, err := e.formulaResolver.ResolveFormula(ctx, cfg.FormulaID, cfg.Version)
	if err != nil {
		return Zero, fmt.Errorf("node %s: resolve sub-formula: %w", node.ID, err)
	}

	childInputs := cloneDecimalMap(seedInputs)
	if in, ok := nodeInputs["in"]; ok {
		childInputs["in"] = in
	}

	childCtx, err := withSubFormulaCall(ctx, cfg.FormulaID, version.Version)
	if err != nil {
		return Zero, fmt.Errorf("node %s: %w", node.ID, err)
	}

	plan, allResults, err := e.calculateGraph(childCtx, &version.Graph, childInputs)
	if err != nil {
		return Zero, fmt.Errorf("node %s: calculate sub-formula %s v%d: %w", node.ID, cfg.FormulaID, version.Version, err)
	}

	outputs := collectOutputValues(plan, allResults)
	if len(outputs) == 0 {
		return Zero, fmt.Errorf("node %s: sub-formula %s produced no outputs", node.ID, cfg.FormulaID)
	}

	if len(outputs) > 1 {
		return Zero, fmt.Errorf("node %s: sub-formula %s must have exactly one output", node.ID, cfg.FormulaID)
	}

	for _, value := range outputs {
		return value, nil
	}

	return Zero, fmt.Errorf("node %s: sub-formula %s output resolution failed", node.ID, cfg.FormulaID)
}

func collectOutputValues(plan *ExecutionPlan, allResults map[string]Decimal) map[string]Decimal {
	outputs := make(map[string]Decimal, len(plan.DAG.OutputNodes()))
	for _, outID := range plan.DAG.OutputNodes() {
		if val, ok := allResults[outID]; ok {
			outputs[outID] = val
		}
	}
	return outputs
}

func cloneDecimalMap(inputs map[string]Decimal) map[string]Decimal {
	cloned := make(map[string]Decimal, len(inputs))
	for key, value := range inputs {
		cloned[key] = value
	}
	return cloned
}

// Validate implements Engine.Validate. It checks the graph for structural
// errors without executing any computation.
func (e *defaultEngine) Validate(graph *domain.FormulaGraph) []ValidationError {
	var errs []ValidationError

	// 1. Cycle detection.
	_, dagErr := BuildDAG(graph)
	if dagErr != nil {
		errs = append(errs, ValidationError{
			Message: dagErr.Error(),
		})
		// If we can't build the DAG, further validation is unreliable.
		return errs
	}

	dag, _ := BuildDAG(graph)

	// 2. Check that all declared outputs exist in the graph.
	nodeIDs := make(map[string]bool, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodeIDs[n.ID] = true
	}
	for _, outID := range graph.Outputs {
		if !nodeIDs[outID] {
			errs = append(errs, ValidationError{
				NodeID:  outID,
				Message: "declared output node does not exist in graph",
			})
		}
	}

	// 3. Check that output nodes have no outgoing edges (they are terminal).
	for _, outID := range graph.Outputs {
		if succs := dag.forward[outID]; len(succs) > 0 {
			errs = append(errs, ValidationError{
				NodeID:  outID,
				Message: "output node has outgoing edges; it should be a terminal node",
			})
		}
	}

	// 4. Check that input variable nodes have no incoming edges.
	for _, n := range graph.Nodes {
		if n.Type == domain.NodeVariable {
			if dag.inDegree[n.ID] > 0 {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: "variable node should not have incoming edges",
				})
			}
		}
	}

	// 5. Check node configs are valid JSON and type-compatible.
	for _, n := range graph.Nodes {
		if cfgErr := validateNodeConfig(n); cfgErr != nil {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Message: cfgErr.Error(),
			})
		}
	}

	// 6. Check that operator/function nodes have the required input ports connected.
	errs = append(errs, validateRequiredPorts(graph, dag)...)

	return errs
}

// preloadTableData finds all tableLookup nodes in the graph and pre-loads
// their table data into the inputs map with "table:<key>" entries so that
// the evaluator can resolve them during execution.
func (e *defaultEngine) preloadTableData(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]Decimal) error {
	for _, node := range graph.Nodes {
		if node.Type != domain.NodeTableLookup {
			continue
		}
		var cfg domain.TableLookupConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("node %s: invalid tableLookup config: %w", node.ID, err)
		}
		tableData, err := e.tableResolver.ResolveTable(ctx, cfg.TableID, cfg.EffectiveKeyColumns(), cfg.Column)
		if err != nil {
			return fmt.Errorf("node %s: resolve table %s: %w", node.ID, cfg.TableID, err)
		}
		for key, val := range tableData {
			d, err := decimal.NewFromString(val)
			if err != nil {
				return fmt.Errorf("table %s key %s: invalid decimal value %q: %w", cfg.TableID, key, val, err)
			}
			inputs["table:"+key] = d
		}
	}
	return nil
}

// parseInputs converts string input values to Decimals.
func parseInputs(inputs map[string]string) (map[string]Decimal, error) {
	result := make(map[string]Decimal, len(inputs))
	for k, v := range inputs {
		d, err := decimal.NewFromString(v)
		if err != nil {
			return nil, fmt.Errorf("input %q: cannot parse %q as decimal: %w", k, v, err)
		}
		result[k] = d
	}
	return result, nil
}

// validateNodeConfig checks that a node's config can be unmarshalled into
// the appropriate config struct for its type.
func validateNodeConfig(node domain.FormulaNode) error {
	if node.Config == nil {
		return fmt.Errorf("missing config")
	}

	switch node.Type {
	case domain.NodeVariable:
		var cfg domain.VariableConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid variable config: %w", err)
		}
		if cfg.Name == "" {
			return fmt.Errorf("variable config missing name")
		}

	case domain.NodeConstant:
		var cfg domain.ConstantConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid constant config: %w", err)
		}
		if cfg.Value == "" {
			return fmt.Errorf("constant config missing value")
		}
		if _, err := decimal.NewFromString(cfg.Value); err != nil {
			return fmt.Errorf("constant value %q is not a valid number", cfg.Value)
		}

	case domain.NodeOperator:
		var cfg domain.OperatorConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid operator config: %w", err)
		}
		validOps := map[string]bool{
			"add": true, "subtract": true, "multiply": true,
			"divide": true, "power": true, "modulo": true,
		}
		if !validOps[cfg.Op] {
			return fmt.Errorf("unknown operator %q", cfg.Op)
		}

	case domain.NodeFunction:
		var cfg domain.FunctionConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid function config: %w", err)
		}
		validFns := map[string]bool{
			"round": true, "floor": true, "ceil": true, "abs": true,
			"min": true, "max": true, "sqrt": true, "ln": true, "exp": true,
		}
		if !validFns[cfg.Fn] {
			return fmt.Errorf("unknown function %q", cfg.Fn)
		}

	case domain.NodeConditional:
		var cfg domain.ConditionalConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid conditional config: %w", err)
		}
		validComps := map[string]bool{
			"eq": true, "ne": true, "gt": true, "ge": true, "lt": true, "le": true,
		}
		if !validComps[cfg.Comparator] {
			return fmt.Errorf("unknown comparator %q", cfg.Comparator)
		}

	case domain.NodeAggregate:
		var cfg domain.AggregateConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid aggregate config: %w", err)
		}
		validAggs := map[string]bool{
			"sum": true, "product": true, "count": true, "avg": true,
		}
		if !validAggs[cfg.Fn] {
			return fmt.Errorf("unknown aggregate function %q", cfg.Fn)
		}

	case domain.NodeTableLookup:
		var cfg domain.TableLookupConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid tableLookup config: %w", err)
		}
		if cfg.TableID == "" {
			return fmt.Errorf("tableLookup config missing tableId")
		}

	case domain.NodeSubFormula:
		var cfg domain.SubFormulaConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid subFormula config: %w", err)
		}
		if cfg.FormulaID == "" {
			return fmt.Errorf("subFormula config missing formulaId")
		}
	}

	return nil
}

// validateRequiredPorts checks that nodes with known input port requirements
// have those ports connected by edges.
func validateRequiredPorts(graph *domain.FormulaGraph, dag *DAG) []ValidationError {
	var errs []ValidationError

	// Build a map of nodeID -> set of connected target ports.
	connectedPorts := make(map[string]map[string]bool)
	for _, e := range graph.Edges {
		if connectedPorts[e.Target] == nil {
			connectedPorts[e.Target] = make(map[string]bool)
		}
		port := e.TargetPort
		if port == "" {
			port = "in"
		}
		connectedPorts[e.Target][port] = true
	}

	for _, n := range graph.Nodes {
		ports := connectedPorts[n.ID]
		hasPort := func(name string) bool {
			return ports != nil && ports[name]
		}

		switch n.Type {
		case domain.NodeOperator:
			if !hasPort("left") {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: "operator node missing 'left' input connection",
				})
			}
			if !hasPort("right") {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: "operator node missing 'right' input connection",
				})
			}

		case domain.NodeFunction:
			var cfg domain.FunctionConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				continue
			}
			switch cfg.Fn {
			case "min", "max":
				if !hasPort("left") || !hasPort("right") {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Message: fmt.Sprintf("function %q requires 'left' and 'right' input connections", cfg.Fn),
					})
				}
			default:
				if !hasPort("in") {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Message: fmt.Sprintf("function %q requires 'in' input connection", cfg.Fn),
					})
				}
			}

		case domain.NodeConditional:
			for _, port := range []string{"condition", "conditionRight", "thenValue", "elseValue"} {
				if !hasPort(port) {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Message: fmt.Sprintf("conditional node missing '%s' input connection", port),
					})
				}
			}

		case domain.NodeTableLookup:
			var tlCfg domain.TableLookupConfig
			if err := json.Unmarshal(n.Config, &tlCfg); err != nil {
				continue
			}
			for _, kc := range tlCfg.EffectiveKeyColumns() {
				if !hasPort(kc) {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Message: fmt.Sprintf("tableLookup node missing %q input connection", kc),
					})
				}
			}
		}
	}

	return errs
}
