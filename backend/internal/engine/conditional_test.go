package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// buildConditionalGraph builds a minimal FormulaGraph for testing the
// Conditional node end-to-end through Calculate. The graph has:
//
//   - Variables for every input port the Conditional needs
//   - One Conditional node with the supplied config
//   - One output edge from the Conditional
//
// portValues defines which input variable -> port mappings exist; the
// caller still passes raw input values via the inputs map at calc time.
func buildConditionalGraph(t *testing.T, cfg domain.ConditionalConfig, portValues map[string]string) *domain.FormulaGraph {
	t.Helper()

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal conditional config: %v", err)
	}

	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{
				ID:     "cond",
				Type:   domain.NodeConditional,
				Config: cfgJSON,
			},
		},
		Outputs: []string{"cond"},
	}

	// Each port -> a variable node feeding into that port
	for port, varName := range portValues {
		varCfgJSON, _ := json.Marshal(domain.VariableConfig{Name: varName})
		nodeID := "var_" + port
		graph.Nodes = append(graph.Nodes, domain.FormulaNode{
			ID:     nodeID,
			Type:   domain.NodeVariable,
			Config: varCfgJSON,
		})
		graph.Edges = append(graph.Edges, domain.FormulaEdge{
			Source:     nodeID,
			Target:     "cond",
			SourcePort: "out",
			TargetPort: port,
		})
	}

	return graph
}

// newTestEngine spins up a minimal engine sufficient for evaluating a
// Conditional graph (no tables, no sub-formulas, no loops).
func newTestEngine() Engine {
	return NewEngine(EngineConfig{
		Workers:   1,
		Precision: DefaultPrecision(),
		CacheSize: 16,
	})
}

func TestConditional_LegacySingleComparison(t *testing.T) {
	cases := []struct {
		name       string
		comparator string
		left, right, then, els string
		want       string
	}{
		{"eq_true",  "eq", "5", "5", "100", "200", "100"},
		{"eq_false", "eq", "5", "6", "100", "200", "200"},
		{"ne_true",  "ne", "5", "6", "100", "200", "100"},
		{"gt_true",  "gt", "9", "5", "100", "200", "100"},
		{"gt_false", "gt", "5", "9", "100", "200", "200"},
		{"ge_eq",    "ge", "5", "5", "100", "200", "100"},
		{"lt_true",  "lt", "1", "9", "100", "200", "100"},
		{"le_eq",    "le", "5", "5", "100", "200", "100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := domain.ConditionalConfig{Comparator: tc.comparator}
			graph := buildConditionalGraph(t, cfg, map[string]string{
				"condition":      "L",
				"conditionRight": "R",
				"thenValue":      "T",
				"elseValue":      "E",
			})
			engine := newTestEngine()
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"L": tc.left, "R": tc.right, "T": tc.then, "E": tc.els,
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			got := res.Outputs["cond"]
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditional_CompositeAND(t *testing.T) {
	// loss_ratio > 0.5  AND  cumulative_release < release_cap  →  release_amount, else 0
	// (Spec 002 §3 / 公式 19 example.)
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "gt"}, // term 0:  loss_ratio > 0.5
			{Op: "lt"}, // term 1:  cumulative_release < release_cap
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "loss_ratio",
		"conditionRight_0": "loss_threshold",
		"condition_1":      "cumulative_release",
		"conditionRight_1": "release_cap",
		"thenValue":        "release_amount",
		"elseValue":        "zero",
	})
	engine := newTestEngine()

	tests := []struct {
		name string
		lr, cr string
		want string
	}{
		{"both_true",            "0.7", "100", "1000"},
		{"first_false",          "0.3", "100", "0"},
		{"second_false",         "0.7", "999", "0"},
		{"both_false",           "0.3", "999", "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"loss_ratio":         tc.lr,
				"loss_threshold":     "0.5",
				"cumulative_release": tc.cr,
				"release_cap":        "500",
				"release_amount":     "1000",
				"zero":               "0",
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if got := res.Outputs["cond"]; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditional_CompositeOR(t *testing.T) {
	// (age < 18) OR (age > 65) → discount, else full_price
	cfg := domain.ConditionalConfig{
		Combinator: "or",
		Conditions: []domain.ConditionTerm{
			{Op: "lt"},
			{Op: "gt"},
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "age",
		"conditionRight_0": "min_age",
		"condition_1":      "age",
		"conditionRight_1": "max_age",
		"thenValue":        "discount",
		"elseValue":        "full",
	})
	engine := newTestEngine()

	tests := []struct {
		name string
		age  string
		want string
	}{
		{"young",  "10", "50"},
		{"adult",  "30", "100"},
		{"senior", "70", "50"},
		{"on_min", "18", "100"}, // 18 is not < 18 and not > 65
		{"on_max", "65", "100"}, // 65 is not < 18 and not > 65
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"age":      tc.age,
				"min_age":  "18",
				"max_age":  "65",
				"discount": "50",
				"full":     "100",
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if got := res.Outputs["cond"]; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditional_NegateInsideTerm(t *testing.T) {
	// NOT(x == 0) AND y > 0  →  takes "yes" branch only when x ≠ 0 and y > 0
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "eq", Negate: true},
			{Op: "gt"},
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "x",
		"conditionRight_0": "zero",
		"condition_1":      "y",
		"conditionRight_1": "zero",
		"thenValue":        "yes",
		"elseValue":        "no",
	})
	engine := newTestEngine()

	tests := []struct {
		name, x, y, want string
	}{
		{"x_ne_zero_y_pos", "5", "3", "1"},  // NOT(false) AND true → true
		{"x_eq_zero",      "0", "3", "0"},  // NOT(true) AND ... → false
		{"y_not_pos",      "5", "0", "0"},  // NOT(false) AND false → false
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"x": tc.x, "y": tc.y,
				"zero": "0", "yes": "1", "no": "0",
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if got := res.Outputs["cond"]; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditional_DefaultCombinatorIsAnd(t *testing.T) {
	// Empty Combinator string should be treated as "and".
	cfg := domain.ConditionalConfig{
		// Combinator omitted intentionally
		Conditions: []domain.ConditionTerm{
			{Op: "gt"},
			{Op: "lt"},
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "a",
		"conditionRight_0": "lo",
		"condition_1":      "a",
		"conditionRight_1": "hi",
		"thenValue":        "t",
		"elseValue":        "e",
	})
	engine := newTestEngine()
	res, err := engine.Calculate(context.Background(), graph, map[string]string{
		"a": "5", "lo": "1", "hi": "10", "t": "1", "e": "0",
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := res.Outputs["cond"]; got != "1" {
		t.Fatalf("default-combinator AND: got %q, want %q", got, "1")
	}
}

func TestConditional_ThreeConditions(t *testing.T) {
	// Three-term AND: x > 0 AND x < 100 AND y == 1
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "gt"},
			{Op: "lt"},
			{Op: "eq"},
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "x",
		"conditionRight_0": "zero",
		"condition_1":      "x",
		"conditionRight_1": "hundred",
		"condition_2":      "y",
		"conditionRight_2": "one",
		"thenValue":        "ok",
		"elseValue":        "no",
	})
	engine := newTestEngine()

	tests := []struct {
		name, x, y, want string
	}{
		{"all_true",         "50", "1", "1"},
		{"x_too_low",        "-5", "1", "0"},
		{"x_too_high",       "200", "1", "0"},
		{"y_wrong",          "50", "2", "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"x": tc.x, "y": tc.y,
				"zero": "0", "hundred": "100", "one": "1",
				"ok": "1", "no": "0",
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if got := res.Outputs["cond"]; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditional_ValidatorRejectsBadCombinator(t *testing.T) {
	cfg := domain.ConditionalConfig{
		Combinator: "xor", // not supported
		Conditions: []domain.ConditionTerm{{Op: "eq"}},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "a",
		"conditionRight_0": "b",
		"thenValue":        "t",
		"elseValue":        "e",
	})
	engine := newTestEngine()
	_, err := engine.Calculate(context.Background(), graph, map[string]string{
		"a": "1", "b": "1", "t": "1", "e": "0",
	})
	if err == nil {
		t.Fatal("expected error for unknown combinator, got nil")
	}
	if !strings.Contains(err.Error(), "combinator") {
		t.Fatalf("expected combinator error, got: %v", err)
	}
}

func TestConditional_ValidatorRejectsBadOpInTerm(t *testing.T) {
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "eq"},
			{Op: "approx"}, // not supported
		},
	}
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "a",
		"conditionRight_0": "b",
		"condition_1":      "c",
		"conditionRight_1": "d",
		"thenValue":        "t",
		"elseValue":        "e",
	})
	engine := newTestEngine()
	_, err := engine.Calculate(context.Background(), graph, map[string]string{
		"a": "1", "b": "1", "c": "2", "d": "2", "t": "1", "e": "0",
	})
	if err == nil {
		t.Fatal("expected error for unknown op in term, got nil")
	}
	if !strings.Contains(err.Error(), "approx") && !strings.Contains(err.Error(), "op") {
		t.Fatalf("expected op error, got: %v", err)
	}
}

func TestConditional_ValidatorRejectsMissingPort(t *testing.T) {
	cfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "gt"},
			{Op: "lt"},
		},
	}
	// Intentionally omit "conditionRight_1" — missing port for term 1.
	graph := buildConditionalGraph(t, cfg, map[string]string{
		"condition_0":      "a",
		"conditionRight_0": "b",
		"condition_1":      "c",
		// "conditionRight_1": missing on purpose
		"thenValue": "t",
		"elseValue": "e",
	})
	engine := newTestEngine()
	_, err := engine.Calculate(context.Background(), graph, map[string]string{
		"a": "5", "b": "1", "c": "9", "t": "1", "e": "0",
	})
	if err == nil {
		t.Fatal("expected error for missing port, got nil")
	}
	if !strings.Contains(err.Error(), "conditionRight_1") {
		t.Fatalf("expected error mentioning conditionRight_1, got: %v", err)
	}
}

func TestConditional_LegacyFormulaStillWorks(t *testing.T) {
	// Regression: a graph saved with the old single-comparator format
	// (no Conditions slice) must continue to evaluate exactly as before.
	cfgJSON := []byte(`{"comparator":"gt"}`)
	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			{ID: "cond", Type: domain.NodeConditional, Config: cfgJSON},
			{ID: "L", Type: domain.NodeVariable, Config: mustMarshal(t, domain.VariableConfig{Name: "x"})},
			{ID: "R", Type: domain.NodeVariable, Config: mustMarshal(t, domain.VariableConfig{Name: "y"})},
			{ID: "T", Type: domain.NodeVariable, Config: mustMarshal(t, domain.VariableConfig{Name: "yes"})},
			{ID: "E", Type: domain.NodeVariable, Config: mustMarshal(t, domain.VariableConfig{Name: "no"})},
		},
		Edges: []domain.FormulaEdge{
			{Source: "L", Target: "cond", SourcePort: "out", TargetPort: "condition"},
			{Source: "R", Target: "cond", SourcePort: "out", TargetPort: "conditionRight"},
			{Source: "T", Target: "cond", SourcePort: "out", TargetPort: "thenValue"},
			{Source: "E", Target: "cond", SourcePort: "out", TargetPort: "elseValue"},
		},
		Outputs: []string{"cond"},
	}
	engine := newTestEngine()
	res, err := engine.Calculate(context.Background(), graph, map[string]string{
		"x": "10", "y": "5", "yes": "1", "no": "0",
	})
	if err != nil {
		t.Fatalf("legacy calc: %v", err)
	}
	if got := res.Outputs["cond"]; got != "1" {
		t.Fatalf("legacy gt: got %q, want %q", got, "1")
	}
}

func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
