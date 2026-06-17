package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

func TestDecimalPowRejectsImaginaryFractionalResult(t *testing.T) {
	node := domain.FormulaNode{
		ID:     "pow",
		Type:   domain.NodeOperator,
		Config: mustJSON(domain.OperatorConfig{Op: "power"}),
	}
	ev := NewEvaluator(DefaultPrecision(), nil)

	_, err := ev.EvaluateNode(&node, map[string]Decimal{
		"left":  NewDecimal("-1"),
		"right": NewDecimal("0.5"),
	})
	if err == nil {
		t.Fatal("expected error for negative base with fractional exponent, got nil")
	}
	if !strings.Contains(err.Error(), "power failed") {
		t.Fatalf("expected power error, got: %v", err)
	}
}

func TestDecimalFunctionsUseDecimalPrecision(t *testing.T) {
	node := domain.FormulaNode{
		ID:     "sqrt",
		Type:   domain.NodeFunction,
		Config: mustJSON(domain.FunctionConfig{Fn: "sqrt"}),
	}
	ev := NewEvaluator(DefaultPrecision(), nil)

	got, err := ev.EvaluateNode(&node, map[string]Decimal{"in": NewDecimal("2")})
	if err != nil {
		t.Fatalf("unexpected sqrt error: %v", err)
	}
	if !strings.HasPrefix(got.String(), "1.414213562373") {
		t.Fatalf("sqrt(2) = %s, want decimal precision prefix", got.String())
	}
}

func TestRoundRejectsInvalidPlaces(t *testing.T) {
	node := domain.FormulaNode{
		ID:   "round",
		Type: domain.NodeFunction,
		Config: mustJSON(domain.FunctionConfig{
			Fn:   "round",
			Args: map[string]string{"places": "abc"},
		}),
	}
	ev := NewEvaluator(DefaultPrecision(), nil)

	_, err := ev.EvaluateNode(&node, map[string]Decimal{"in": NewDecimal("1.234")})
	if err == nil {
		t.Fatal("expected invalid places error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid round places") {
		t.Fatalf("expected invalid places error, got: %v", err)
	}
}

func TestGraphHashFullDigestAndMarshalError(t *testing.T) {
	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "c", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "1"})},
		},
		Outputs: []string{"c"},
	}
	hash, err := graphHash(graph)
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("graph hash length = %d, want 64 hex chars", len(hash))
	}

	badGraph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "bad", Type: domain.NodeConstant, Config: json.RawMessage("{")},
		},
		Outputs: []string{"bad"},
	}
	if _, err := graphHash(badGraph); err == nil {
		t.Fatal("expected marshal error for invalid raw config, got nil")
	}
}

func TestCacheKeyIncludesPrecisionConfig(t *testing.T) {
	cfg := DefaultEngineConfig()
	cfg.Workers = 1
	cfg.Precision.OutputPrecision = 2
	eng := NewEngine(cfg).(*defaultEngine)
	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "c", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "1.23456"})},
		},
		Outputs: []string{"c"},
	}

	first, err := eng.Calculate(context.Background(), graph, nil)
	if err != nil {
		t.Fatalf("first calculation failed: %v", err)
	}
	if first.Outputs["c"] != "1.23" {
		t.Fatalf("first output = %s, want 1.23", first.Outputs["c"])
	}

	eng.config.Precision.OutputPrecision = 4
	second, err := eng.Calculate(context.Background(), graph, nil)
	if err != nil {
		t.Fatalf("second calculation failed: %v", err)
	}
	if second.CacheHit {
		t.Fatal("second calculation was served from stale cache after precision change")
	}
	if second.Outputs["c"] != "1.2346" {
		t.Fatalf("second output = %s, want 1.2346", second.Outputs["c"])
	}
}

func TestLoopErrorsWhenBodyOutputIsMissing(t *testing.T) {
	body := &domain.FormulaVersion{
		ID:        "body@1",
		FormulaID: "body",
		Version:   1,
		State:     domain.StatePublished,
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "actual", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "t", DataType: "decimal"})},
			},
			Outputs: []string{"missing"},
		},
	}
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{"body": body}}
	eng := loopEngine(resolver, 0)
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "1")

	_, err := eng.Calculate(context.Background(), &graph, nil)
	if err == nil {
		t.Fatal("expected missing body output error, got nil")
	}
	if !strings.Contains(err.Error(), `body output "missing" was not produced`) {
		t.Fatalf("expected missing body output error, got: %v", err)
	}
}

func TestRoundHalfDown(t *testing.T) {
	pc := PrecisionConfig{OutputPrecision: 1, Rounding: RoundHalfDown}

	if got := pc.RoundOutput(NewDecimal("1.25")); got.String() != "1.2" {
		t.Fatalf("1.25 half-down = %s, want 1.2", got.String())
	}
	if got := pc.RoundOutput(NewDecimal("-1.25")); got.String() != "-1.2" {
		t.Fatalf("-1.25 half-down = %s, want -1.2", got.String())
	}
	if got := pc.RoundOutput(NewDecimal("1.26")); got.String() != "1.3" {
		t.Fatalf("1.26 half-down = %s, want 1.3", got.String())
	}
}
