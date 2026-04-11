package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// Evaluator computes the result of a single FormulaNode given its resolved
// input values. Input values are keyed by target port name.
//
// TableResolver is optional and only required when the formula contains
// NodeTableAggregate nodes. NodeTableLookup uses pre-loaded data injected
// into inputs (the "table:<key>" entries built by preloadTableData), so
// the legacy lookup path does not depend on this field.
type Evaluator struct {
	Precision     PrecisionConfig
	TableResolver TableResolver
}

// NewEvaluator creates an Evaluator with the given precision configuration.
// The optional resolver is used by NodeTableAggregate to scan full table
// rows at evaluation time. Pass nil if no aggregate nodes will run.
func NewEvaluator(precision PrecisionConfig, resolver TableResolver) *Evaluator {
	return &Evaluator{Precision: precision, TableResolver: resolver}
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
	case domain.NodeTableAggregate:
		return ev.evalTableAggregate(node, inputs)
	case domain.NodeConditional:
		return ev.evalConditional(node, inputs)
	case domain.NodeAggregate:
		return ev.evalAggregate(node, inputs)
	case domain.NodeSubFormula:
		// Sub-formula nodes are evaluated by the engine layer which resolves
		// the referenced formula and recursively calculates it. By the time
		// the evaluator sees this node, the result should already be seeded
		// as the "in" port by the executor.
		return ev.evalSubFormula(node, inputs)
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
// One input port per key column provides the lookup values; together they form a
// composite key ("|"-joined) that maps to a pre-loaded "table:<compositeKey>" entry.
func (ev *Evaluator) evalTableLookup(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.TableLookupConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid tableLookup config: %w", node.ID, err)
	}

	keyColumns := cfg.EffectiveKeyColumns()
	parts := make([]string, 0, len(keyColumns))
	for _, kc := range keyColumns {
		v, ok := inputs[kc]
		if !ok {
			return Zero, fmt.Errorf("node %s: missing %q input for table lookup", node.ID, kc)
		}
		parts = append(parts, v.String())
	}
	compositeKey := strings.Join(parts, "|")

	tableKey := "table:" + compositeKey
	val, ok := inputs[tableKey]
	if !ok {
		return Zero, fmt.Errorf("node %s: no table entry for key %q in table %s", node.ID, compositeKey, cfg.TableID)
	}
	return val, nil
}

// evalTableAggregate evaluates a NodeTableAggregate (task #040, spec 004).
//
// The semantics are SQL-like:
//
//	SELECT <aggregate>(<expression>)
//	FROM <tableId>
//	WHERE <filters joined by FilterCombinator>
//
// v1 only supports a single column name in cfg.Expression. Filters can
// reference either literal values or other node outputs via InputPort.
// Empty result-set semantics: count→0, sum→0, product→1; avg/min/max
// return an error so the user notices the empty filter.
func (ev *Evaluator) evalTableAggregate(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.TableAggregateConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid tableAggregate config: %w", node.ID, err)
	}
	if cfg.TableID == "" {
		return Zero, fmt.Errorf("node %s: tableAggregate missing tableId", node.ID)
	}
	if cfg.Expression == "" {
		return Zero, fmt.Errorf("node %s: tableAggregate missing expression (column name)", node.ID)
	}
	if ev.TableResolver == nil {
		return Zero, fmt.Errorf("node %s: tableAggregate requires a TableResolver but none was configured on the engine", node.ID)
	}

	// Background context here is intentional: the resolver's GetRows
	// hits a process-local cache (task #037), so request cancellation
	// would only affect the very first cold load. Aggregating over a
	// shared cached slice has no I/O.
	rows, err := ev.TableResolver.GetRows(context.Background(), cfg.TableID)
	if err != nil {
		return Zero, fmt.Errorf("node %s: load table %s: %w", node.ID, cfg.TableID, err)
	}

	combinator := cfg.FilterCombinator
	if combinator == "" {
		combinator = "and"
	}
	if combinator != "and" && combinator != "or" {
		return Zero, fmt.Errorf("node %s: unknown filterCombinator %q", node.ID, combinator)
	}

	// Walk every row, track three things separately so count and the
	// numeric reductions get coherent semantics across columns of any
	// type:
	//
	//   presentCount — rows that pass the filter AND have the
	//     expression column present in the row map. This is what
	//     'count' uses, mirroring SQL `COUNT(column)` (NULLs/missing
	//     cells are excluded, regardless of whether the value parses
	//     as numeric).
	//
	//   values — rows where the column is present AND parses as a
	//     Decimal. Used by sum / product / avg / min / max. Rows
	//     with non-numeric or missing values are silently skipped,
	//     which is the "ignore empty cell" behavior chain-ladder
	//     triangles depend on.
	//
	// Counting on a text column (e.g. region codes) returns
	// presentCount — that's the codex round 1 P2 fix; previously
	// count fell off the slice population step and returned 0.
	values := make([]Decimal, 0, len(rows))
	presentCount := 0
	for _, row := range rows {
		ok, ferr := matchTableFilters(row, cfg.Filters, combinator, inputs)
		if ferr != nil {
			return Zero, fmt.Errorf("node %s: %w", node.ID, ferr)
		}
		if !ok {
			continue
		}
		raw, present := row[cfg.Expression]
		if !present {
			continue
		}
		presentCount++
		d, err := decimal.NewFromString(raw)
		if err != nil {
			continue
		}
		values = append(values, d)
	}

	return ev.aggregateDecimalValues(node.ID, cfg.Aggregate, values, presentCount)
}

// matchTableFilters runs every filter against a row and combines the
// results with the supplied combinator. The filter's right-hand side
// is either a literal Value or a dynamic InputPort lookup; mixing both
// on the same filter is rejected as a config error.
func matchTableFilters(row map[string]string, filters []domain.TableFilter, combinator string, inputs map[string]Decimal) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}

	var running bool
	for i, f := range filters {
		if f.Value != "" && f.InputPort != "" {
			return false, fmt.Errorf("filter %d on column %s: cannot set both value and inputPort", i, f.Column)
		}
		cellVal, present := row[f.Column]
		// A row without the filter column never matches; it's not an
		// error (sparse triangle data is the whole point).
		if !present {
			if combinator == "or" {
				if i == 0 {
					running = false
				}
				continue
			}
			running = false
			continue
		}

		// Resolve the right-hand side.
		var rhs string
		if f.InputPort != "" {
			d, ok := inputs[f.InputPort]
			if !ok {
				return false, fmt.Errorf("filter %d on column %s: inputPort %q not connected", i, f.Column, f.InputPort)
			}
			rhs = d.String()
		} else {
			rhs = f.Value
		}

		// Compare. We try numeric first; if either side fails to parse
		// as Decimal we fall back to string compare, which only makes
		// sense for eq/ne.
		match, cerr := compareCell(cellVal, f.Op, rhs)
		if cerr != nil {
			return false, fmt.Errorf("filter %d on column %s: %w", i, f.Column, cerr)
		}
		if f.Negate {
			match = !match
		}
		if i == 0 {
			running = match
			continue
		}
		if combinator == "or" {
			running = running || match
		} else {
			running = running && match
		}
	}
	return running, nil
}

// compareCell does the actual comparison for a TableFilter. Numeric on
// both sides goes through Decimal; otherwise eq/ne fall back to string
// equality and the ordering ops (gt/ge/lt/le) error out.
func compareCell(left, op, right string) (bool, error) {
	leftDec, lerr := decimal.NewFromString(left)
	rightDec, rerr := decimal.NewFromString(right)
	if lerr == nil && rerr == nil {
		return compareDecimals("tableFilter", op, leftDec, rightDec)
	}
	switch op {
	case "eq":
		return left == right, nil
	case "ne":
		return left != right, nil
	default:
		return false, fmt.Errorf("op %q on non-numeric column requires eq/ne", op)
	}
}

// aggregateDecimalValues reduces a slice of numeric values using the named
// aggregate. presentCount is the number of selected rows whose expression
// column was present (regardless of whether the value parsed as numeric).
// Count uses presentCount, mirroring SQL `COUNT(column)` semantics; the
// numeric reductions use the values slice, which is necessarily a subset
// of the present rows (only those whose value parsed as Decimal).
//
// Empty input semantics: count → 0 (presentCount), sum → 0, product → 1.
// avg/min/max return an error on an empty values slice so the user notices.
//
// avg uses DivRound with the engine's intermediate precision so that the
// new node's averaging behaves identically to the legacy aggregate node
// (which also routes division through DivRound). Without this, shopspring's
// global DivisionPrecision (16 digits) would override the user's configured
// ENGINE_INTERMEDIATE_PRECISION setting.
func (ev *Evaluator) aggregateDecimalValues(nodeID, mode string, values []Decimal, presentCount int) (Decimal, error) {
	switch mode {
	case "count":
		return decimal.NewFromInt(int64(presentCount)), nil
	case "sum":
		acc := Zero
		for _, v := range values {
			acc = acc.Add(v)
		}
		return acc, nil
	case "product":
		acc := One
		for _, v := range values {
			acc = acc.Mul(v)
		}
		return acc, nil
	case "avg":
		if len(values) == 0 {
			return Zero, fmt.Errorf("node %s: avg over empty selection", nodeID)
		}
		acc := Zero
		for _, v := range values {
			acc = acc.Add(v)
		}
		return acc.DivRound(decimal.NewFromInt(int64(len(values))), ev.Precision.IntermediatePrecision), nil
	case "min":
		if len(values) == 0 {
			return Zero, fmt.Errorf("node %s: min over empty selection", nodeID)
		}
		m := values[0]
		for _, v := range values[1:] {
			if v.LessThan(m) {
				m = v
			}
		}
		return m, nil
	case "max":
		if len(values) == 0 {
			return Zero, fmt.Errorf("node %s: max over empty selection", nodeID)
		}
		m := values[0]
		for _, v := range values[1:] {
			if v.GreaterThan(m) {
				m = v
			}
		}
		return m, nil
	default:
		return Zero, fmt.Errorf("node %s: unknown aggregate %q", nodeID, mode)
	}
}

// evalConditional evaluates a conditional (if-then-else) node. Two
// branches are supported:
//
//   - Legacy single comparison (cfg.Conditions empty): reads ports
//     "condition" and "conditionRight", uses cfg.Comparator. Original
//     behavior, kept untouched for backward compatibility.
//
//   - Composite (cfg.Conditions non-empty): for each ConditionTerm i,
//     reads ports "condition_i" and "conditionRight_i", applies the
//     term's Op (and Negate flag), then joins all terms with the
//     configured Combinator ("and" default, or "or"). Combinator is
//     uniform across the term list — mixed AND/OR must be expressed
//     by nesting Conditional nodes.
//
// Both branches still consult "thenValue" / "elseValue" for outputs.
func (ev *Evaluator) evalConditional(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	var cfg domain.ConditionalConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return Zero, fmt.Errorf("node %s: invalid conditional config: %w", node.ID, err)
	}

	thenVal, okThen := inputs["thenValue"]
	elseVal, okElse := inputs["elseValue"]
	if !okThen || !okElse {
		return Zero, fmt.Errorf("node %s: conditional requires 'thenValue' and 'elseValue' inputs", node.ID)
	}

	var result bool
	var err error
	if len(cfg.Conditions) > 0 {
		result, err = ev.evalCompositeConditions(node.ID, cfg, inputs)
	} else {
		result, err = ev.evalLegacySingleCondition(node.ID, cfg, inputs)
	}
	if err != nil {
		return Zero, err
	}

	if result {
		return thenVal, nil
	}
	return elseVal, nil
}

// evalLegacySingleCondition implements the original single-comparison
// behavior used when ConditionalConfig.Conditions is empty.
func (ev *Evaluator) evalLegacySingleCondition(nodeID string, cfg domain.ConditionalConfig, inputs map[string]Decimal) (bool, error) {
	left, l := inputs["condition"]
	right, r := inputs["conditionRight"]
	if !l || !r {
		return false, fmt.Errorf("node %s: conditional requires 'condition' and 'conditionRight' inputs", nodeID)
	}
	return compareDecimals(nodeID, cfg.Comparator, left, right)
}

// evalCompositeConditions evaluates each ConditionTerm, applies Negate,
// and joins them with cfg.Combinator. The first term seeds the running
// truth value; subsequent terms AND or OR into it. We could short-circuit
// once the result is decided, but every input has already been evaluated
// up the DAG (levels-based execution), so the only thing to save is a
// per-term comparison call — micro-optimization, not worth the branch
// asymmetry, so we keep the loop straight-through.
func (ev *Evaluator) evalCompositeConditions(nodeID string, cfg domain.ConditionalConfig, inputs map[string]Decimal) (bool, error) {
	combinator := cfg.Combinator
	if combinator == "" {
		combinator = "and"
	}
	if combinator != "and" && combinator != "or" {
		return false, fmt.Errorf("node %s: unknown combinator %q", nodeID, combinator)
	}

	var running bool
	for i, term := range cfg.Conditions {
		leftKey := fmt.Sprintf("condition_%d", i)
		rightKey := fmt.Sprintf("conditionRight_%d", i)
		left, l := inputs[leftKey]
		right, r := inputs[rightKey]
		if !l || !r {
			return false, fmt.Errorf("node %s: composite conditional term %d requires '%s' and '%s' inputs", nodeID, i, leftKey, rightKey)
		}
		cmp, err := compareDecimals(nodeID, term.Op, left, right)
		if err != nil {
			return false, err
		}
		if term.Negate {
			cmp = !cmp
		}
		if i == 0 {
			running = cmp
			continue
		}
		if combinator == "or" {
			running = running || cmp
		} else {
			running = running && cmp
		}
	}
	return running, nil
}

// compareDecimals applies a single comparison operator to two Decimal
// values. Shared between the legacy single-condition path and every
// term in the composite path so the operator semantics stay in one
// place.
func compareDecimals(nodeID, op string, left, right Decimal) (bool, error) {
	switch op {
	case "eq":
		return left.Equal(right), nil
	case "ne":
		return !left.Equal(right), nil
	case "gt":
		return left.GreaterThan(right), nil
	case "ge":
		return left.GreaterThanOrEqual(right), nil
	case "lt":
		return left.LessThan(right), nil
	case "le":
		return left.LessThanOrEqual(right), nil
	default:
		return false, fmt.Errorf("node %s: unknown comparator %q", nodeID, op)
	}
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

// evalSubFormula handles sub-formula reference nodes. The actual recursive
// calculation is done at the engine level; by the time the evaluator sees this
// node, the result should be provided as the "in" input port.
func (ev *Evaluator) evalSubFormula(node *domain.FormulaNode, inputs map[string]Decimal) (Decimal, error) {
	val, ok := inputs["in"]
	if !ok {
		return Zero, fmt.Errorf("node %s: sub-formula result not provided (missing 'in' input)", node.ID)
	}
	return val, nil
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

