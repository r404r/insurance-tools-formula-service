package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

type typedCachedResult struct {
	Outputs        map[string]string
	Intermediates  map[string]string
	NodesEvaluated int
	ParallelLevels int
}

// typedResultCache keeps wire-form results separate from ResultCache, whose
// public implementation stores Decimal maps for the legacy execution path.
type typedResultCache struct {
	mu      sync.RWMutex
	items   map[string]typedCachedResult
	maxSize int
}

func newTypedResultCache(maxSize int) *typedResultCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &typedResultCache{items: make(map[string]typedCachedResult), maxSize: maxSize}
}

func (c *typedResultCache) get(key CacheKey) (typedCachedResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.items[key.String()]
	if !ok {
		return typedCachedResult{}, false
	}
	result.Outputs = copyStringResults(result.Outputs)
	result.Intermediates = copyStringResults(result.Intermediates)
	return result, true
}

func (c *typedResultCache) set(key CacheKey, result typedCachedResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.maxSize {
		for oldKey := range c.items {
			delete(c.items, oldKey)
			break
		}
	}
	c.items[key.String()] = typedCachedResult{
		Outputs:        copyStringResults(result.Outputs),
		Intermediates:  copyStringResults(result.Intermediates),
		NodesEvaluated: result.NodesEvaluated,
		ParallelLevels: result.ParallelLevels,
	}
}

func (c *typedResultCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]typedCachedResult)
}

func (c *typedResultCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *typedResultCache) deleteString(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func copyStringResults(input map[string]string) map[string]string {
	copy := make(map[string]string, len(input))
	for key, value := range input {
		copy[key] = value
	}
	return copy
}

func computeRawInputHash(inputs map[string]string) string {
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		fmt.Fprintf(hash, "%s=%s;", key, inputs[key])
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// graphUsesTypedValues opts a graph into tagged-value evaluation only when it
// declares an integer, string, or boolean variable.  This keeps established
// decimal-only formulas on the existing precision- and cache-tested path.
func graphUsesTypedValues(graph *domain.FormulaGraph) bool {
	if graph == nil {
		return false
	}
	for _, node := range graph.Nodes {
		if node.Type != domain.NodeVariable {
			continue
		}
		var cfg domain.VariableConfig
		if json.Unmarshal(node.Config, &cfg) == nil {
			switch cfg.DataType {
			case "integer", "string", "boolean":
				return true
			}
		}
	}
	return false
}

func (e *defaultEngine) calculateTyped(ctx context.Context, graph *domain.FormulaGraph, rawInputs map[string]string) (*CalculationResult, error) {
	start := time.Now()
	formulaID, err := graphHash(graph)
	if err != nil {
		return nil, fmt.Errorf("hash graph: %w", err)
	}
	cacheKey := CacheKey{
		FormulaID: formulaID,
		Version:   "typed:" + precisionCacheVersion(e.config.Precision),
		InputHash: computeRawInputHash(rawInputs),
	}
	if cached, ok := e.typedCache.get(cacheKey); ok {
		return &CalculationResult{
			Outputs:        cached.Outputs,
			Intermediates:  cached.Intermediates,
			NodesEvaluated: cached.NodesEvaluated,
			ParallelLevels: cached.ParallelLevels,
			ExecutionTime:  time.Since(start),
			CacheHit:       true,
		}, nil
	}
	plan, err := BuildPlan(graph)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	results := make(map[string]Value, len(graph.Nodes))
	for _, level := range plan.Levels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, nodeID := range level {
			node := plan.DAG.Node(nodeID)
			inputs, err := typedNodeInputs(plan, nodeID, results)
			if err != nil {
				return nil, err
			}
			value, err := e.evaluateTypedNode(ctx, node, inputs, rawInputs)
			if err != nil {
				return nil, fmt.Errorf("evaluate node %s: %w", nodeID, err)
			}
			results[nodeID] = value
		}
	}

	outputs := make(map[string]string, len(graph.Outputs))
	for _, outputID := range plan.DAG.OutputNodes() {
		value, ok := results[outputID]
		if !ok {
			return nil, fmt.Errorf("output node %s was not evaluated", outputID)
		}
		outputs[outputID] = e.serializeTypedValue(value)
	}
	intermediates := make(map[string]string, len(results))
	for id, value := range results {
		intermediates[id] = e.serializeTypedValue(value)
	}
	result := &CalculationResult{
		Outputs:        outputs,
		Intermediates:  intermediates,
		NodesEvaluated: len(results),
		ParallelLevels: len(plan.Levels),
		ExecutionTime:  time.Since(start),
	}
	e.storeTypedCache(cacheKey, typedCachedResult{
		Outputs:        outputs,
		Intermediates:  intermediates,
		NodesEvaluated: result.NodesEvaluated,
		ParallelLevels: result.ParallelLevels,
	})
	return result, nil
}

func (e *defaultEngine) serializeTypedValue(value Value) string {
	if value.Kind == ValueDecimal {
		return e.config.Precision.RoundOutput(value.Decimal).String()
	}
	return value.WireString()
}

func typedNodeInputs(plan *ExecutionPlan, nodeID string, results map[string]Value) (map[string]Value, error) {
	inputs := make(map[string]Value)
	for _, edge := range plan.DAG.IncomingEdges(nodeID) {
		value, ok := results[edge.Source]
		if !ok {
			return nil, fmt.Errorf("node %s: dependency %s not yet computed", nodeID, edge.Source)
		}
		port := edge.TargetPort
		if port == "" {
			port = "in"
		}
		inputs[port] = value
	}
	return inputs, nil
}

func (e *defaultEngine) evaluateTypedNode(ctx context.Context, node *domain.FormulaNode, inputs map[string]Value, rawInputs map[string]string) (Value, error) {
	switch node.Type {
	case domain.NodeVariable:
		var cfg domain.VariableConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return Value{}, fmt.Errorf("invalid variable config: %w", err)
		}
		raw, ok := rawInputs[cfg.Name]
		if !ok {
			return Value{}, fmt.Errorf("missing input variable %q", cfg.Name)
		}
		return typedInput(raw, cfg.DataType)
	case domain.NodeConstant:
		var cfg domain.ConstantConfig
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return Value{}, fmt.Errorf("invalid constant config: %w", err)
		}
		return typedInput(cfg.Value, "decimal")
	case domain.NodeOperator:
		return e.evalTypedOperator(node, inputs)
	case domain.NodeFunction:
		return e.evalTypedFunction(node, inputs)
	case domain.NodeTableLookup:
		return e.evalTypedLookup(ctx, node, inputs)
	case domain.NodeConditional:
		return e.evalTypedConditional(node, inputs)
	case domain.NodeAggregate:
		return e.evalTypedAggregate(node, inputs)
	default:
		return Value{}, fmt.Errorf("node type %q is not supported by typed evaluation", node.Type)
	}
}

func numericTypedOperands(node *domain.FormulaNode, inputs map[string]Value) (Decimal, Decimal, error) {
	left, ok := inputs["left"]
	if !ok {
		return Zero, Zero, fmt.Errorf("missing 'left' input")
	}
	leftDecimal, leftOK := left.numeric()
	if !leftOK {
		return Zero, Zero, fmt.Errorf("numeric operands required; left is %s", left.Kind)
	}
	right, ok := inputs["right"]
	if !ok {
		return Zero, Zero, fmt.Errorf("missing 'right' input")
	}
	rightDecimal, rightOK := right.numeric()
	if !rightOK {
		return Zero, Zero, fmt.Errorf("numeric operands required; right is %s", right.Kind)
	}
	return leftDecimal, rightDecimal, nil
}

func (e *defaultEngine) evalTypedOperator(node *domain.FormulaNode, inputs map[string]Value) (Value, error) {
	var cfg domain.OperatorConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Value{}, fmt.Errorf("invalid operator config: %w", err)
	}
	if cfg.Op == "negate" {
		value, ok := inputs["left"]
		if !ok {
			return Value{}, fmt.Errorf("missing 'left' input")
		}
		d, ok := value.numeric()
		if !ok {
			return Value{}, fmt.Errorf("numeric operands required; left is %s", value.Kind)
		}
		return Value{Kind: value.Kind, Decimal: d.Neg()}, nil
	}
	left, right, err := numericTypedOperands(node, inputs)
	if err != nil {
		return Value{}, err
	}
	switch cfg.Op {
	case "add":
		return numericResult(inputs, left.Add(right)), nil
	case "subtract":
		return numericResult(inputs, left.Sub(right)), nil
	case "multiply":
		return numericResult(inputs, left.Mul(right)), nil
	case "divide":
		if right.IsZero() {
			return Value{}, fmt.Errorf("division by zero")
		}
		return Value{Kind: ValueDecimal, Decimal: left.DivRound(right, e.config.Precision.IntermediatePrecision)}, nil
	case "modulo":
		if right.IsZero() {
			return Value{}, fmt.Errorf("modulo by zero")
		}
		return numericResult(inputs, left.Mod(right)), nil
	case "power":
		out, err := decimalPow(left, right, e.config.Precision.IntermediatePrecision)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: ValueDecimal, Decimal: out}, nil
	default:
		return Value{}, fmt.Errorf("unknown operator %q", cfg.Op)
	}
}

func numericResult(inputs map[string]Value, result Decimal) Value {
	if inputs["left"].Kind == ValueInteger && inputs["right"].Kind == ValueInteger && result.Equal(result.Truncate(0)) {
		return Value{Kind: ValueInteger, Decimal: result}
	}
	return Value{Kind: ValueDecimal, Decimal: result}
}

func (e *defaultEngine) evalTypedFunction(node *domain.FormulaNode, inputs map[string]Value) (Value, error) {
	decimalInputs := make(map[string]Decimal, len(inputs))
	for port, value := range inputs {
		d, ok := value.numeric()
		if !ok {
			return Value{}, fmt.Errorf("numeric operands required; %s is %s", port, value.Kind)
		}
		decimalInputs[port] = d
	}
	value, err := NewEvaluator(e.config.Precision, e.tableResolver).evalFunction(node, decimalInputs)
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: ValueDecimal, Decimal: value}, nil
}

func (e *defaultEngine) evalTypedLookup(ctx context.Context, node *domain.FormulaNode, inputs map[string]Value) (Value, error) {
	if e.tableResolver == nil {
		return Value{}, fmt.Errorf("table lookup requires a TableResolver")
	}
	var cfg domain.TableLookupConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Value{}, fmt.Errorf("invalid tableLookup config: %w", err)
	}
	column := cfg.Column
	if column == "" {
		var err error
		column, err = e.resolveUniqueLookupColumn(ctx, cfg)
		if err != nil {
			return Value{}, err
		}
	}
	parts := make([]string, 0, len(cfg.EffectiveKeyColumns()))
	for _, column := range cfg.EffectiveKeyColumns() {
		value, ok := inputs[column]
		if !ok {
			return Value{}, fmt.Errorf("missing %q input for table lookup", column)
		}
		parts = append(parts, value.WireString())
	}
	values, err := e.tableResolver.ResolveTable(ctx, cfg.TableID, cfg.EffectiveKeyColumns(), column)
	if err != nil {
		return Value{}, fmt.Errorf("resolve table %s: %w", cfg.TableID, err)
	}
	value, ok := values[strings.Join(parts, "|")]
	if !ok {
		return Value{}, fmt.Errorf("no table entry for key %q in table %s", strings.Join(parts, "|"), cfg.TableID)
	}
	return Value{Kind: ValueString, String: value}, nil
}

func (e *defaultEngine) evalTypedConditional(node *domain.FormulaNode, inputs map[string]Value) (Value, error) {
	var cfg domain.ConditionalConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Value{}, fmt.Errorf("invalid conditional config: %w", err)
	}
	var matched bool
	var err error
	if len(cfg.Conditions) == 0 {
		matched, err = compareTypedInputs(node.ID, cfg.Comparator, inputs["condition"], inputs["conditionRight"], inputs)
	} else {
		matched, err = compareTypedConditions(node.ID, cfg, inputs)
	}
	if err != nil {
		return Value{}, err
	}
	thenValue, hasThen := inputs["thenValue"]
	elseValue, hasElse := inputs["elseValue"]
	if !hasThen && !hasElse && len(cfg.Conditions) == 0 {
		return Value{Kind: ValueBoolean, Bool: matched}, nil
	}
	if !hasThen || !hasElse {
		return Value{}, fmt.Errorf("conditional requires 'thenValue' and 'elseValue' inputs")
	}
	if matched {
		return thenValue, nil
	}
	return elseValue, nil
}

func compareTypedConditions(nodeID string, cfg domain.ConditionalConfig, inputs map[string]Value) (bool, error) {
	combinator := cfg.Combinator
	if combinator == "" {
		combinator = "and"
	}
	if combinator != "and" && combinator != "or" {
		return false, fmt.Errorf("unknown combinator %q", cfg.Combinator)
	}
	var result bool
	for i, term := range cfg.Conditions {
		left, leftOK := inputs[fmt.Sprintf("condition_%d", i)]
		right, rightOK := inputs[fmt.Sprintf("conditionRight_%d", i)]
		if !leftOK || !rightOK {
			return false, fmt.Errorf("composite conditional term %d has missing inputs", i)
		}
		matched, err := compareTyped(nodeID, term.Op, left, right)
		if err != nil {
			return false, err
		}
		if term.Negate {
			matched = !matched
		}
		if i == 0 {
			result = matched
		} else if combinator == "and" {
			result = result && matched
		} else {
			result = result || matched
		}
	}
	return result, nil
}

func compareTypedInputs(nodeID, op string, left, right Value, inputs map[string]Value) (bool, error) {
	if _, ok := inputs["condition"]; !ok {
		return false, fmt.Errorf("conditional requires 'condition' and 'conditionRight' inputs")
	}
	if _, ok := inputs["conditionRight"]; !ok {
		return false, fmt.Errorf("conditional requires 'condition' and 'conditionRight' inputs")
	}
	return compareTyped(nodeID, op, left, right)
}

func compareTyped(nodeID, op string, left, right Value) (bool, error) {
	leftNumeric, leftIsNumeric := left.numeric()
	rightNumeric, rightIsNumeric := right.numeric()
	if leftIsNumeric && rightIsNumeric {
		return compareDecimals(nodeID, op, leftNumeric, rightNumeric)
	}
	if op == "eq" {
		return left.Kind == right.Kind && left.WireString() == right.WireString(), nil
	}
	if op == "ne" {
		return left.Kind != right.Kind || left.WireString() != right.WireString(), nil
	}
	return false, fmt.Errorf("comparator %q requires numeric operands; got %s and %s", op, left.Kind, right.Kind)
}

func (e *defaultEngine) evalTypedAggregate(node *domain.FormulaNode, inputs map[string]Value) (Value, error) {
	var cfg domain.AggregateConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Value{}, fmt.Errorf("invalid aggregate config: %w", err)
	}
	keys := make([]string, 0)
	for key := range inputs {
		if key == "items" || strings.HasPrefix(key, "items:") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return Value{}, fmt.Errorf("no items provided for aggregate")
	}
	values := make([]Decimal, 0, len(keys))
	for _, key := range keys {
		value, ok := inputs[key].numeric()
		if !ok {
			return Value{}, fmt.Errorf("numeric operands required; %s is %s", key, inputs[key].Kind)
		}
		values = append(values, value)
	}
	switch cfg.Fn {
	case "sum":
		acc := Zero
		for _, value := range values {
			acc = acc.Add(value)
		}
		return Value{Kind: ValueDecimal, Decimal: acc}, nil
	case "product":
		acc := One
		for _, value := range values {
			acc = acc.Mul(value)
		}
		return Value{Kind: ValueDecimal, Decimal: acc}, nil
	case "count":
		return Value{Kind: ValueInteger, Decimal: decimal.NewFromInt(int64(len(values)))}, nil
	case "avg":
		acc := Zero
		for _, value := range values {
			acc = acc.Add(value)
		}
		return Value{Kind: ValueDecimal, Decimal: acc.DivRound(decimal.NewFromInt(int64(len(values))), e.config.Precision.IntermediatePrecision)}, nil
	default:
		return Value{}, fmt.Errorf("unknown aggregate function %q", cfg.Fn)
	}
}
