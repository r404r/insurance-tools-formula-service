package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// buildCompositeConditionalGraph builds a minimal graph carrying a single
// composite Conditional node wired with the given number of condition terms.
// Each term gets a "var<i>L" / "var<i>R" pair of variable nodes plus the
// thenValue / elseValue feeders. The shape mirrors what the engine and the
// formula export bundle produce, so the parser validator must accept it
// as long as the wire count matches 2*N + 2.
func buildCompositeConditionalGraph(t *testing.T, terms []domain.ConditionTerm, combinator string) *domain.FormulaGraph {
	t.Helper()

	cfg := domain.ConditionalConfig{
		Combinator: combinator,
		Conditions: terms,
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}

	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "cond", Type: domain.NodeConditional, Config: cfgJSON},
			{ID: "thenVar", Type: domain.NodeVariable, Config: mustMarshalVar(t, "thenInput")},
			{ID: "elseVar", Type: domain.NodeVariable, Config: mustMarshalVar(t, "elseInput")},
		},
		Edges: []domain.FormulaEdge{
			{Source: "thenVar", Target: "cond", SourcePort: "out", TargetPort: "thenValue"},
			{Source: "elseVar", Target: "cond", SourcePort: "out", TargetPort: "elseValue"},
		},
		Outputs: []string{"cond"},
	}

	for i := range terms {
		leftID := "L" + itoa(i)
		rightID := "R" + itoa(i)
		graph.Nodes = append(graph.Nodes,
			domain.FormulaNode{ID: leftID, Type: domain.NodeVariable, Config: mustMarshalVar(t, leftID)},
			domain.FormulaNode{ID: rightID, Type: domain.NodeVariable, Config: mustMarshalVar(t, rightID)},
		)
		graph.Edges = append(graph.Edges,
			domain.FormulaEdge{Source: leftID, Target: "cond", SourcePort: "out", TargetPort: "condition_" + itoa(i)},
			domain.FormulaEdge{Source: rightID, Target: "cond", SourcePort: "out", TargetPort: "conditionRight_" + itoa(i)},
		)
	}

	return graph
}

func mustMarshalVar(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(domain.VariableConfig{Name: name})
	if err != nil {
		t.Fatalf("marshal var: %v", err)
	}
	return b
}

// Tiny non-allocating itoa for indices 0-9. We never need more in tests.
func itoa(i int) string {
	if i >= 0 && i <= 9 {
		return string(rune('0' + i))
	}
	// Fallback for larger indices in case a future test wants them.
	return "x"
}

// TestValidateGraph_AcceptsCompositeConditional is the regression for the
// codex-review P1: before the fix, ValidateGraph emitted "conditional node
// expects 3 inputs..." for any composite Conditional, blocking import of
// every exported graph that used the new AND/OR feature.
func TestValidateGraph_AcceptsCompositeConditional(t *testing.T) {
	cases := []struct {
		name       string
		combinator string
		terms      []domain.ConditionTerm
	}{
		{"two_term_and", "and", []domain.ConditionTerm{{Op: "gt"}, {Op: "lt"}}},
		{"two_term_or", "or", []domain.ConditionTerm{{Op: "lt"}, {Op: "gt"}}},
		{"three_term_and", "and", []domain.ConditionTerm{{Op: "gt"}, {Op: "lt"}, {Op: "eq"}}},
		{"single_term", "and", []domain.ConditionTerm{{Op: "eq"}}},
		{"with_negate", "and", []domain.ConditionTerm{{Op: "eq", Negate: true}, {Op: "gt"}}},
		{"empty_combinator_defaults_and", "", []domain.ConditionTerm{{Op: "gt"}, {Op: "lt"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graph := buildCompositeConditionalGraph(t, tc.terms, tc.combinator)
			errs := ValidateGraph(graph)
			if len(errs) > 0 {
				t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
			}
		})
	}
}

func TestValidateGraph_RejectsCompositeWithWrongPortCount(t *testing.T) {
	// Two terms declared but only one term's worth of ports wired.
	graph := buildCompositeConditionalGraph(t, []domain.ConditionTerm{{Op: "gt"}}, "and")
	// Manually swap the config to claim two terms while leaving the wire
	// count at 1 + 1 + 2 = 4 instead of the expected 6.
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{{Op: "gt"}, {Op: "lt"}},
	}
	cfgJSON, _ := json.Marshal(cfg)
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == "cond" {
			graph.Nodes[i].Config = cfgJSON
		}
	}
	errs := ValidateGraph(graph)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "composite conditional with 2 term(s) expects 6 inputs") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected port-count error, got: %v", errs)
	}
}

func TestValidateGraph_RejectsCompositeWithUnknownCombinator(t *testing.T) {
	graph := buildCompositeConditionalGraph(t, []domain.ConditionTerm{{Op: "eq"}, {Op: "eq"}}, "xor")
	errs := ValidateGraph(graph)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "unknown conditional combinator") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected combinator error, got: %v", errs)
	}
}

func TestValidateGraph_RejectsCompositeWithUnknownOp(t *testing.T) {
	graph := buildCompositeConditionalGraph(t,
		[]domain.ConditionTerm{{Op: "eq"}, {Op: "approx"}}, "and")
	errs := ValidateGraph(graph)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "term 1: unknown op") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected term-op error, got: %v", errs)
	}
}

// TestDAGToAST_RejectsCompositeConditionalWithClearMessage covers the
// codex-review P2 follow-up: the visual→text serializer cannot yet round-trip
// composite conditionals because the lexer/parser side has no boolean
// combinator syntax. Until that work lands (out of scope for task #039 per
// spec 003), DAGToAST must produce a recognizable error that the frontend
// can detect to disable text-mode for the formula — same UX as the existing
// loopNoTextMode limitation. This test pins the contract.
func TestDAGToAST_RejectsCompositeConditionalWithClearMessage(t *testing.T) {
	graph := buildCompositeConditionalGraph(t,
		[]domain.ConditionTerm{{Op: "gt"}, {Op: "lt"}}, "and")
	_, err := DAGToAST(graph)
	if err == nil {
		t.Fatal("expected error from DAGToAST for composite conditional, got nil")
	}
	if !strings.Contains(err.Error(), "composite conditional") {
		t.Fatalf("expected error to mention 'composite conditional', got: %v", err)
	}
	if !strings.Contains(err.Error(), "visual editor") {
		t.Fatalf("expected error to direct user to visual editor, got: %v", err)
	}
}

// TestDAGToAST_RejectsTableAggregateWithClearMessage is the task #046
// counterpart for the composite-conditional test above. NodeTableAggregate
// (introduced in task #040) cannot round-trip through the text editor
// because the text grammar has no SQL-style aggregation syntax. The
// serializer must therefore emit a recognizable error that the frontend
// can detect to keep the formula in visual-only mode. Without this case
// the LDF seed formula from task #045 (which is the first in-tree user of
// tableAggregate) shows a raw "Unsupported node type" comment in the text
// pane.
func TestDAGToAST_RejectsTableAggregateWithClearMessage(t *testing.T) {
	aggCfg := domain.TableAggregateConfig{
		TableID:    "some-table",
		Aggregate:  "avg",
		Expression: "value",
		Filters: []domain.TableFilter{
			{Column: "year", Op: "eq", InputPort: "year"},
		},
	}
	cfgJSON, err := json.Marshal(aggCfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}
	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "year", Type: domain.NodeVariable, Config: mustMarshalVar(t, "year")},
			{ID: "agg", Type: domain.NodeTableAggregate, Config: cfgJSON},
		},
		Edges: []domain.FormulaEdge{
			{Source: "year", Target: "agg", SourcePort: "out", TargetPort: "year"},
		},
		Outputs: []string{"agg"},
	}
	_, err = DAGToAST(graph)
	if err == nil {
		t.Fatal("expected error from DAGToAST for tableAggregate, got nil")
	}
	if !strings.Contains(err.Error(), "tableAggregate") {
		t.Fatalf("expected error to mention 'tableAggregate', got: %v", err)
	}
	if !strings.Contains(err.Error(), "visual editor") {
		t.Fatalf("expected error to direct user to visual editor, got: %v", err)
	}
}

// TestValidateGraph_LegacyConditionalStillAccepted ensures the legacy
// single-comparator if/then/else shape still passes through unchanged
// after the composite branch was added.
func TestValidateGraph_LegacyConditionalStillAccepted(t *testing.T) {
	cfgJSON, _ := json.Marshal(domain.ConditionalConfig{Comparator: "gt"})
	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "cond", Type: domain.NodeConditional, Config: cfgJSON},
			{ID: "L", Type: domain.NodeVariable, Config: mustMarshalVar(t, "L")},
			{ID: "R", Type: domain.NodeVariable, Config: mustMarshalVar(t, "R")},
		},
		Edges: []domain.FormulaEdge{
			{Source: "L", Target: "cond", SourcePort: "out", TargetPort: "left"},
			{Source: "R", Target: "cond", SourcePort: "out", TargetPort: "right"},
		},
		Outputs: []string{"cond"},
	}
	errs := ValidateGraph(graph)
	if len(errs) > 0 {
		t.Fatalf("legacy comparator graph rejected: %v", errs)
	}
}
