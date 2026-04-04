package parser

import (
	"testing"
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
