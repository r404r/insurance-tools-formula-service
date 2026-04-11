package engine

import (
	"context"
	"crypto/sha256"
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

	// ClearCache removes all cached computation results.
	ClearCache()

	// CacheStats returns the current number of cached entries and the maximum
	// cache capacity.
	CacheStats() (size int, maxSize int)
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

	// CacheHit is true when the result was served from cache without
	// re-evaluating the graph.
	CacheHit bool `json:"cacheHit"`
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

// DefaultMaxLoopIterations is the engine-level cap on loop iterations when
// the loop node does not specify its own maxIterations.
const DefaultMaxLoopIterations = 1000

// EngineConfig holds configuration for the default engine implementation.
type EngineConfig struct {
	Workers             int
	Precision           PrecisionConfig
	CacheSize           int
	TableResolver       TableResolver
	FormulaResolver     FormulaResolver
	MaxLoopIterations   int // 0 means use DefaultMaxLoopIterations
}

// DefaultEngineConfig returns a sensible default configuration.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		Workers:           4,
		Precision:         DefaultPrecision(),
		CacheSize:         1000,
		MaxLoopIterations: DefaultMaxLoopIterations,
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
	if cfg.MaxLoopIterations <= 0 {
		cfg.MaxLoopIterations = DefaultMaxLoopIterations
	}
	engine := &defaultEngine{
		cache:           NewResultCache(cfg.CacheSize),
		config:          cfg,
		tableResolver:   cfg.TableResolver,
		formulaResolver: cfg.FormulaResolver,
	}
	engine.executor = NewExecutor(cfg.Workers, cfg.Precision, engine.executeSubFormula, engine.executeLoop)
	return engine
}

// graphHash returns a short SHA-256 digest of the serialised graph, used as
// the cache key's "formula" dimension so that two different graph versions
// never share a cache entry.
func graphHash(graph *domain.FormulaGraph) string {
	b, _ := json.Marshal(graph)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:8]) // 16 hex chars — enough for uniqueness
}

// Calculate implements Engine.Calculate.
func (e *defaultEngine) Calculate(ctx context.Context, graph *domain.FormulaGraph, inputs map[string]string) (*CalculationResult, error) {
	start := time.Now()

	// Parse string inputs to Decimal.
	decInputs, err := parseInputs(inputs)
	if err != nil {
		return nil, fmt.Errorf("parse inputs: %w", err)
	}

	// Check cache before executing the graph.
	cacheKey := CacheKey{
		FormulaID: graphHash(graph),
		Version:   "1",
		InputHash: ComputeInputHash(decInputs),
	}
	if cached, ok := e.cache.Get(cacheKey); ok {
		outputs := make(map[string]string, len(cached.Outputs))
		for k, v := range cached.Outputs {
			outputs[k] = v.String()
		}
		intermediates := make(map[string]string, len(cached.Intermediates))
		for k, v := range cached.Intermediates {
			intermediates[k] = v.String()
		}
		return &CalculationResult{
			Outputs:        outputs,
			Intermediates:  intermediates,
			NodesEvaluated: cached.NodesEvaluated,
			ParallelLevels: cached.ParallelLevels,
			ExecutionTime:  time.Since(start),
			CacheHit:       true,
		}, nil
	}

	plan, allResults, err := e.calculateGraph(ctx, graph, decInputs)
	if err != nil {
		return nil, err
	}

	// Collect outputs.
	outputValues := collectOutputValues(plan, allResults)
	outputs := make(map[string]string, len(outputValues))
	outputDecimals := make(map[string]Decimal, len(outputValues))
	for outID, val := range outputValues {
		rounded := e.config.Precision.RoundOutput(val)
		outputs[outID] = rounded.String()
		outputDecimals[outID] = rounded
	}

	// Collect all intermediates.
	intermediates := make(map[string]string, len(allResults))
	intermediateDecimals := make(map[string]Decimal, len(allResults))
	for k, v := range allResults {
		intermediates[k] = v.String()
		intermediateDecimals[k] = v
	}

	// Store full result in cache (outputs + intermediates + metadata).
	e.cache.Set(cacheKey, CachedResult{
		Outputs:        outputDecimals,
		Intermediates:  intermediateDecimals,
		NodesEvaluated: len(allResults),
		ParallelLevels: len(plan.Levels),
	})

	return &CalculationResult{
		Outputs:        outputs,
		Intermediates:  intermediates,
		NodesEvaluated: len(allResults),
		ParallelLevels: len(plan.Levels),
		ExecutionTime:  time.Since(start),
		CacheHit:       false,
	}, nil
}

// ClearCache implements Engine.ClearCache.
//
// In addition to flushing the ResultCache (cached formula outputs), this also
// cascades to the TableResolver when it supports invalidation. The table
// HTTP handler calls ClearCache() on every Update/Delete (see table_handler.go),
// so this keeps the parsed-rows cache in StoreTableResolver consistent with
// the underlying lookup_tables rows without any extra plumbing.
func (e *defaultEngine) ClearCache() {
	e.cache.Clear()
	if inv, ok := e.tableResolver.(interface{ InvalidateAll() }); ok {
		inv.InvalidateAll()
	}
}

// CacheStats implements Engine.CacheStats.
func (e *defaultEngine) CacheStats() (size int, maxSize int) {
	return e.cache.Len(), e.cache.maxSize
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

// executeLoop handles NodeLoop execution. It generates a bounded integer
// iteration sequence, calls the body sub-formula for each step, and
// aggregates the results.
func (e *defaultEngine) executeLoop(ctx context.Context, node *domain.FormulaNode, nodeInputs map[string]Decimal, seedInputs map[string]Decimal) (Decimal, error) {
	if e.formulaResolver == nil {
		return Zero, fmt.Errorf("node %s: loop requires a formula resolver", node.ID)
	}

	var cfg domain.LoopConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid loop config: %w", node.ID, err)
	}

	// Validate config fields (belt-and-suspenders; static validation also covers these).
	if cfg.FormulaID == "" {
		return Zero, fmt.Errorf("node %s: loop node missing formulaId", node.ID)
	}
	if cfg.Iterator == "" {
		return Zero, fmt.Errorf("node %s: loop node missing iterator", node.ID)
	}

	// Read start/end/step from node inputs.
	startVal, ok := nodeInputs["start"]
	if !ok {
		return Zero, fmt.Errorf("node %s: loop node missing 'start' input", node.ID)
	}
	endVal, ok := nodeInputs["end"]
	if !ok {
		return Zero, fmt.Errorf("node %s: loop node missing 'end' input", node.ID)
	}
	stepVal, hasStep := nodeInputs["step"]
	if !hasStep {
		stepVal = One
	}

	// All three must be integer-valued decimals.
	if !startVal.Equal(startVal.Truncate(0)) {
		return Zero, fmt.Errorf("node %s: loop node 'start' must be an integer, got %s", node.ID, startVal.String())
	}
	if !endVal.Equal(endVal.Truncate(0)) {
		return Zero, fmt.Errorf("node %s: loop node 'end' must be an integer, got %s", node.ID, endVal.String())
	}
	if !stepVal.Equal(stepVal.Truncate(0)) {
		return Zero, fmt.Errorf("node %s: loop node 'step' must be an integer, got %s", node.ID, stepVal.String())
	}
	if stepVal.IsZero() {
		return Zero, fmt.Errorf("node %s: loop node step cannot be zero", node.ID)
	}

	// Determine effective maxIterations.
	maxIter := e.config.MaxLoopIterations
	if cfg.MaxIterations != nil && *cfg.MaxIterations > 0 && *cfg.MaxIterations < maxIter {
		maxIter = *cfg.MaxIterations
	}

	// Determine inclusiveEnd (default true).
	inclusiveEnd := true
	if cfg.InclusiveEnd != nil {
		inclusiveEnd = *cfg.InclusiveEnd
	}

	// Resolve the body formula.
	version, err := e.formulaResolver.ResolveFormula(ctx, cfg.FormulaID, cfg.Version)
	if err != nil {
		return Zero, fmt.Errorf("node %s: resolve loop body formula: %w", node.ID, err)
	}

	// Push the call-stack guard entry for the body formula.
	loopCtx, err := withSubFormulaCall(ctx, cfg.FormulaID, version.Version)
	if err != nil {
		return Zero, fmt.Errorf("node %s: %w", node.ID, err)
	}

	// Guard: iterator variable must not shadow a parent input to avoid silent wrong results.
	if _, conflict := seedInputs[cfg.Iterator]; conflict {
		return Zero, fmt.Errorf("node %s: loop iterator %q conflicts with an existing input variable", node.ID, cfg.Iterator)
	}

	// Pre-build the execution plan for the body formula once; reuse across all iterations.
	bodyPlan, err := BuildPlan(&version.Graph)
	if err != nil {
		return Zero, fmt.Errorf("node %s: loop body plan error: %w", node.ID, err)
	}
	if len(bodyPlan.DAG.OutputNodes()) == 0 {
		return Zero, fmt.Errorf("node %s: loop body formula has no output nodes", node.ID)
	}
	if len(bodyPlan.DAG.OutputNodes()) > 1 {
		return Zero, fmt.Errorf("node %s: loop body formula must have exactly one output", node.ID)
	}
	bodyOutputID := bodyPlan.DAG.OutputNodes()[0]

	// Preload table data for the body graph once (table content is iteration-independent).
	baseChildInputs := cloneDecimalMap(seedInputs)
	if e.tableResolver != nil {
		if err := e.preloadTableData(loopCtx, &version.Graph, baseChildInputs); err != nil {
			return Zero, fmt.Errorf("node %s: loop body table preload: %w", node.ID, err)
		}
	}

	// ── Fold mode: stateful accumulation (separate path) ──
	if cfg.Aggregation == "fold" {
		return e.executeFoldLoop(loopCtx, node, cfg, bodyPlan, bodyOutputID, baseChildInputs, startVal, endVal, stepVal, inclusiveEnd, maxIter)
	}

	// ── Map-reduce mode: independent iterations + aggregation ──
	var iterResults []Decimal
	current := startVal

	for {
		// Check termination condition.
		var done bool
		if stepVal.IsPositive() {
			if inclusiveEnd {
				done = current.GreaterThan(endVal)
			} else {
				done = current.GreaterThanOrEqual(endVal)
			}
		} else {
			if inclusiveEnd {
				done = current.LessThan(endVal)
			} else {
				done = current.LessThanOrEqual(endVal)
			}
		}
		if done {
			break
		}

		// Check maxIterations guard before executing (so the error message is clear).
		if len(iterResults) >= maxIter {
			return Zero, fmt.Errorf("node %s: loop exceeded maxIterations (%d)", node.ID, maxIter)
		}

		// Build child inputs: clone base (with table data) + inject iterator variable.
		childInputs := cloneDecimalMap(baseChildInputs)
		childInputs[cfg.Iterator] = current

		allResults, err := e.executor.Execute(loopCtx, bodyPlan, childInputs)
		if err != nil {
			return Zero, fmt.Errorf("node %s: loop iteration %s=%s: %w", node.ID, cfg.Iterator, current.String(), err)
		}

		if v, ok := allResults[bodyOutputID]; ok {
			iterResults = append(iterResults, v)
		}

		current = current.Add(stepVal)
	}

	// Aggregate results (aggregateLoopResults handles empty slices for
	// identity-element aggregations like sum/product/count).
	return aggregateLoopResults(node.ID, cfg.Aggregation, iterResults, e.config.Precision)
}

// executeFoldLoop implements the fold/accumulate pattern where each iteration
// receives the previous iteration's result via an accumulator variable.
func (e *defaultEngine) executeFoldLoop(
	ctx context.Context,
	node *domain.FormulaNode,
	cfg domain.LoopConfig,
	bodyPlan *ExecutionPlan,
	bodyOutputID string,
	baseChildInputs map[string]Decimal,
	startVal, endVal, stepVal Decimal,
	inclusiveEnd bool,
	maxIter int,
) (Decimal, error) {
	if cfg.AccumulatorVar == "" {
		return Zero, fmt.Errorf("node %s: fold mode requires accumulatorVar", node.ID)
	}
	if cfg.AccumulatorVar == cfg.Iterator {
		return Zero, fmt.Errorf("node %s: fold accumulatorVar %q must differ from iterator %q", node.ID, cfg.AccumulatorVar, cfg.Iterator)
	}
	if _, conflict := baseChildInputs[cfg.AccumulatorVar]; conflict {
		return Zero, fmt.Errorf("node %s: fold accumulatorVar %q conflicts with an existing input variable", node.ID, cfg.AccumulatorVar)
	}

	// Parse initial accumulator value (default 0).
	acc := Zero
	if cfg.InitValue != "" {
		var err error
		acc, err = decimal.NewFromString(cfg.InitValue)
		if err != nil {
			return Zero, fmt.Errorf("node %s: invalid fold initValue %q: %w", node.ID, cfg.InitValue, err)
		}
	}

	current := startVal
	iterCount := 0

	for {
		var done bool
		if stepVal.IsPositive() {
			if inclusiveEnd {
				done = current.GreaterThan(endVal)
			} else {
				done = current.GreaterThanOrEqual(endVal)
			}
		} else {
			if inclusiveEnd {
				done = current.LessThan(endVal)
			} else {
				done = current.LessThanOrEqual(endVal)
			}
		}
		if done {
			break
		}

		if iterCount >= maxIter {
			return Zero, fmt.Errorf("node %s: fold loop exceeded maxIterations (%d)", node.ID, maxIter)
		}

		childInputs := cloneDecimalMap(baseChildInputs)
		childInputs[cfg.Iterator] = current
		childInputs[cfg.AccumulatorVar] = acc

		allResults, err := e.executor.Execute(ctx, bodyPlan, childInputs)
		if err != nil {
			return Zero, fmt.Errorf("node %s: fold iteration %s=%s: %w", node.ID, cfg.Iterator, current.String(), err)
		}

		if v, ok := allResults[bodyOutputID]; ok {
			acc = v
		}

		current = current.Add(stepVal)
		iterCount++
	}

	// 0 iterations → return initValue (this is correct for fold: fold [] init = init)
	return acc, nil
}

// aggregateLoopResults applies the specified aggregation function to the loop
// iteration results. For aggregations with a well-defined identity element
// (sum, product, count) an empty slice returns that identity. For aggregations
// that require at least one value (avg, min, max, last) an empty slice is an
// error.
func aggregateLoopResults(nodeID string, aggregation string, results []Decimal, prec PrecisionConfig) (Decimal, error) {
	switch aggregation {
	case "sum":
		acc := Zero
		for _, v := range results {
			acc = acc.Add(v)
		}
		return acc, nil

	case "product":
		acc := One
		for _, v := range results {
			acc = acc.Mul(v)
		}
		return acc, nil

	case "count":
		return NewDecimalFromInt(int64(len(results))), nil

	case "avg":
		if len(results) == 0 {
			return Zero, fmt.Errorf("node %s: loop produced zero iterations", nodeID)
		}
		acc := Zero
		for _, v := range results {
			acc = acc.Add(v)
		}
		count := NewDecimalFromInt(int64(len(results)))
		return acc.DivRound(count, prec.IntermediatePrecision), nil

	case "min":
		if len(results) == 0 {
			return Zero, fmt.Errorf("node %s: loop produced zero iterations", nodeID)
		}
		m := results[0]
		for _, v := range results[1:] {
			if v.LessThan(m) {
				m = v
			}
		}
		return m, nil

	case "max":
		if len(results) == 0 {
			return Zero, fmt.Errorf("node %s: loop produced zero iterations", nodeID)
		}
		m := results[0]
		for _, v := range results[1:] {
			if v.GreaterThan(m) {
				m = v
			}
		}
		return m, nil

	case "last":
		if len(results) == 0 {
			return Zero, fmt.Errorf("node %s: loop produced zero iterations", nodeID)
		}
		return results[len(results)-1], nil

	default:
		return Zero, fmt.Errorf("node %s: unknown loop aggregation %q", nodeID, aggregation)
	}
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
func isValidIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

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
		if len(cfg.Conditions) > 0 {
			// Composite path: validate each term and the combinator. The
			// legacy Comparator field is intentionally ignored when in
			// composite mode (the evaluator does the same).
			combinator := cfg.Combinator
			if combinator == "" {
				combinator = "and"
			}
			if combinator != "and" && combinator != "or" {
				return fmt.Errorf("unknown combinator %q", cfg.Combinator)
			}
			for i, term := range cfg.Conditions {
				if !validComps[term.Op] {
					return fmt.Errorf("conditional term %d: unknown op %q", i, term.Op)
				}
			}
		} else {
			// Legacy path: Comparator must be set and valid.
			if !validComps[cfg.Comparator] {
				return fmt.Errorf("unknown comparator %q", cfg.Comparator)
			}
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

	case domain.NodeLoop:
		var cfg domain.LoopConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("invalid loop config: %w", err)
		}
		if cfg.Mode != "range" {
			return fmt.Errorf("loop config mode must be \"range\", got %q", cfg.Mode)
		}
		if cfg.FormulaID == "" {
			return fmt.Errorf("loop config missing formulaId")
		}
		if cfg.Iterator == "" {
			return fmt.Errorf("loop config missing iterator")
		}
		if !isValidIdentifier(cfg.Iterator) {
			return fmt.Errorf("loop config iterator %q must be a valid identifier (letters, digits, underscores)", cfg.Iterator)
		}
		validAggs := map[string]bool{
			"sum": true, "product": true, "count": true, "avg": true,
			"min": true, "max": true, "last": true, "fold": true,
		}
		if !validAggs[cfg.Aggregation] {
			return fmt.Errorf("loop config has invalid aggregation %q", cfg.Aggregation)
		}
		if cfg.Aggregation == "fold" {
			if cfg.AccumulatorVar == "" {
				return fmt.Errorf("loop config with fold aggregation requires accumulatorVar")
			}
			if !isValidIdentifier(cfg.AccumulatorVar) {
				return fmt.Errorf("loop config accumulatorVar %q must be a valid identifier", cfg.AccumulatorVar)
			}
			if cfg.AccumulatorVar == cfg.Iterator {
				return fmt.Errorf("loop config accumulatorVar must differ from iterator")
			}
			if cfg.InitValue != "" {
				if _, err := decimal.NewFromString(cfg.InitValue); err != nil {
					return fmt.Errorf("loop config initValue %q is not a valid decimal", cfg.InitValue)
				}
			}
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
			var condCfg domain.ConditionalConfig
			// Tolerate unmarshal failures here — the config-level validator
			// above already reports them; we just default to legacy port set
			// so the user does not get cascaded "missing port" noise on top
			// of the real error.
			_ = json.Unmarshal(n.Config, &condCfg)

			// Then/else are required by both branches.
			for _, port := range []string{"thenValue", "elseValue"} {
				if !hasPort(port) {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Message: fmt.Sprintf("conditional node missing '%s' input connection", port),
					})
				}
			}

			if len(condCfg.Conditions) > 0 {
				// Composite path: each term i needs `condition_i` and `conditionRight_i`.
				for i := range condCfg.Conditions {
					leftPort := fmt.Sprintf("condition_%d", i)
					rightPort := fmt.Sprintf("conditionRight_%d", i)
					if !hasPort(leftPort) {
						errs = append(errs, ValidationError{
							NodeID:  n.ID,
							Message: fmt.Sprintf("conditional node missing '%s' input connection", leftPort),
						})
					}
					if !hasPort(rightPort) {
						errs = append(errs, ValidationError{
							NodeID:  n.ID,
							Message: fmt.Sprintf("conditional node missing '%s' input connection", rightPort),
						})
					}
				}
			} else {
				// Legacy path: single condition / conditionRight.
				for _, port := range []string{"condition", "conditionRight"} {
					if !hasPort(port) {
						errs = append(errs, ValidationError{
							NodeID:  n.ID,
							Message: fmt.Sprintf("conditional node missing '%s' input connection", port),
						})
					}
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

		case domain.NodeLoop:
			if !hasPort("start") {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: "loop node missing 'start' input connection",
				})
			}
			if !hasPort("end") {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Message: "loop node missing 'end' input connection",
				})
			}
			// 'step' is optional; no error if absent.
		}
	}

	return errs
}
