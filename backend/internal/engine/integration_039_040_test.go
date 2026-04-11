package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// TestDisasterReserveRelease is the integration regression test for both
// task #039 (Conditional AND/OR/NOT) and task #040 (NodeTableAggregate).
//
// The formula models a real Japanese non-life actuarial scenario from
// docs/specs/002-Japan-insurance-ref.md formula 19 (異常危険準備金):
//
//	IF avg_historical_loss_ratio > 0.5
//	   AND current_loss_ratio    > 0.5
//	   AND NOT (is_safe_zone == 1)
//	THEN release release_amount
//	ELSE 0
//
// The historical average is computed via TableAggregate with a dynamic
// year-bound filter, exercising every code path added by task #040.
// The conditional is a 3-term composite with one Negate, exercising
// every code path added by task #039.
//
// 20 test cases (see docs/tasks/041-integration-test-039-040.md table)
// pin the boundary, branch-coverage and "all-true → release" scenarios.
// All historical averages are exact decimals (no recurring fractions)
// so the assertions can be straight string equality.
//
// This test ships in the standard go test ./... regression set so any
// future change to the conditional or aggregate path that breaks the
// integration is caught immediately.
func TestDisasterReserveRelease(t *testing.T) {
	// Reference table — 5 years of loss ratios. Picked so every
	// "year < N" subset has an exact-decimal mean.
	resolver := newStubResolver(map[string][]map[string]string{
		"loss_history": {
			{"year": "2018", "loss_ratio": "0.40"},
			{"year": "2019", "loss_ratio": "0.60"},
			{"year": "2020", "loss_ratio": "0.80"},
			{"year": "2021", "loss_ratio": "0.40"},
			{"year": "2022", "loss_ratio": "0.20"},
		},
	})

	// Build the formula graph once; all 20 cases reuse it.
	graph := buildDisasterReserveGraph(t)

	engine := NewEngine(EngineConfig{
		Workers:       1,
		Precision:     DefaultPrecision(),
		CacheSize:     16,
		TableResolver: resolver,
	})

	cases := []struct {
		name           string
		currentYear    string
		currentLossRat string
		isSafeZone     string
		releaseAmount  string
		want           string // expected formula output
		intent         string // human-readable test purpose
	}{
		{"01_avgBelowThreshold_avg2023", "2023", "0.70", "0", "100", "0", "c0 false (avg 0.48 not > 0.5)"},
		{"02_allTrue_year2022", "2022", "0.70", "0", "100", "100", "all conditions true → release"},
		{"03_allTrue_year2021", "2021", "0.70", "0", "100", "100", "all true, different year (avg 0.60)"},
		{"04_avgEqualsThreshold", "2020", "0.70", "0", "100", "0", "avg 0.50 not strictly > 0.5"},
		{"05_earliestYear", "2019", "0.70", "0", "100", "0", "avg 0.40 (single historical year)"},
		{"06_currLossLow", "2022", "0.40", "0", "100", "0", "c1 false (curr 0.40 not > 0.5)"},
		{"07_currEqualsThreshold", "2022", "0.50", "0", "100", "0", "curr 0.50 not strictly > 0.5"},
		{"08_currJustAbove", "2022", "0.51", "0", "100", "100", "curr 0.51 above threshold"},
		{"09_safeZone_year2022", "2022", "0.70", "1", "100", "0", "c2 false: NOT(safe==1) → false"},
		{"10_safeZone_year2021", "2021", "0.70", "1", "100", "0", "safe zone, different year"},
		{"11_releaseZero", "2022", "0.70", "0", "0", "0", "all true but release_amount=0"},
		{"12_largeRelease", "2022", "0.70", "0", "5000", "5000", "all true, large release"},
		{"13_normalRelease", "2021", "0.60", "0", "250", "250", "regular release"},
		{"14_avgFails_year2020", "2020", "0.60", "0", "250", "0", "c0 false at year 2020 (avg=0.50)"},
		{"15_currAndSafeBoundary", "2021", "0.51", "1", "999", "0", "c1 just true but c2 false"},
		{"16_irregularRelease", "2022", "0.61", "0", "12345", "12345", "non-round release amount"},
		{"17_extremeCurrLossLow_avg", "2023", "0.99", "0", "100", "0", "extreme curr but c0 still false"},
		{"18_extremeCurrLowestAvg", "2019", "0.99", "0", "100", "0", "lowest historical avg"},
		{"19_currJustAtBoundary", "2022", "0.55", "0", "7777", "7777", "curr 0.55 above threshold"},
		{"20_doubleFalse", "2022", "0.50", "1", "100", "0", "c1 AND c2 both false"},
	}

	if len(cases) != 20 {
		t.Fatalf("expected exactly 20 test cases, got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Calculate(context.Background(), graph, map[string]string{
				"current_year":       tc.currentYear,
				"current_loss_ratio": tc.currentLossRat,
				"is_safe_zone":       tc.isSafeZone,
				"release_amount":     tc.releaseAmount,
			})
			if err != nil {
				t.Fatalf("calculate %s: %v", tc.intent, err)
			}
			got := res.Outputs["cond"]
			if got != tc.want {
				t.Fatalf("[%s] %s\n  inputs: year=%s lr=%s safe=%s release=%s\n  got %q, want %q",
					tc.name, tc.intent, tc.currentYear, tc.currentLossRat, tc.isSafeZone, tc.releaseAmount, got, tc.want)
			}
		})
	}
}

// buildDisasterReserveGraph constructs the FormulaGraph used by
// TestDisasterReserveRelease. It is its own helper (rather than inlined)
// so that the test body stays focused on the case table — and so the
// graph topology is reviewable independently from the data.
func buildDisasterReserveGraph(t *testing.T) *domain.FormulaGraph {
	t.Helper()

	mustJSON := func(v interface{}) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		return b
	}

	// TableAggregate config: avg(loss_ratio) where year < bound
	aggCfg := domain.TableAggregateConfig{
		TableID:    "loss_history",
		Aggregate:  "avg",
		Expression: "loss_ratio",
		Filters: []domain.TableFilter{
			{Column: "year", Op: "lt", InputPort: "bound"},
		},
	}

	// Composite Conditional config: 3-term AND with one Negate
	condCfg := domain.ConditionalConfig{
		Combinator: "and",
		Conditions: []domain.ConditionTerm{
			{Op: "gt"},               // term 0: avg_historical > 0.5
			{Op: "gt"},               // term 1: current_loss_ratio > 0.5
			{Op: "eq", Negate: true}, // term 2: NOT (is_safe_zone == 1)
		},
	}

	graph := &domain.FormulaGraph{
		Nodes: []domain.FormulaNode{
			// ── Input variables ──
			{ID: "v_year", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "current_year"})},
			{ID: "v_curr_lr", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "current_loss_ratio"})},
			{ID: "v_safe", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "is_safe_zone"})},
			{ID: "v_release", Type: domain.NodeVariable, Config: mustJSON(domain.VariableConfig{Name: "release_amount"})},

			// ── Constants ──
			{ID: "c_avg_thresh", Type: domain.NodeConstant, Config: mustJSON(map[string]string{"value": "0.5"})},
			{ID: "c_curr_thresh", Type: domain.NodeConstant, Config: mustJSON(map[string]string{"value": "0.5"})},
			{ID: "c_safe_marker", Type: domain.NodeConstant, Config: mustJSON(map[string]string{"value": "1"})},
			{ID: "c_zero", Type: domain.NodeConstant, Config: mustJSON(map[string]string{"value": "0"})},

			// ── TableAggregate (task #040) ──
			{ID: "agg", Type: domain.NodeTableAggregate, Config: mustJSON(aggCfg)},

			// ── Composite Conditional (task #039) ──
			{ID: "cond", Type: domain.NodeConditional, Config: mustJSON(condCfg)},
		},
		Edges: []domain.FormulaEdge{
			// year variable feeds the table aggregate's dynamic filter port
			{Source: "v_year", Target: "agg", SourcePort: "out", TargetPort: "bound"},

			// Conditional's input wiring
			{Source: "agg", Target: "cond", SourcePort: "out", TargetPort: "condition_0"},
			{Source: "c_avg_thresh", Target: "cond", SourcePort: "out", TargetPort: "conditionRight_0"},
			{Source: "v_curr_lr", Target: "cond", SourcePort: "out", TargetPort: "condition_1"},
			{Source: "c_curr_thresh", Target: "cond", SourcePort: "out", TargetPort: "conditionRight_1"},
			{Source: "v_safe", Target: "cond", SourcePort: "out", TargetPort: "condition_2"},
			{Source: "c_safe_marker", Target: "cond", SourcePort: "out", TargetPort: "conditionRight_2"},
			{Source: "v_release", Target: "cond", SourcePort: "out", TargetPort: "thenValue"},
			{Source: "c_zero", Target: "cond", SourcePort: "out", TargetPort: "elseValue"},
		},
		Outputs: []string{"cond"},
	}
	return graph
}
