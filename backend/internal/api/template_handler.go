package api

import (
	"encoding/json"
	"net/http"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// FormulaTemplate is a pre-built formula template that users can instantiate.
type FormulaTemplate struct {
	ID          string                    `json:"id"`
	Domain      domain.InsuranceDomain    `json:"domain"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Graph       domain.FormulaGraph       `json:"graph"`
}

// TemplateHandler serves the static formula template catalogue.
type TemplateHandler struct{}

// List returns all built-in formula templates.
// GET /api/v1/templates
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": allTemplates})
}

// mustJSON encodes v as JSON; panics on error (only called at init time with
// known-good values, mirrors the helper in main.go).
func mustTplJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func tplVar(name string) json.RawMessage {
	return mustTplJSON(map[string]any{"name": name, "dataType": "decimal"})
}
func tplConst(value string) json.RawMessage {
	return mustTplJSON(map[string]any{"value": value})
}
func tplOp(op string) json.RawMessage {
	return mustTplJSON(map[string]any{"op": op})
}
func tplFn(fn string) json.RawMessage {
	return mustTplJSON(map[string]any{"fn": fn, "args": map[string]string{"places": "2"}})
}

// tplFnPlain builds a function node config without the round-specific
// args.places field. Used for sqrt / abs / ln / exp etc.
func tplFnPlain(fn string) json.RawMessage {
	return mustTplJSON(map[string]any{"fn": fn})
}

// tplTableAgg builds a TableAggregate node config (task #040). For
// templates the tableId is typically a placeholder the user fills in
// after instantiating the template — see the chain-ladder template
// for the in-tree example.
func tplTableAgg(tableID, aggregate, expression string, filters []domain.TableFilter, combinator string) json.RawMessage {
	return mustTplJSON(domain.TableAggregateConfig{
		TableID:          tableID,
		Aggregate:        aggregate,
		Expression:       expression,
		Filters:          filters,
		FilterCombinator: combinator,
	})
}

// allTemplates is the static catalogue of built-in templates.
var allTemplates = []FormulaTemplate{
	// ─── Life Insurance ──────────────────────────────────────────────
	{
		ID:          "tpl-life-net-premium",
		Domain:      "life",
		Name:        "寿险净保费",
		Description: "净保费 = 保额 × 死亡率 × 贴现因子（v = 1/(1+i)）。输入: sumAssured, qx, interestRate",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("sumAssured")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("qx")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("interestRate")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("add")},     // 1 + interestRate
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("divide")},  // v = 1 / (1+i)
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")}, // sumAssured × qx
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("multiply")}, // × v
				{ID: "n9", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n8", SourcePort: "out", TargetPort: "right"},
				{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n9"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 160}, "n3": {X: 50, Y: 270},
				"n4": {X: 50, Y: 380}, "n5": {X: 260, Y: 325}, "n6": {X: 470, Y: 325},
				"n7": {X: 260, Y: 105}, "n8": {X: 580, Y: 180}, "n9": {X: 750, Y: 180},
			}},
		},
	},
	{
		ID:          "tpl-life-term-risk",
		Domain:      "life",
		Name:        "定期寿险风险保费",
		Description: "风险保费 = 保额 × 死亡率 × (1 + 附加费用率)。输入: sumAssured, qx, loadingRatio",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("sumAssured")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("qx")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("loadingRatio")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("add")},      // 1 + loadingRatio
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")}, // sumAssured × qx
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")}, // × loading
				{ID: "n8", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n8"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 160}, "n3": {X: 50, Y: 270},
				"n4": {X: 50, Y: 380}, "n5": {X: 260, Y: 325}, "n6": {X: 260, Y: 105},
				"n7": {X: 470, Y: 215}, "n8": {X: 650, Y: 215},
			}},
		},
	},
	{
		ID:          "tpl-life-annuity",
		Domain:      "life",
		Name:        "年金现值",
		Description: "年金现值 = 年给付额 × 年金系数。输入: annualBenefit, annuityFactor",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("annualBenefit")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("annuityFactor")},
				{ID: "n3", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n4", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n3", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n3", SourcePort: "out", TargetPort: "right"},
				{Source: "n3", Target: "n4", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n4"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 80}, "n2": {X: 50, Y: 200},
				"n3": {X: 260, Y: 140}, "n4": {X: 460, Y: 140},
			}},
		},
	},

	// ─── Property Insurance ──────────────────────────────────────────
	{
		ID:          "tpl-property-basic",
		Domain:      "property",
		Name:        "财产险基础保费",
		Description: "保费 = 保险价值 × 基础费率 × 使用性质系数。输入: insuredValue, baseRate, occupancyFactor",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("insuredValue")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("baseRate")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("occupancyFactor")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("multiply")}, // insuredValue × baseRate
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")}, // × occupancyFactor
				{ID: "n6", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n6"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 170}, "n3": {X: 50, Y: 290},
				"n4": {X: 260, Y: 110}, "n5": {X: 470, Y: 200}, "n6": {X: 660, Y: 200},
			}},
		},
	},
	{
		ID:          "tpl-property-home",
		Domain:      "property",
		Name:        "家财险保费",
		Description: "保费 = 建筑物价值 × 建筑费率 + 室内财产价值 × 财产费率。输入: buildingValue, buildingRate, contentsValue, contentsRate",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("buildingValue")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("buildingRate")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("contentsValue")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("contentsRate")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")}, // building premium
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")}, // contents premium
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("add")},      // total
				{ID: "n8", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n8"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 160}, "n3": {X: 50, Y: 290},
				"n4": {X: 50, Y: 400}, "n5": {X: 270, Y: 105}, "n6": {X: 270, Y: 345},
				"n7": {X: 490, Y: 225}, "n8": {X: 680, Y: 225},
			}},
		},
	},
	{
		ID:          "tpl-property-engineering",
		Domain:      "property",
		Name:        "工程险保费",
		Description: "保费 = 合同金额 × 基础费率 × 工程性质系数 × 工期系数。输入: contractAmount, baseRate, projectFactor, durationFactor",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("contractAmount")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("baseRate")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("projectFactor")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("durationFactor")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n8", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n8"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 170}, "n3": {X: 50, Y: 290},
				"n4": {X: 50, Y: 410}, "n5": {X: 260, Y: 110}, "n6": {X: 460, Y: 200},
				"n7": {X: 650, Y: 290}, "n8": {X: 830, Y: 290},
			}},
		},
	},

	// ─── Auto Insurance ──────────────────────────────────────────────
	{
		ID:          "tpl-auto-commercial",
		Domain:      "auto",
		Name:        "车险商业保费",
		Description: "商业保费 = 基础保费 × 车型系数 × 驾驶人系数 × 无赔款优待系数。输入: basePremium, vehicleFactor, driverFactor, ncdDiscount",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("basePremium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("vehicleFactor")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("driverFactor")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("ncdDiscount")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n8", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n8"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 170}, "n3": {X: 50, Y: 290},
				"n4": {X: 50, Y: 410}, "n5": {X: 260, Y: 110}, "n6": {X: 460, Y: 200},
				"n7": {X: 650, Y: 290}, "n8": {X: 830, Y: 290},
			}},
		},
	},
	{
		ID:          "tpl-auto-compulsory",
		Domain:      "auto",
		Name:        "交强险保费",
		Description: "交强险 = 基础费率 × 车型系数。输入: basePremium, vehicleTypeFactor",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("basePremium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("vehicleTypeFactor")},
				{ID: "n3", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n4", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n3", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n3", SourcePort: "out", TargetPort: "right"},
				{Source: "n3", Target: "n4", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n4"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 80}, "n2": {X: 50, Y: 200},
				"n3": {X: 260, Y: 140}, "n4": {X: 460, Y: 140},
			}},
		},
	},
	{
		ID:          "tpl-auto-ubi",
		Domain:      "auto",
		Name:        "UBI 里程险保费",
		Description: "UBI保费 = 基础保费 × 里程系数 × 驾驶评分系数。输入: basePremium, mileageFactor, drivingScoreFactor",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("basePremium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("mileageFactor")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("drivingScoreFactor")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("multiply")}, // base × mileage
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")}, // × drivingScore
				{ID: "n6", Type: domain.NodeFunction, Config: tplFn("round")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "in"},
			},
			Outputs: []string{"n6"},
			Layout: &domain.GraphLayout{Positions: map[string]domain.Position{
				"n1": {X: 50, Y: 50}, "n2": {X: 50, Y: 170}, "n3": {X: 50, Y: 290},
				"n4": {X: 260, Y: 110}, "n5": {X: 460, Y: 200}, "n6": {X: 650, Y: 200},
			}},
		},
	},

	// ───────────────────────────────────────────────────────────────
	// Task #045: 17 actuarial templates from spec 002 (Japanese
	// insurance coverage analysis), excluding #2 (already covered by
	// the existing 寿险净保费 / tpl-life-net-premium pair), #14 (needs
	// normal-quantile function not yet in the engine), and #19
	// (release rule belongs in user-defined business logic).
	//
	// Each template mirrors a seed formula in main.go. Layout is left
	// nil so the visual editor auto-arranges nodes when the user
	// instantiates the template. The chain-ladder template is the
	// only one that references a real seeded table by name (a
	// placeholder tableId users replace after instantiating).
	// ───────────────────────────────────────────────────────────────

	// ── Spec #1: 死亡率 q_x = d_x / l_x ───────────────────────────
	{
		ID:          "tpl-jp-01-mortality-rate",
		Domain:      "life",
		Name:        "日本生命保険 死亡率 qx",
		Description: "Mortality rate q_x = d_x / l_x. 入力: d, l",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("d")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("l")},
				{ID: "n3", Type: domain.NodeOperator, Config: tplOp("divide")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n3", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n3", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n3"},
		},
	},

	// ── Spec #3: 基数 D_x = v^x · l_x ─────────────────────────────
	{
		ID:          "tpl-jp-03-commutation-d",
		Domain:      "life",
		Name:        "日本生命保険 基数 Dx",
		Description: "Commutation function D_x = v^x · l_x. C_x, M_x, N_x も同様の構造で構築可能",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("v")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("x")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("l")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("power")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #4: 将来法責任準備金 V = A − P · ä ────────────────────
	{
		ID:          "tpl-jp-04-prospective-reserve",
		Domain:      "life",
		Name:        "日本生命保険 将来法責任準備金",
		Description: "Prospective reserve V = A − P · ä. 入力: A(将来給付現価), P(純保険料), a(年金現価)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("A")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("P")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("a")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("subtract")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #5: チルメル式責任準備金 Vz = V − α(1 − a_part/a_full) ──
	{
		ID:          "tpl-jp-05-zillmer-reserve",
		Domain:      "life",
		Name:        "日本生命保険 チルメル式責任準備金",
		Description: "Zillmer reserve V_z = V − α(1 − a_part/a_full). 入力: V, alpha, a_part, a_full",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("V")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("alpha")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("a_part")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("a_full")},
				{ID: "n5", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("divide")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n9", Type: domain.NodeOperator, Config: tplOp("subtract")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n2", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n9", SourcePort: "out", TargetPort: "left"},
				{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n9"},
		},
	},

	// ── Spec #6: 損害率 LR = (paid + adj) / premium ───────────────
	{
		ID:          "tpl-jp-06-loss-ratio",
		Domain:      "property",
		Name:        "日本損害保険 損害率",
		Description: "Loss ratio LR = (正味支払保険金 + 損害調査費) / 正味収入保険料",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("paid")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("adj")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("premium")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("divide")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #7: 発生保険金 incurred = paid + (end_res − begin_res) ──
	{
		ID:          "tpl-jp-07-incurred-claims",
		Domain:      "property",
		Name:        "日本損害保険 発生保険金",
		Description: "Incurred losses = 当期支払保険金 + (期末支払備金 − 期初支払備金)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("paid")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("end_res")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("begin_res")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("add")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #8: チェインラダー LDF (single development year) ─────
	// Uses a TableAggregate node. The tableId here is a placeholder
	// the user must replace with their own claims-triangle table id
	// after instantiating the template.
	{
		ID:          "tpl-jp-08-chain-ladder-ldf",
		Domain:      "property",
		Name:        "日本損害保険 チェインラダー LDF",
		Description: "Chain ladder LDF[j] = avg(development_ratio WHERE dev_year = j). instantiating user must replace tableId with their own claims-triangle table",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("dev_year")},
				{
					ID:   "n2",
					Type: domain.NodeTableAggregate,
					Config: tplTableAgg("REPLACE_WITH_TABLE_ID", "avg", "development_ratio",
						[]domain.TableFilter{
							{Column: "dev_year", Op: "eq", InputPort: "dev_year"},
						}, "and"),
				},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n2", SourcePort: "out", TargetPort: "dev_year"},
			},
			Outputs: []string{"n2"},
		},
	},

	// ── Spec #9: BF 法 ult = C + E · (1 − 1/f) ────────────────────
	{
		ID:          "tpl-jp-09-bornhuetter-ferguson",
		Domain:      "property",
		Name:        "日本損害保険 BF法予測",
		Description: "Bornhuetter-Ferguson final loss = C + E · (1 − 1/f)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("C")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("E")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("f")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("divide")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("add")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n2", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n8"},
		},
	},

	// ── Spec #10: 法定 IBNR 要積立額 b = avg(直近3年) / 12 ────────
	{
		ID:          "tpl-jp-10-ibnr-statutory-b",
		Domain:      "property",
		Name:        "日本損害保険 法定IBNR要積立額b",
		Description: "Statutory IBNR amount b = (y1 + y2 + y3) / 3 / 12 (1/12 法)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("y1")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("y2")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("y3")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("3")},
				{ID: "n5", Type: domain.NodeConstant, Config: tplConst("12")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("divide")},
				{ID: "n9", Type: domain.NodeOperator, Config: tplOp("divide")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n8", SourcePort: "out", TargetPort: "right"},
				{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n9", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n9"},
		},
	},

	// ── Spec #11: 1/24 法未経過保険料 ─────────────────────────────
	// Closed form for the UNEARNED fraction:
	//   unearned_fraction = (2·start_month − 1) / 24
	// 1月始期 → 1/24 (only 0.5 months elapsed by year-end... wait,
	// month=1 means the contract started in January, so by Dec 31
	// 11.5 months have elapsed → 23/24 earned, 1/24 unearned).
	{
		ID:          "tpl-jp-11-unearned-1-24",
		Domain:      "property",
		Name:        "日本損害保険 1/24法未経過保険料",
		Description: "Unearned premium under 1/24 method: premium · (2·start_month − 1)/24. 1月始期 → 1/24, 12月始期 → 23/24 が未経過",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("premium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("start_month")},
				{ID: "n3", Type: domain.NodeConstant, Config: tplConst("2")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n5", Type: domain.NodeConstant, Config: tplConst("24")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")},  // 2 * start_month
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("subtract")},  // 2*start_month - 1
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("divide")},    // /24
				{ID: "n9", Type: domain.NodeOperator, Config: tplOp("multiply")},  // premium *
			},
			Edges: []domain.FormulaEdge{
				{Source: "n3", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n8", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n9", SourcePort: "out", TargetPort: "left"},
				{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n9"},
		},
	},

	// ── Spec #12: 短期料率返還 refund = premium · (1 − short_rate) ──
	{
		ID:          "tpl-jp-12-shortrate-refund",
		Domain:      "property",
		Name:        "日本損害保険 短期料率返還",
		Description: "Short-rate refund = annual_premium · (1 − short_rate)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("premium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("short_rate")},
				{ID: "n3", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n3", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #13: Bühlmann μ̂ = (n·X + K·μ) / (n + K) ─────────────
	{
		ID:          "tpl-jp-13-buhlmann-credibility",
		Domain:      "property",
		Name:        "日本損害保険 Bühlmann信頼度",
		Description: "Bühlmann credibility μ̂ = (n·X + K·μ) / (n+K)",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("X")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("mu")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("n")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("K")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n8", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n9", Type: domain.NodeOperator, Config: tplOp("divide")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n1", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "right"},
				{Source: "n3", Target: "n8", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n8", SourcePort: "out", TargetPort: "right"},
				{Source: "n7", Target: "n9", SourcePort: "out", TargetPort: "left"},
				{Source: "n8", Target: "n9", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n9"},
		},
	},

	// ── Spec #15: 休業損害（自賠責）loss = 6100 · days ────────────
	{
		ID:          "tpl-jp-15-lost-wages-jibaiseki",
		Domain:      "auto",
		Name:        "日本損害賠償 休業損害(自賠責基準)",
		Description: "Lost wages under jibaiseki standard: 6,100 yen × 休業日数",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("days")},
				{ID: "n2", Type: domain.NodeConstant, Config: tplConst("6100")},
				{ID: "n3", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n2", Target: "n3", SourcePort: "out", TargetPort: "left"},
				{Source: "n1", Target: "n3", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n3"},
		},
	},

	// ── Spec #16: 逸失利益 lost = income · (1 − rate) · leibniz ───
	{
		ID:          "tpl-jp-16-lost-future-income",
		Domain:      "auto",
		Name:        "日本損害賠償 逸失利益",
		Description: "Lost future income = annual_income · (1 − living_expense_rate) · leibniz_coefficient",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("income")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("rate")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("leibniz")},
				{ID: "n4", Type: domain.NodeConstant, Config: tplConst("1")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n6", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n7", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n5", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n6", SourcePort: "out", TargetPort: "left"},
				{Source: "n5", Target: "n6", SourcePort: "out", TargetPort: "right"},
				{Source: "n6", Target: "n7", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n7", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n7"},
		},
	},

	// ── Spec #17: SMR ソルベンシー・マージン比率 ──────────────────
	{
		ID:          "tpl-jp-17-solvency-margin-ratio",
		Domain:      "life",
		Name:        "日本生命保険 ソルベンシー・マージン比率",
		Description: "Solvency margin ratio = SMM / (0.5 · sqrt(R1² + (R2+R3)² + R4²)) · 100",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("SMM")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("R1")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("R2")},
				{ID: "n4", Type: domain.NodeVariable, Config: tplVar("R3")},
				{ID: "n5", Type: domain.NodeVariable, Config: tplVar("R4")},
				{ID: "n6", Type: domain.NodeConstant, Config: tplConst("2")},
				{ID: "n7", Type: domain.NodeConstant, Config: tplConst("0.5")},
				{ID: "n8", Type: domain.NodeConstant, Config: tplConst("100")},
				{ID: "n9", Type: domain.NodeOperator, Config: tplOp("power")},
				{ID: "n10", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n11", Type: domain.NodeOperator, Config: tplOp("power")},
				{ID: "n12", Type: domain.NodeOperator, Config: tplOp("power")},
				{ID: "n13", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n14", Type: domain.NodeOperator, Config: tplOp("add")},
				{ID: "n15", Type: domain.NodeFunction, Config: tplFnPlain("sqrt")},
				{ID: "n16", Type: domain.NodeOperator, Config: tplOp("multiply")},
				{ID: "n17", Type: domain.NodeOperator, Config: tplOp("divide")},
				{ID: "n18", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n2", Target: "n9", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n9", SourcePort: "out", TargetPort: "right"},
				{Source: "n3", Target: "n10", SourcePort: "out", TargetPort: "left"},
				{Source: "n4", Target: "n10", SourcePort: "out", TargetPort: "right"},
				{Source: "n10", Target: "n11", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n11", SourcePort: "out", TargetPort: "right"},
				{Source: "n5", Target: "n12", SourcePort: "out", TargetPort: "left"},
				{Source: "n6", Target: "n12", SourcePort: "out", TargetPort: "right"},
				{Source: "n9", Target: "n13", SourcePort: "out", TargetPort: "left"},
				{Source: "n11", Target: "n13", SourcePort: "out", TargetPort: "right"},
				{Source: "n13", Target: "n14", SourcePort: "out", TargetPort: "left"},
				{Source: "n12", Target: "n14", SourcePort: "out", TargetPort: "right"},
				{Source: "n14", Target: "n15", SourcePort: "out", TargetPort: "in"},
				{Source: "n7", Target: "n16", SourcePort: "out", TargetPort: "left"},
				{Source: "n15", Target: "n16", SourcePort: "out", TargetPort: "right"},
				{Source: "n1", Target: "n17", SourcePort: "out", TargetPort: "left"},
				{Source: "n16", Target: "n17", SourcePort: "out", TargetPort: "right"},
				{Source: "n17", Target: "n18", SourcePort: "out", TargetPort: "left"},
				{Source: "n8", Target: "n18", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n18"},
		},
	},

	// ── Spec #18: 逆ざや (planned − actual) · reserve ─────────────
	{
		ID:          "tpl-jp-18-negative-spread",
		Domain:      "life",
		Name:        "日本生命保険 逆ざや",
		Description: "Negative interest spread = (平均予定利率 − 運用利回り) × 利息算入対象責任準備金",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("planned")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("actual")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("reserve")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("multiply")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},

	// ── Spec #20: 自賠責収支調整 surplus = premium − paid − admin ──
	{
		ID:          "tpl-jp-20-cali-surplus",
		Domain:      "auto",
		Name:        "日本自賠責 収支調整剰余金",
		Description: "CALI no-profit/no-loss surplus = collected_premium − claims_paid − admin_expense",
		Graph: domain.FormulaGraph{
			Nodes: []domain.FormulaNode{
				{ID: "n1", Type: domain.NodeVariable, Config: tplVar("premium")},
				{ID: "n2", Type: domain.NodeVariable, Config: tplVar("paid")},
				{ID: "n3", Type: domain.NodeVariable, Config: tplVar("admin")},
				{ID: "n4", Type: domain.NodeOperator, Config: tplOp("subtract")},
				{ID: "n5", Type: domain.NodeOperator, Config: tplOp("subtract")},
			},
			Edges: []domain.FormulaEdge{
				{Source: "n1", Target: "n4", SourcePort: "out", TargetPort: "left"},
				{Source: "n2", Target: "n4", SourcePort: "out", TargetPort: "right"},
				{Source: "n4", Target: "n5", SourcePort: "out", TargetPort: "left"},
				{Source: "n3", Target: "n5", SourcePort: "out", TargetPort: "right"},
			},
			Outputs: []string{"n5"},
		},
	},
}
