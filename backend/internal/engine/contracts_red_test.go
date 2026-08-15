package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/parser"
)

// These tests are the Red boundary for C-20260815-003. They deliberately
// exercise text -> graph -> execution whenever the user-visible text syntax
// is part of the contract, so a local evaluator-only fix cannot mask a broken
// serializer or execution-plan port convention.

func TestBuildDAGRejectsInvalidOutputAndNodeIdentity(t *testing.T) {
	constant := func(id string) domain.FormulaNode {
		return domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeConstant,
			Config: mustJSON(domain.ConstantConfig{Value: "1"}),
		}
	}

	cases := []struct {
		name  string
		graph *domain.FormulaGraph
		want  string
	}{
		{
			name:  "empty outputs",
			graph: &domain.FormulaGraph{Nodes: []domain.FormulaNode{constant("out")}},
			want:  "at least one output",
		},
		{
			name: "unknown output",
			graph: &domain.FormulaGraph{
				Nodes:   []domain.FormulaNode{constant("out")},
				Outputs: []string{"missing"},
			},
			want: "output references unknown node",
		},
		{
			name: "duplicate outputs",
			graph: &domain.FormulaGraph{
				Nodes:   []domain.FormulaNode{constant("out")},
				Outputs: []string{"out", "out"},
			},
			want: "duplicate output",
		},
		{
			name: "duplicate node ID",
			graph: &domain.FormulaGraph{
				Nodes:   []domain.FormulaNode{constant("out"), constant("out")},
				Outputs: []string{"out"},
			},
			want: "duplicate node ID",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildDAG(tc.graph)
			if err == nil {
				t.Fatalf("BuildDAG accepted invalid graph; want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("BuildDAG error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestTextUnaryNegateCalculates(t *testing.T) {
	graph := mustParseGraph(t, "-3")
	result, err := newContractEngine(nil).Calculate(context.Background(), graph, nil)
	if err != nil {
		t.Fatalf("calculate -3: %v", err)
	}
	if got := result.Outputs[graph.Outputs[0]]; got != "-3" {
		t.Fatalf("-3 output = %q, want %q", got, "-3")
	}
}

func TestStandaloneTextComparisonCalculatesToDecimalBoolean(t *testing.T) {
	cases := []struct {
		expression string
		want       string
	}{
		{expression: "3 > 2", want: "1"},
		{expression: "2 > 3", want: "0"},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			graph := mustParseGraph(t, tc.expression)
			result, err := newContractEngine(nil).Calculate(context.Background(), graph, nil)
			if err != nil {
				t.Fatalf("calculate %q: %v", tc.expression, err)
			}
			if got := result.Outputs[graph.Outputs[0]]; got != tc.want {
				t.Fatalf("%s output = %q, want %q", tc.expression, got, tc.want)
			}
		})
	}
}

func TestCanonicalVariadicAggregateTextCalculates(t *testing.T) {
	cases := []struct {
		expression string
		want       string
	}{
		{expression: "sum(1, 2, 3)", want: "6"},
		{expression: "avg(1, 2, 3)", want: "2"},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			graph := mustParseGraph(t, tc.expression)
			result, err := newContractEngine(nil).Calculate(context.Background(), graph, nil)
			if err != nil {
				t.Fatalf("calculate %q: %v", tc.expression, err)
			}
			if got := result.Outputs[graph.Outputs[0]]; got != tc.want {
				t.Fatalf("%s output = %q, want %q", tc.expression, got, tc.want)
			}
		})
	}
}

func TestFloorAndCeilHonorPlacesForPositiveAndNegativeValues(t *testing.T) {
	cases := []struct {
		fn     string
		input  string
		places string
		want   string
	}{
		{fn: "floor", input: "12.349", places: "2", want: "12.34"},
		{fn: "ceil", input: "12.341", places: "2", want: "12.35"},
		{fn: "floor", input: "-12.341", places: "2", want: "-12.35"},
		{fn: "ceil", input: "-12.349", places: "2", want: "-12.34"},
	}

	for _, tc := range cases {
		t.Run(tc.fn+"("+tc.input+", "+tc.places+")", func(t *testing.T) {
			node := &domain.FormulaNode{
				ID:     tc.fn,
				Type:   domain.NodeFunction,
				Config: mustJSON(domain.FunctionConfig{Fn: tc.fn, Args: map[string]string{"places": tc.places}}),
			}
			got, err := NewEvaluator(DefaultPrecision(), nil).EvaluateNode(node, map[string]Decimal{"in": NewDecimal(tc.input)})
			if err != nil {
				t.Fatalf("evaluate %s(%s, %s): %v", tc.fn, tc.input, tc.places, err)
			}
			if got.String() != tc.want {
				t.Fatalf("%s(%s, %s) = %q, want %q", tc.fn, tc.input, tc.places, got, tc.want)
			}
		})
	}
}

func TestLookupPreloadNamespaceIncludesTableAndColumn(t *testing.T) {
	cases := []struct {
		name       string
		leftTable  string
		leftColumn string
		rightTable string
		rightCol   string
	}{
		{
			name:       "two tables with the same key",
			leftTable:  "mortality",
			leftColumn: "value",
			rightTable: "expense",
			rightCol:   "value",
		},
		{
			name:       "one table with two columns",
			leftTable:  "rates",
			leftColumn: "net",
			rightTable: "rates",
			rightCol:   "gross",
		},
	}

	resolver := contractLookupResolver{
		"mortality|value": {"42": "10"},
		"expense|value":   {"42": "20"},
		"rates|net":       {"42": "10"},
		"rates|gross":     {"42": "20"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graph := lookupAdditionGraph(tc.leftTable, tc.leftColumn, tc.rightTable, tc.rightCol)
			result, err := newContractEngine(resolver).Calculate(context.Background(), graph, map[string]string{"key": "42"})
			if err != nil {
				t.Fatalf("calculate lookup graph: %v", err)
			}
			if got := result.Outputs["sum"]; got != "30" {
				t.Fatalf("lookup result = %q, want %q; values with the same key must remain isolated", got, "30")
			}
		})
	}
}

func TestTwoArgumentLookupSelectsOnlyUniqueValueColumn(t *testing.T) {
	t.Run("one non-key column is selected", func(t *testing.T) {
		repo := newFakeTableRepo()
		repo.put("rates", []map[string]string{{"key": "42", "value": "10"}})
		graph := mustParseGraph(t, "lookup(rates, key)")

		result, err := newContractEngine(&StoreTableResolver{Tables: repo}).Calculate(
			context.Background(), graph, map[string]string{"key": "42"},
		)
		if err != nil {
			t.Fatalf("calculate two-argument lookup with unique value column: %v", err)
		}
		if got := result.Outputs[graph.Outputs[0]]; got != "10" {
			t.Fatalf("two-argument lookup output = %q, want %q", got, "10")
		}
	})

	t.Run("multiple non-key columns are rejected explicitly", func(t *testing.T) {
		repo := newFakeTableRepo()
		repo.put("rates", []map[string]string{{"key": "42", "net": "10", "gross": "20"}})
		graph := mustParseGraph(t, "lookup(rates, key)")

		_, err := newContractEngine(&StoreTableResolver{Tables: repo}).Calculate(
			context.Background(), graph, map[string]string{"key": "42"},
		)
		if err == nil {
			t.Fatal("two-argument lookup with multiple value columns succeeded; want a deterministic ambiguity error")
		}
		if !strings.Contains(err.Error(), "ambiguous lookup column") {
			t.Fatalf("ambiguous two-argument lookup error = %q, want it to contain %q", err, "ambiguous lookup column")
		}
	})
}

func mustParseGraph(t *testing.T, expression string) *domain.FormulaGraph {
	t.Helper()
	ast, err := parser.NewParser(expression).Parse()
	if err != nil {
		t.Fatalf("parse %q: %v", expression, err)
	}
	graph, err := parser.ASTToDAG(ast)
	if err != nil {
		t.Fatalf("ASTToDAG %q: %v", expression, err)
	}
	return graph
}

func newContractEngine(resolver TableResolver) Engine {
	return NewEngine(EngineConfig{
		Workers:       1,
		Precision:     DefaultPrecision(),
		CacheSize:     0,
		TableResolver: resolver,
	})
}

type contractLookupResolver map[string]map[string]string

func (r contractLookupResolver) ResolveTable(_ context.Context, tableID string, _ []string, column string) (map[string]string, error) {
	rows, ok := r[tableID+"|"+column]
	if !ok {
		return nil, fmt.Errorf("missing table fixture for %s.%s", tableID, column)
	}
	return rows, nil
}

func (r contractLookupResolver) GetRows(_ context.Context, _ string) ([]map[string]string, error) {
	return nil, nil
}

func lookupAdditionGraph(leftTable, leftColumn, rightTable, rightColumn string) *domain.FormulaGraph {
	return &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "key", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "key", DataType: "decimal"})},
			{ID: "left", Type: domain.NodeTableLookup, Config: mustJSON(domain.TableLookupConfig{TableID: leftTable, Column: leftColumn})},
			{ID: "right", Type: domain.NodeTableLookup, Config: mustJSON(domain.TableLookupConfig{TableID: rightTable, Column: rightColumn})},
			{ID: "sum", Type: domain.NodeOperator, Config: mustJSON(domain.OperatorConfig{Op: "add"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "key", Target: "left", SourcePort: "out", TargetPort: "key"},
			{Source: "key", Target: "right", SourcePort: "out", TargetPort: "key"},
			{Source: "left", Target: "sum", SourcePort: "out", TargetPort: "left"},
			{Source: "right", Target: "sum", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"sum"},
	}
}
