package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

func TestStandaloneComparisonRoundTripsAsComparisonExpression(t *testing.T) {
	const expression = "premium > threshold"

	ast, err := NewParser(expression).Parse()
	if err != nil {
		t.Fatalf("parse %q: %v", expression, err)
	}
	graph, err := ASTToDAG(ast)
	if err != nil {
		t.Fatalf("ASTToDAG %q: %v", expression, err)
	}
	gotAST, err := DAGToAST(graph)
	if err != nil {
		t.Fatalf("DAGToAST standalone comparison: %v", err)
	}
	if got := ASTToText(gotAST); got != expression {
		t.Fatalf("standalone comparison round-trip = %q, want %q", got, expression)
	}
}

func TestASTToDAGUsesCanonicalVariadicAggregateNodes(t *testing.T) {
	cases := []struct {
		expression string
		fn         string
	}{
		{expression: "sum(a, b, c)", fn: "sum"},
		{expression: "avg(a, b, c)", fn: "avg"},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			ast, err := NewParser(tc.expression).Parse()
			if err != nil {
				t.Fatalf("parse %q: %v", tc.expression, err)
			}
			graph, err := ASTToDAG(ast)
			if err != nil {
				t.Fatalf("ASTToDAG %q: %v", tc.expression, err)
			}

			var aggregate *domain.FormulaNode
			for i := range graph.Nodes {
				if graph.Nodes[i].ID == graph.Outputs[0] {
					aggregate = &graph.Nodes[i]
					break
				}
			}
			if aggregate == nil {
				t.Fatalf("output node %q not found", graph.Outputs[0])
			}
			if aggregate.Type != domain.NodeAggregate {
				t.Fatalf("%s output node type = %q, want canonical %q", tc.expression, aggregate.Type, domain.NodeAggregate)
			}

			var cfg domain.AggregateConfig
			if err := json.Unmarshal(aggregate.Config, &cfg); err != nil {
				t.Fatalf("unmarshal aggregate config: %v", err)
			}
			if cfg.Fn != tc.fn {
				t.Fatalf("aggregate function = %q, want %q", cfg.Fn, tc.fn)
			}

			for i := 0; i < 3; i++ {
				port := "items:" + string(rune('0'+i))
				found := false
				for _, edge := range graph.Edges {
					if edge.Target == aggregate.ID && edge.TargetPort == port {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("canonical aggregate graph has no edge to %q", port)
				}
			}

			gotAST, err := DAGToAST(graph)
			if err != nil {
				t.Fatalf("DAGToAST canonical aggregate: %v", err)
			}
			if got := ASTToText(gotAST); got != tc.expression {
				t.Fatalf("canonical aggregate round-trip = %q, want %q", got, tc.expression)
			}
		})
	}
}

func TestASTToDAGRejectsThreeArgumentMinMaxWithoutSilentTruncation(t *testing.T) {
	for _, expression := range []string{"min(9, 8, 1)", "max(1, 8, 9)"} {
		t.Run(expression, func(t *testing.T) {
			ast, err := NewParser(expression).Parse()
			if err != nil {
				t.Fatalf("parse %q: %v", expression, err)
			}
			_, err = ASTToDAG(ast)
			if err == nil {
				t.Fatalf("ASTToDAG accepted %q; fixed-arity min/max must reject extra arguments", expression)
			}
			if !strings.Contains(err.Error(), "expects exactly 2 arguments") {
				t.Fatalf("ASTToDAG error = %q, want it to contain %q", err, "expects exactly 2 arguments")
			}
		})
	}
}
