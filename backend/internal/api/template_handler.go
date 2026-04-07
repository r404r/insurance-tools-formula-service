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
}
