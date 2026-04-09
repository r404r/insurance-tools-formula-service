package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// mockFormulaResolver resolves formulaID → FormulaVersion from an in-memory map.
// Version is always 1; nil version selector is ignored.
type mockFormulaResolver struct {
	formulas map[string]*domain.FormulaVersion
}

func (r *mockFormulaResolver) ResolveFormula(_ context.Context, formulaID string, _ *int) (*domain.FormulaVersion, error) {
	v, ok := r.formulas[formulaID]
	if !ok {
		return nil, fmt.Errorf("formula %q not found", formulaID)
	}
	return v, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// identityFormulaVersion creates a FormulaVersion whose graph contains a single
// variable node named `varName` and declares it as the output.
// The formula simply returns the value of the variable when called.
func identityFormulaVersion(formulaID, varName string) *domain.FormulaVersion {
	nodeID := "out"
	return &domain.FormulaVersion{
		ID:        formulaID + "@1",
		FormulaID: formulaID,
		Version:   1,
		State:     domain.StatePublished,
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{
					ID:     nodeID,
					Type:   domain.NodeVariable,
					Config: mustJSON(domain.VariableConfig{Name: varName, DataType: "decimal"}),
				},
			},
			Edges:   []domain.FormulaEdge{},
			Outputs: []string{nodeID},
		},
	}
}

// loopEngine creates a test engine with the given formula resolver and optional maxIter override.
func loopEngine(resolver FormulaResolver, maxIter int) Engine {
	cfg := DefaultEngineConfig()
	cfg.Workers = 1
	cfg.CacheSize = 0 // disable cache for deterministic tests
	cfg.FormulaResolver = resolver
	if maxIter > 0 {
		cfg.MaxLoopIterations = maxIter
	}
	return NewEngine(cfg)
}

// parentGraph builds a simple FormulaGraph containing one loop node whose
// start/end/(optional step) are fed by constant nodes.
func parentLoopGraph(loopCfg domain.LoopConfig, start, end string, step ...string) domain.FormulaGraph {
	nodes := []domain.FormulaNode{
		{ID: "c_start", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: start})},
		{ID: "c_end", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: end})},
		{ID: "loop", Type: domain.NodeLoop, Config: mustJSON(loopCfg)},
	}
	edges := []domain.FormulaEdge{
		{Source: "c_start", Target: "loop", SourcePort: "out", TargetPort: "start"},
		{Source: "c_end", Target: "loop", SourcePort: "out", TargetPort: "end"},
	}
	if len(step) > 0 {
		nodes = append(nodes, domain.FormulaNode{
			ID:     "c_step",
			Type:   domain.NodeConstant,
			Config: mustJSON(domain.ConstantConfig{Value: step[0]}),
		})
		edges = append(edges, domain.FormulaEdge{
			Source: "c_step", Target: "loop", SourcePort: "out", TargetPort: "step",
		})
	}
	return domain.FormulaGraph{
		Nodes:   nodes,
		Edges:   edges,
		Outputs: []string{"loop"},
	}
}

// ---------------------------------------------------------------------------
// Tests: aggregation modes
// ---------------------------------------------------------------------------

// sum 1..5 using identity formula → 1+2+3+4+5 = 15
func TestLoop_Sum(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "5")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.Outputs["loop"]
	if got != "15" {
		t.Errorf("sum 1..5 = %s, want 15", got)
	}
}

// avg 1..4 → (1+2+3+4)/4 = 2.5
func TestLoop_Avg(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "avg"}
	graph := parentLoopGraph(cfg, "1", "4")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.Outputs["loop"]
	// 2.5 rounded to output precision (8 places)
	if got != "2.5" {
		t.Errorf("avg 1..4 = %s, want 2.5", got)
	}
}

// last 1..5 → 5
func TestLoop_Last(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "last"}
	graph := parentLoopGraph(cfg, "1", "5")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.Outputs["loop"]
	if got != "5" {
		t.Errorf("last 1..5 = %s, want 5", got)
	}
}

// min 3..7 → 3
func TestLoop_Min(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "min"}
	graph := parentLoopGraph(cfg, "3", "7")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "3" {
		t.Errorf("min 3..7 = %s, want 3", result.Outputs["loop"])
	}
}

// max 3..7 → 7
func TestLoop_Max(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "max"}
	graph := parentLoopGraph(cfg, "3", "7")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "7" {
		t.Errorf("max 3..7 = %s, want 7", result.Outputs["loop"])
	}
}

// count 1..10 → 10
func TestLoop_Count(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "count"}
	graph := parentLoopGraph(cfg, "1", "10")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "10" {
		t.Errorf("count 1..10 = %s, want 10", result.Outputs["loop"])
	}
}

// product 1..5 → 1*2*3*4*5 = 120
func TestLoop_Product(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "product"}
	graph := parentLoopGraph(cfg, "1", "5")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "120" {
		t.Errorf("product 1..5 = %s, want 120", result.Outputs["loop"])
	}
}

// ---------------------------------------------------------------------------
// Tests: step behaviour
// ---------------------------------------------------------------------------

// step=2: sum 1,3,5 → 9
func TestLoop_StepTwo(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "5", "2")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "9" {
		t.Errorf("sum 1,3,5 (step=2) = %s, want 9", result.Outputs["loop"])
	}
}

// step=-1: sum 5,4,3,2,1 → 15 (same as forward but reversed)
func TestLoop_NegativeStep(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "5", "1", "-1")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "15" {
		t.Errorf("sum 5..1 step=-1 = %s, want 15", result.Outputs["loop"])
	}
}

// step omitted defaults to 1: sum 1..3 → 6
func TestLoop_DefaultStep(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "3") // no step arg → no step edge → defaults to 1

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "6" {
		t.Errorf("sum 1..3 (default step) = %s, want 6", result.Outputs["loop"])
	}
}

// ---------------------------------------------------------------------------
// Tests: inclusiveEnd
// ---------------------------------------------------------------------------

// inclusiveEnd=false: 1..5 exclusive → 1,2,3,4 → sum=10
func TestLoop_InclusiveEndFalse(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	inclusive := false
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum", InclusiveEnd: &inclusive}
	graph := parentLoopGraph(cfg, "1", "5")

	result, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outputs["loop"] != "10" {
		t.Errorf("sum 1..<5 (exclusive) = %s, want 10", result.Outputs["loop"])
	}
}

// ---------------------------------------------------------------------------
// Tests: error cases
// ---------------------------------------------------------------------------

func TestLoop_StepZeroError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "5", "0") // step=0

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for step=0, got nil")
	}
}

func TestLoop_NonIntegerStartError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1.5", "5") // fractional start

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for non-integer start, got nil")
	}
}

func TestLoop_ZeroIterationsError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	// step=1 but start > end → 0 iterations
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "5", "1") // start=5 > end=1, step defaults to 1

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for zero iterations (start>end with step>0), got nil")
	}
}

func TestLoop_MaxIterationsExceeded(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 3) // engine max = 3

	// 1..10 → 10 iterations > 3
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "10")

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for maxIterations exceeded, got nil")
	}
}

func TestLoop_NodeMaxIterationsOverride(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 1000) // engine max = 1000

	// node-level cap = 3, but we iterate 1..5 → 5 > 3
	nodeCap := 3
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum", MaxIterations: &nodeCap}
	graph := parentLoopGraph(cfg, "1", "5")

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for node-level maxIterations exceeded, got nil")
	}
}

func TestLoop_UnknownFormulaError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "does-not-exist", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "3")

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for unknown formula, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests: recursion guard
// ---------------------------------------------------------------------------

// A loop calls formula A, which contains a loop that calls formula A again.
// This should be detected as cyclic sub-formula reference.
func TestLoop_RecursionGuard(t *testing.T) {
	// Formula A: a loop from 1..2 calling formula A itself (via sub-formula node).
	// We build it as a subFormula node referencing "formula-a" to trigger the guard.
	subNode := domain.FormulaNode{
		ID:     "sub",
		Type:   domain.NodeSubFormula,
		Config: mustJSON(domain.SubFormulaConfig{FormulaID: "formula-a"}),
	}
	selfRefGraph := domain.FormulaGraph{
		Nodes:   []domain.FormulaNode{subNode},
		Edges:   []domain.FormulaEdge{},
		Outputs: []string{"sub"},
	}

	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"formula-a": {
			ID:        "formula-a@1",
			FormulaID: "formula-a",
			Version:   1,
			State:     domain.StatePublished,
			Graph:     selfRefGraph,
		},
	}}

	eng := loopEngine(resolver, 0)

	// Outer graph: a loop calling formula-a (which itself calls formula-a).
	cfg := domain.LoopConfig{Mode: "range", FormulaID: "formula-a", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "2")

	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected cyclic sub-formula error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests: Validate integration
// ---------------------------------------------------------------------------

func TestLoop_NonIntegerEndError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "4.5") // fractional end
	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for non-integer end, got nil")
	}
}

func TestLoop_NonIntegerStepError(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	// parentLoopGraph only accepts start/end strings; inject a fractional step node manually.
	startNode := domain.FormulaNode{ID: "c_start", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "1"})}
	endNode := domain.FormulaNode{ID: "c_end", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "5"})}
	stepNode := domain.FormulaNode{ID: "c_step", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "0.5"})}
	loopNode := domain.FormulaNode{ID: "loop", Type: domain.NodeLoop, Config: mustJSON(cfg)}
	graph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{startNode, endNode, stepNode, loopNode},
		Edges: []domain.FormulaEdge{
			{Source: "c_start", Target: "loop", SourcePort: "out", TargetPort: "start"},
			{Source: "c_end", Target: "loop", SourcePort: "out", TargetPort: "end"},
			{Source: "c_step", Target: "loop", SourcePort: "out", TargetPort: "step"},
		},
		Outputs: []string{"loop"},
	}
	_, err := eng.Calculate(context.Background(), &graph, map[string]string{})
	if err == nil {
		t.Fatal("expected error for non-integer step, got nil")
	}
}

func TestLoop_IteratorConflictsWithParentInput(t *testing.T) {
	resolver := &mockFormulaResolver{formulas: map[string]*domain.FormulaVersion{
		"body": identityFormulaVersion("body", "t"),
	}}
	eng := loopEngine(resolver, 0)

	cfg := domain.LoopConfig{Mode: "range", FormulaID: "body", Iterator: "t", Aggregation: "sum"}
	graph := parentLoopGraph(cfg, "1", "3")

	// Pass "t" as a parent input — this conflicts with the iterator name.
	_, err := eng.Calculate(context.Background(), &graph, map[string]string{"t": "99"})
	if err == nil {
		t.Fatal("expected error when iterator name conflicts with parent input, got nil")
	}
}

func TestLoop_ValidateConfig(t *testing.T) {
	eng := NewEngine(DefaultEngineConfig())

	// Valid loop config.
	validGraph := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "c_start", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "1"})},
			{ID: "c_end", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "5"})},
			{ID: "loop", Type: domain.NodeLoop, Config: mustJSON(domain.LoopConfig{
				Mode: "range", FormulaID: "some-formula", Iterator: "t", Aggregation: "sum",
			})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "c_start", Target: "loop", SourcePort: "out", TargetPort: "start"},
			{Source: "c_end", Target: "loop", SourcePort: "out", TargetPort: "end"},
		},
		Outputs: []string{"loop"},
	}
	if errs := eng.Validate(&validGraph); len(errs) != 0 {
		t.Errorf("expected no validation errors for valid loop, got: %v", errs)
	}

	// Missing start port.
	missingStart := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "c_end", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "5"})},
			{ID: "loop", Type: domain.NodeLoop, Config: mustJSON(domain.LoopConfig{
				Mode: "range", FormulaID: "some-formula", Iterator: "t", Aggregation: "sum",
			})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "c_end", Target: "loop", SourcePort: "out", TargetPort: "end"},
		},
		Outputs: []string{"loop"},
	}
	errs := eng.Validate(&missingStart)
	if len(errs) == 0 {
		t.Error("expected validation error for missing 'start' port, got none")
	}

	// Invalid aggregation.
	badAgg := domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "c_start", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "1"})},
			{ID: "c_end", Type: domain.NodeConstant, Config: mustJSON(domain.ConstantConfig{Value: "5"})},
			{ID: "loop", Type: domain.NodeLoop, Config: mustJSON(domain.LoopConfig{
				Mode: "range", FormulaID: "some-formula", Iterator: "t", Aggregation: "median",
			})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "c_start", Target: "loop", SourcePort: "out", TargetPort: "start"},
			{Source: "c_end", Target: "loop", SourcePort: "out", TargetPort: "end"},
		},
		Outputs: []string{"loop"},
	}
	errs = eng.Validate(&badAgg)
	if len(errs) == 0 {
		t.Error("expected validation error for invalid aggregation 'median', got none")
	}
}
