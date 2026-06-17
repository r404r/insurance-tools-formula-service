package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

func TestMultiArgRoundTrip(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			"sum(a, b, c)",
			"sum(a, b, c)",
		},
		{
			"avg(x, y, z)",
			"avg(x, y, z)",
		},
		{
			// Nested if-else: serialiser wraps the else-branch in parens to make
			// precedence explicit; the result is semantically equivalent.
			"if age >= 65 then sum(basePremium, elderSurcharge, hazardBonus) else if age >= 18 then avg(basePremium, youngDiscount) else basePremium * 0.5",
			"if age >= 65 then sum(basePremium, elderSurcharge, hazardBonus) else (if age >= 18 then avg(basePremium, youngDiscount) else basePremium * 0.5)",
		},
	}
	for _, c := range cases {
		p := NewParser(c.input)
		ast, err := p.Parse()
		if err != nil {
			t.Fatalf("parse %q: %v", c.input, err)
		}
		graph, err := ASTToDAG(ast)
		if err != nil {
			t.Fatalf("ASTToDAG %q: %v", c.input, err)
		}
		ast2, err := DAGToAST(graph)
		if err != nil {
			t.Fatalf("DAGToAST %q: %v", c.input, err)
		}
		got := ASTToText(ast2)
		if got != c.want {
			t.Errorf("round-trip mismatch:\n  input: %s\n  want:  %s\n  got:   %s", c.input, c.want, got)
		}
	}
}

func TestParserRejectsExcessiveNestingDepth(t *testing.T) {
	input := strings.Repeat("(", maxParseDepth+1) + "1" + strings.Repeat(")", maxParseDepth+1)
	_, err := NewParser(input).Parse()
	if err == nil {
		t.Fatal("expected depth error, got nil")
	}
	if !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("error = %q, want maximum depth", err.Error())
	}
}

func TestParserRejectsExcessiveUnaryDepth(t *testing.T) {
	input := strings.Repeat("-", maxParseDepth+1) + "1"
	_, err := NewParser(input).Parse()
	if err == nil {
		t.Fatal("expected depth error, got nil")
	}
	if !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("error = %q, want maximum depth", err.Error())
	}
}

func TestLoopRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"sum_loop basic",
			`sum_loop("body-id", t, 1, n)`,
			`sum_loop("body-id", t, 1, n)`,
		},
		{
			"product_loop basic",
			`product_loop("body-id", t, 1, n)`,
			`product_loop("body-id", t, 1, n)`,
		},
		{
			"avg_loop with step",
			`avg_loop("body-id", i, 0, 100, 2)`,
			`avg_loop("body-id", i, 0, 100, 2)`,
		},
		{
			"loop with expression args",
			`sum_loop("body-id", t, 1, n + 1)`,
			`sum_loop("body-id", t, 1, n + 1)`,
		},
		{
			"fold_loop basic",
			`fold_loop("reserve-step", t, 0, n, V, 0)`,
			`fold_loop("reserve-step", t, 0, n, V, 0)`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewParser(c.input)
			ast, err := p.Parse()
			if err != nil {
				t.Fatalf("parse %q: %v", c.input, err)
			}
			graph, err := ASTToDAG(ast)
			if err != nil {
				t.Fatalf("ASTToDAG %q: %v", c.input, err)
			}
			// Verify DAG contains a loop node
			hasLoop := false
			for _, n := range graph.Nodes {
				if n.Type == domain.NodeLoop {
					hasLoop = true
					var cfg domain.LoopConfig
					if err := json.Unmarshal(n.Config, &cfg); err != nil {
						t.Fatalf("bad loop config: %v", err)
					}
					if cfg.Mode != "range" {
						t.Errorf("expected mode 'range', got %q", cfg.Mode)
					}
				}
			}
			if !hasLoop {
				t.Fatal("expected a NodeLoop in the DAG, found none")
			}
			ast2, err := DAGToAST(graph)
			if err != nil {
				t.Fatalf("DAGToAST %q: %v", c.input, err)
			}
			got := ASTToText(ast2)
			if got != c.want {
				t.Errorf("round-trip mismatch:\n  input: %s\n  want:  %s\n  got:   %s", c.input, c.want, got)
			}
		})
	}
}

func TestLoopDAGConfig(t *testing.T) {
	input := `product_loop("factorial-body", t, 1, 10)`
	p := NewParser(input)
	ast, err := p.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	graph, err := ASTToDAG(ast)
	if err != nil {
		t.Fatalf("ASTToDAG: %v", err)
	}
	for _, n := range graph.Nodes {
		if n.Type == domain.NodeLoop {
			var cfg domain.LoopConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if cfg.FormulaID != "factorial-body" {
				t.Errorf("expected formulaId 'factorial-body', got %q", cfg.FormulaID)
			}
			if cfg.Iterator != "t" {
				t.Errorf("expected iterator 't', got %q", cfg.Iterator)
			}
			if cfg.Aggregation != "product" {
				t.Errorf("expected aggregation 'product', got %q", cfg.Aggregation)
			}
			return
		}
	}
	t.Fatal("no NodeLoop found in DAG")
}
