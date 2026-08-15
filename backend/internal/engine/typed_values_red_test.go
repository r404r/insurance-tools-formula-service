package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// These tests are the Red boundary for C-20260815-006.  Formula inputs are
// wire strings, but VariableConfig already promises four semantic types.  The
// engine must preserve those types internally rather than coercing every value
// to Decimal before graph evaluation.

func TestTypedVariableInputsRoundTripAsStrings(t *testing.T) {
	cases := []struct {
		name, dataType, input string
	}{
		{name: "decimal", dataType: "decimal", input: "12.5"},
		{name: "integer", dataType: "integer", input: "42"},
		{name: "string", dataType: "string", input: "preferred"},
		{name: "boolean", dataType: "boolean", input: "true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graph := typedVariableOutputGraph("value", tc.dataType)
			result, err := NewEngine(DefaultEngineConfig()).Calculate(
				context.Background(), graph, map[string]string{"value": tc.input},
			)
			if err != nil {
				t.Fatalf("Calculate(%s): %v", tc.dataType, err)
			}
			if got := result.Outputs["value"]; got != tc.input {
				t.Fatalf("output = %q, want wire string %q", got, tc.input)
			}
		})
	}
}

func TestTypedValuesUseRawInputCacheKey(t *testing.T) {
	engine := NewEngine(EngineConfig{CacheSize: 8})
	graph := typedVariableOutputGraph("value", "string")
	first, err := engine.Calculate(context.Background(), graph, map[string]string{"value": "01"})
	if err != nil {
		t.Fatalf("first calculate: %v", err)
	}
	if first.CacheHit {
		t.Fatal("first typed calculation unexpectedly hit cache")
	}
	second, err := engine.Calculate(context.Background(), graph, map[string]string{"value": "01"})
	if err != nil {
		t.Fatalf("second calculate: %v", err)
	}
	if !second.CacheHit {
		t.Fatal("identical raw typed input did not hit cache")
	}
	third, err := engine.Calculate(context.Background(), graph, map[string]string{"value": "1"})
	if err != nil {
		t.Fatalf("third calculate: %v", err)
	}
	if third.CacheHit {
		t.Fatal("distinct raw string input shared a typed cache entry")
	}
}

func TestTypedStringLookupAndBooleanConditional(t *testing.T) {
	t.Run("string lookup remains a string", func(t *testing.T) {
		graph := &domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "key", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "plan", DataType: "string"})},
				{ID: "lookup", Type: domain.NodeTableLookup, Config: mustJSON(domain.TableLookupConfig{TableID: "plans", KeyColumns: []string{"key"}, Column: "label"})},
			},
			Edges:   []domain.FormulaEdge{{Source: "key", Target: "lookup", TargetPort: "key"}},
			Outputs: []string{"lookup"},
		}
		resolver := typedLookupResolver{"plans|label": {"gold": "Preferred plan"}}
		result, err := NewEngine(EngineConfig{TableResolver: resolver}).Calculate(context.Background(), graph, map[string]string{"plan": "gold"})
		if err != nil {
			t.Fatalf("calculate string lookup: %v", err)
		}
		if got := result.Outputs["lookup"]; got != "Preferred plan" {
			t.Fatalf("lookup output = %q, want string value", got)
		}
	})

	t.Run("boolean equality controls string branches", func(t *testing.T) {
		graph := &domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "actual", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "actual", DataType: "boolean"})},
				{ID: "expected", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "expected", DataType: "boolean"})},
				{ID: "yes", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "yes", DataType: "string"})},
				{ID: "no", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "no", DataType: "string"})},
				{ID: "result", Type: domain.NodeConditional, Config: mustJSON(domain.ConditionalConfig{Comparator: "eq"})},
			},
			Edges: []domain.FormulaEdge{
				{Source: "actual", Target: "result", TargetPort: "condition"},
				{Source: "expected", Target: "result", TargetPort: "conditionRight"},
				{Source: "yes", Target: "result", TargetPort: "thenValue"},
				{Source: "no", Target: "result", TargetPort: "elseValue"},
			},
			Outputs: []string{"result"},
		}
		result, err := NewEngine(DefaultEngineConfig()).Calculate(context.Background(), graph, map[string]string{
			"actual": "true", "expected": "true", "yes": "approved", "no": "declined",
		})
		if err != nil {
			t.Fatalf("calculate boolean conditional: %v", err)
		}
		if got := result.Outputs["result"]; got != "approved" {
			t.Fatalf("conditional output = %q, want approved", got)
		}
	})
}

func TestTypedEqualityAndNumericTypeErrors(t *testing.T) {
	t.Run("string eq and ne use exact string comparison", func(t *testing.T) {
		for _, tc := range []struct {
			op, left, right string
			want            string
		}{
			{op: "eq", left: "A-01", right: "A-01", want: "matched"},
			{op: "ne", left: "A-01", right: "B-01", want: "matched"},
		} {
			graph := typedConditionalGraph(tc.op)
			result, err := NewEngine(DefaultEngineConfig()).Calculate(context.Background(), graph, map[string]string{
				"left": tc.left, "right": tc.right, "yes": "matched", "no": "missed",
			})
			if err != nil {
				t.Fatalf("%s: %v", tc.op, err)
			}
			if got := result.Outputs["result"]; got != tc.want {
				t.Fatalf("%s output = %q, want %q", tc.op, got, tc.want)
			}
		}
	})

	t.Run("arithmetic rejects string operand rather than coercing it", func(t *testing.T) {
		graph := &domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "text", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "text", DataType: "string"})},
				{ID: "amount", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "amount", DataType: "decimal"})},
				{ID: "add", Type: domain.NodeOperator, Config: mustJSON(domain.OperatorConfig{Op: "add"})},
			},
			Edges:   []domain.FormulaEdge{{Source: "text", Target: "add", TargetPort: "left"}, {Source: "amount", Target: "add", TargetPort: "right"}},
			Outputs: []string{"add"},
		}
		_, err := NewEngine(DefaultEngineConfig()).Calculate(context.Background(), graph, map[string]string{"text": "not-a-number", "amount": "1"})
		if err == nil || !strings.Contains(err.Error(), "numeric operands") {
			t.Fatalf("mixed arithmetic error = %v, want explicit numeric operands error", err)
		}
	})
}

func typedVariableOutputGraph(name, dataType string) *domain.FormulaGraph {
	return &domain.FormulaGraph{
		Nodes:   []domain.FormulaNode{{ID: name, Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: name, DataType: dataType})}},
		Outputs: []string{name},
	}
}

func typedConditionalGraph(op string) *domain.FormulaGraph {
	return &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "left", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "left", DataType: "string"})},
			{ID: "right", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "right", DataType: "string"})},
			{ID: "yes", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "yes", DataType: "string"})},
			{ID: "no", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "no", DataType: "string"})},
			{ID: "result", Type: domain.NodeConditional, Config: mustJSON(domain.ConditionalConfig{Comparator: op})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "left", Target: "result", TargetPort: "condition"},
			{Source: "right", Target: "result", TargetPort: "conditionRight"},
			{Source: "yes", Target: "result", TargetPort: "thenValue"},
			{Source: "no", Target: "result", TargetPort: "elseValue"},
		},
		Outputs: []string{"result"},
	}
}

type typedLookupResolver map[string]map[string]string

func (r typedLookupResolver) ResolveTable(_ context.Context, tableID string, _ []string, column string) (map[string]string, error) {
	values, ok := r[tableID+"|"+column]
	if !ok {
		return nil, fmt.Errorf("missing fixture for %s.%s", tableID, column)
	}
	return values, nil
}

func (typedLookupResolver) GetRows(context.Context, string) ([]map[string]string, error) {
	return nil, nil
}
