package engine

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/shopspring/decimal"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// Evaluator computes the result of a single FormulaNode given its resolved
// input values. Input values are keyed by target port name.
type Evaluator struct {
	Precision PrecisionConfig
}

// NewEvaluator creates an Evaluator with the given precision configuration.
func NewEvaluator(precision PrecisionConfig) *Evaluator {
	return &Evaluator{Precision: precision}
}

// EvaluateNode computes the output of the given node. The inputs map is keyed
// by target port name (e.g. "left", "right", "in", "key", "condition",
// "thenValue", "elseValue", "items").
func (ev *Evaluator) EvaluateNode(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	switch node.Type {
	case domain.NodeVariable:
		return ev.evalVariable(node, inputs)
	case domain.NodeConstant:
		return ev.evalConstant(node)
	case domain.NodeOperator:
		return ev.evalOperator(node, inputs)
	case domain.NodeFunction:
		return ev.evalFunction(node, inputs)
	case domain.NodeTableLookup:
		return ev.evalTableLookup(node, inputs)
	case domain.NodeConditional:
		return ev.evalConditional(node, inputs)
	case domain.NodeAggregate:
		return ev.evalAggregate(node, inputs)
	default:
		return Zero, fmt.Errorf("unsupported node type %q for node %s", node.Type, node.ID)
	}
}

// evalVariable looks up the named variable value from the inputs map.
func (ev *Evaluator) evalVariable(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.VariableConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid variable config: %w", node.ID, err)
	}
	val, ok := inputs[cfg.Name]
	if !ok {
		return Zero, fmt.Errorf("node %s: missing input variable %q", node.ID, cfg.Name)
	}
	return val, nil
}

// evalConstant parses the constant value from the node config.
func (ev *Evaluator) evalConstant(node *domain.FormulaNode) (Decimal, error) {
	var cfg domain.ConstantConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid constant config: %w", node.ID, err)
	}
	d, err := decimal.NewFromString(cfg.Value)
	if err != nil {
		return Zero, fmt.Errorf("node %s: cannot parse constant %q: %w", node.ID, cfg.Value, err)
	}
	return d, nil
}

// evalOperator applies a binary operator to the "left" and "right" inputs.
func (ev *Evaluator) evalOperator(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.OperatorConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid operator config: %w", node.ID, err)
	}

	left, ok := inputs["left"]
	if !ok {
		return Zero, fmt.Errorf("node %s: missing 'left' input", node.ID)
	}
	right, ok := inputs["right"]
	if !ok {
		return Zero, fmt.Errorf("node %s: missing 'right' input", node.ID)
	}

	switch cfg.Op {
	case "add":
		return left.Add(right), nil
	case "subtract":
		return left.Sub(right), nil
	case "multiply":
		return left.Mul(right), nil
	case "divide":
		if right.IsZero() {
			return Zero, fmt.Errorf("node %s: division by zero", node.ID)
		}
		return left.DivRound(right, ev.Precision.IntermediatePrecision), nil
	case "power":
		return decimalPow(left, right, ev.Precision.IntermediatePrecision), nil
	case "modulo":
		if right.IsZero() {
			return Zero, fmt.Errorf("node %s: modulo by zero", node.ID)
		}
		return left.Mod(right), nil
	default:
		return Zero, fmt.Errorf("node %s: unknown operator %q", node.ID, cfg.Op)
	}
}

// evalFunction applies a mathematical function to its inputs.
func (ev *Evaluator) evalFunction(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.FunctionConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid function config: %w", node.ID, err)
	}

	in, hasIn := inputs["in"]

	switch cfg.Fn {
	case "round":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for round", node.ID)
		}
		places := int32(0)
		if p, ok := cfg.Args["places"]; ok {
			pd, err := decimal.NewFromString(p)
			if err == nil {
				places = int32(pd.IntPart())
			}
		}
		return in.Round(places), nil

	case "floor":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for floor", node.ID)
		}
		return in.Floor(), nil

	case "ceil":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for ceil", node.ID)
		}
		return in.Ceil(), nil

	case "abs":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for abs", node.ID)
		}
		return in.Abs(), nil

	case "min":
		left, ok1 := inputs["left"]
		right, ok2 := inputs["right"]
		if !ok1 || !ok2 {
			return Zero, fmt.Errorf("node %s: min requires 'left' and 'right' inputs", node.ID)
		}
		if left.LessThan(right) {
			return left, nil
		}
		return right, nil

	case "max":
		left, ok1 := inputs["left"]
		right, ok2 := inputs["right"]
		if !ok1 || !ok2 {
			return Zero, fmt.Errorf("node %s: max requires 'left' and 'right' inputs", node.ID)
		}
		if left.GreaterThan(right) {
			return left, nil
		}
		return right, nil

	case "sqrt":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for sqrt", node.ID)
		}
		if in.IsNegative() {
			return Zero, fmt.Errorf("node %s: sqrt of negative number", node.ID)
		}
		f, _ := in.Float64()
		return decimal.NewFromFloat(math.Sqrt(f)), nil

	case "ln":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for ln", node.ID)
		}
		if !in.IsPositive() {
			return Zero, fmt.Errorf("node %s: ln of non-positive number", node.ID)
		}
		f, _ := in.Float64()
		return decimal.NewFromFloat(math.Log(f)), nil

	case "exp":
		if !hasIn {
			return Zero, fmt.Errorf("node %s: missing 'in' input for exp", node.ID)
		}
		f, _ := in.Float64()
		return decimal.NewFromFloat(math.Exp(f)), nil

	default:
		return Zero, fmt.Errorf("node %s: unknown function %q", node.ID, cfg.Fn)
	}
}

// evalTableLookup looks up a value from table data provided in the inputs map.
// The "key" input contains the lookup key value. The table data is expected to
// be pre-loaded into the inputs map as encoded column values.
func (ev *Evaluator) evalTableLookup(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.TableLookupConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid tableLookup config: %w", node.ID, err)
	}

	key, ok := inputs["key"]
	if !ok {
		return Zero, fmt.Errorf("node %s: missing 'key' input for table lookup", node.ID)
	}

	// Table data is provided as inputs keyed by "table:<key_value>", where
	// the value is the result column entry for that row.
	tableKey := "table:" + key.String()
	val, ok := inputs[tableKey]
	if !ok {
		return Zero, fmt.Errorf("node %s: no table entry for key %s in table %s", node.ID, key.String(), cfg.TableID)
	}
	return val, nil
}

// evalConditional evaluates a conditional (if-then-else) node.
func (ev *Evaluator) evalConditional(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.ConditionalConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid conditional config: %w", node.ID, err)
	}

	condLeft, ok1 := inputs["condition"]
	condRight, ok2 := inputs["conditionRight"]
	if !ok1 || !ok2 {
		return Zero, fmt.Errorf("node %s: conditional requires 'condition' and 'conditionRight' inputs", node.ID)
	}
	thenVal, ok3 := inputs["thenValue"]
	elseVal, ok4 := inputs["elseValue"]
	if !ok3 || !ok4 {
		return Zero, fmt.Errorf("node %s: conditional requires 'thenValue' and 'elseValue' inputs", node.ID)
	}

	var result bool
	switch cfg.Comparator {
	case "eq":
		result = condLeft.Equal(condRight)
	case "ne":
		result = !condLeft.Equal(condRight)
	case "gt":
		result = condLeft.GreaterThan(condRight)
	case "ge":
		result = condLeft.GreaterThanOrEqual(condRight)
	case "lt":
		result = condLeft.LessThan(condRight)
	case "le":
		result = condLeft.LessThanOrEqual(condRight)
	default:
		return Zero, fmt.Errorf("node %s: unknown comparator %q", node.ID, cfg.Comparator)
	}

	if result {
		return thenVal, nil
	}
	return elseVal, nil
}

// evalAggregate computes an aggregate function over a set of item values.
// Items are provided in the inputs map with keys "items:0", "items:1", etc.
func (ev *Evaluator) evalAggregate(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.AggregateConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid aggregate config: %w", node.ID, err)
	}

	// Collect items from the inputs map.
	items := collectItems(inputs)
	if len(items) == 0 {
		return Zero, fmt.Errorf("node %s: no items provided for aggregate", node.ID)
	}

	switch cfg.Fn {
	case "sum":
		acc := Zero
		for _, v := range items {
			acc = acc.Add(v)
		}
		return acc, nil

	case "product":
		acc := One
		for _, v := range items {
			acc = acc.Mul(v)
		}
		return acc, nil

	case "count":
		return NewDecimalFromInt(int64(len(items))), nil

	case "avg":
		acc := Zero
		for _, v := range items {
			acc = acc.Add(v)
		}
		count := decimal.NewFromInt(int64(len(items)))
		return acc.DivRound(count, ev.Precision.IntermediatePrecision), nil

	default:
		return Zero, fmt.Errorf("node %s: unknown aggregate function %q", node.ID, cfg.Fn)
	}
}

// collectItems gathers values from the inputs map that are keyed with the
// "items:" prefix, in order.
func collectItems(inputs map[string]Decimal) []Decimal {
	var items []Decimal
	for i := 0; ; i++ {
		key := fmt.Sprintf("items:%d", i)
		v, ok := inputs[key]
		if !ok {
			break
		}
		items = append(items, v)
	}
	// Fallback: if no indexed items found, check for a single "items" key.
	if len(items) == 0 {
		if v, ok := inputs["items"]; ok {
			items = append(items, v)
		}
	}
	return items
}

// decimalPow raises base to the power of exp. For integer exponents it uses
// repeated multiplication; for fractional exponents it falls back to float64.
func decimalPow(base, exp Decimal, precision int32) Decimal {
	// Check for integer exponent.
	if exp.Equal(exp.Truncate(0)) {
		n := exp.IntPart()
		if n == 0 {
			return One
		}
		negative := n < 0
		if negative {
			n = -n
		}
		result := One
		b := base
		for n > 0 {
			if n%2 == 1 {
				result = result.Mul(b)
			}
			b = b.Mul(b)
			n /= 2
		}
		if negative {
			if result.IsZero() {
				return Zero
			}
			return One.DivRound(result, precision)
		}
		return result
	}

	// Fractional exponent: fall back to float64.
	bf, _ := base.Float64()
	ef, _ := exp.Float64()
	return decimal.NewFromFloat(math.Pow(bf, ef))
}

