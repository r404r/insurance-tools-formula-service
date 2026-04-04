package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// ASTToDAG flattens an AST tree into a FormulaGraph (flat node list with edges).
// Each AST node gets a unique incrementing ID. Edges are wired up from child
// (source) to parent (target) with port names describing the role.
func ASTToDAG(node *ASTNode) (*domain.FormulaGraph, error) {
	g := &domain.FormulaGraph{}
	counter := 0

	rootID, err := astToDAGWalk(node, g, &counter)
	if err != nil {
		return nil, err
	}

	g.Outputs = []string{rootID}
	return g, nil
}

func nextID(counter *int) string {
	id := fmt.Sprintf("n%d", *counter)
	*counter++
	return id
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func astToDAGWalk(node *ASTNode, g *domain.FormulaGraph, counter *int) (string, error) {
	id := nextID(counter)

	switch node.Kind {
	case KindLiteral:
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeConstant,
			Config: mustMarshal(domain.ConstantConfig{Value: node.Value}),
		})
		return id, nil

	case KindVariable:
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:   id,
			Type: domain.NodeVariable,
			Config: mustMarshal(domain.VariableConfig{
				Name:     node.Value,
				DataType: "decimal",
			}),
		})
		return id, nil

	case KindBinaryOp:
		opName := binaryOpName(node.Op)
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeOperator,
			Config: mustMarshal(domain.OperatorConfig{Op: opName}),
		})
		leftID, err := astToDAGWalk(node.Children[0], g, counter)
		if err != nil {
			return "", err
		}
		rightID, err := astToDAGWalk(node.Children[1], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: leftID, Target: id, SourcePort: "out", TargetPort: "left"},
			domain.FormulaEdge{Source: rightID, Target: id, SourcePort: "out", TargetPort: "right"},
		)
		return id, nil

	case KindUnaryOp:
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeOperator,
			Config: mustMarshal(domain.OperatorConfig{Op: "negate"}),
		})
		childID, err := astToDAGWalk(node.Children[0], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: childID, Target: id, SourcePort: "out", TargetPort: "left"},
		)
		return id, nil

	case KindComparison:
		compName := comparatorName(node.Op)
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeConditional,
			Config: mustMarshal(domain.ConditionalConfig{Comparator: compName}),
		})
		leftID, err := astToDAGWalk(node.Children[0], g, counter)
		if err != nil {
			return "", err
		}
		rightID, err := astToDAGWalk(node.Children[1], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: leftID, Target: id, SourcePort: "out", TargetPort: "left"},
			domain.FormulaEdge{Source: rightID, Target: id, SourcePort: "out", TargetPort: "right"},
		)
		return id, nil

	case KindConditional:
		// The engine and frontend expect a single conditional node with four
		// inputs: condition, conditionRight, thenValue, elseValue and a
		// comparator stored in config.
		cond := node.Children[0]

		var comp string
		var condLeftNode, condRightNode *ASTNode

		if cond.Kind == KindComparison {
			// "if X > Y then ..." — extract comparator and both sides.
			comp = comparatorName(cond.Op)
			condLeftNode = cond.Children[0]
			condRightNode = cond.Children[1]
		} else {
			// "if flag then ..." — treat as "flag != 0".
			comp = "ne"
			condLeftNode = cond
			condRightNode = &ASTNode{Kind: KindLiteral, Value: "0"}
		}

		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:     id,
			Type:   domain.NodeConditional,
			Config: mustMarshal(domain.ConditionalConfig{Comparator: comp}),
		})

		condLeftID, err := astToDAGWalk(condLeftNode, g, counter)
		if err != nil {
			return "", err
		}
		condRightID, err := astToDAGWalk(condRightNode, g, counter)
		if err != nil {
			return "", err
		}
		thenID, err := astToDAGWalk(node.Children[1], g, counter)
		if err != nil {
			return "", err
		}
		elseID, err := astToDAGWalk(node.Children[2], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: condLeftID, Target: id, SourcePort: "out", TargetPort: "condition"},
			domain.FormulaEdge{Source: condRightID, Target: id, SourcePort: "out", TargetPort: "conditionRight"},
			domain.FormulaEdge{Source: thenID, Target: id, SourcePort: "out", TargetPort: "thenValue"},
			domain.FormulaEdge{Source: elseID, Target: id, SourcePort: "out", TargetPort: "elseValue"},
		)
		return id, nil

	case KindFunctionCall:
		fnLower := strings.ToLower(node.FuncName)
		if fnLower == "lookup" {
			return astToDAGLookup(id, node, g, counter)
		}
		if fnLower == "subformula" {
			return astToDAGSubFormula(id, node, g, counter)
		}
		args := make(map[string]string)
		// For round/floor/ceil with a precision argument, store it.
		if (fnLower == "round" || fnLower == "floor" || fnLower == "ceil") && len(node.Children) >= 2 {
			if node.Children[1].Kind == KindLiteral {
				args["places"] = node.Children[1].Value
			}
		}
		g.Nodes = append(g.Nodes, domain.FormulaNode{
			ID:   id,
			Type: domain.NodeFunction,
			Config: mustMarshal(domain.FunctionConfig{
				Fn:   fnLower,
				Args: args,
			}),
		})
		// Use port names that match React Flow handle IDs:
		// min/max take two inputs on left/right; everything else uses "in".
		twoInput := fnLower == "min" || fnLower == "max"
		argPorts := []string{"left", "right"}
		for i, child := range node.Children {
			// For round/floor/ceil the second child is the precision stored in config, skip wiring it as an edge.
			if (fnLower == "round" || fnLower == "floor" || fnLower == "ceil") && i == 1 && child.Kind == KindLiteral {
				continue
			}
			childID, err := astToDAGWalk(child, g, counter)
			if err != nil {
				return "", err
			}
			var port string
			if twoInput && i < len(argPorts) {
				port = argPorts[i]
			} else {
				port = "in"
			}
			g.Edges = append(g.Edges,
				domain.FormulaEdge{Source: childID, Target: id, SourcePort: "out", TargetPort: port},
			)
		}
		return id, nil

	default:
		return "", fmt.Errorf("unknown AST node kind: %d", node.Kind)
	}
}

func astToDAGLookup(id string, node *ASTNode, g *domain.FormulaGraph, counter *int) (string, error) {
	cfg := domain.TableLookupConfig{}
	// lookup(tableName, keyExpr) or lookup(tableName, keyExpr, column)
	if len(node.Children) >= 1 && node.Children[0].Kind == KindVariable {
		cfg.TableID = node.Children[0].Value
	}
	if len(node.Children) >= 3 && node.Children[2].Kind == KindVariable {
		cfg.Column = node.Children[2].Value
	}
	g.Nodes = append(g.Nodes, domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeTableLookup,
		Config: mustMarshal(cfg),
	})
	// The key expression (second arg) becomes an edge input.
	if len(node.Children) >= 2 {
		keyID, err := astToDAGWalk(node.Children[1], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: keyID, Target: id, SourcePort: "out", TargetPort: "key"},
		)
	}
	return id, nil
}

func astToDAGSubFormula(id string, node *ASTNode, g *domain.FormulaGraph, counter *int) (string, error) {
	cfg := domain.SubFormulaConfig{}
	// subFormula(formulaId) or subFormula(formulaId, inputExpr)
	if len(node.Children) >= 1 && node.Children[0].Kind == KindVariable {
		cfg.FormulaID = node.Children[0].Value
	}
	g.Nodes = append(g.Nodes, domain.FormulaNode{
		ID:     id,
		Type:   domain.NodeSubFormula,
		Config: mustMarshal(cfg),
	})
	// The optional second arg becomes an edge input.
	if len(node.Children) >= 2 {
		inputID, err := astToDAGWalk(node.Children[1], g, counter)
		if err != nil {
			return "", err
		}
		g.Edges = append(g.Edges,
			domain.FormulaEdge{Source: inputID, Target: id, SourcePort: "out", TargetPort: "in"},
		)
	}
	return id, nil
}

func binaryOpName(op string) string {
	switch op {
	case "+":
		return "add"
	case "-":
		return "subtract"
	case "*":
		return "multiply"
	case "/":
		return "divide"
	case "^":
		return "power"
	case "%":
		return "modulo"
	default:
		return op
	}
}

func binaryOpSymbol(name string) string {
	switch name {
	case "add":
		return "+"
	case "subtract":
		return "-"
	case "multiply":
		return "*"
	case "divide":
		return "/"
	case "power":
		return "^"
	case "modulo":
		return "%"
	default:
		return name
	}
}

func comparatorName(op string) string {
	switch op {
	case ">":
		return "gt"
	case "<":
		return "lt"
	case ">=":
		return "ge"
	case "<=":
		return "le"
	case "==":
		return "eq"
	case "!=":
		return "ne"
	default:
		return op
	}
}

func comparatorSymbol(name string) string {
	switch name {
	case "gt":
		return ">"
	case "lt":
		return "<"
	case "ge":
		return ">="
	case "le":
		return "<="
	case "eq":
		return "=="
	case "ne":
		return "!="
	default:
		return name
	}
}

// DAGToAST reconstructs an AST tree from a FormulaGraph by finding the output
// node and walking edges backwards.
func DAGToAST(graph *domain.FormulaGraph) (*ASTNode, error) {
	if len(graph.Outputs) == 0 {
		return nil, fmt.Errorf("graph has no output nodes")
	}

	nodeMap := make(map[string]*domain.FormulaNode, len(graph.Nodes))
	for i := range graph.Nodes {
		nodeMap[graph.Nodes[i].ID] = &graph.Nodes[i]
	}

	// Build incoming-edge map: target -> list of edges sorted by port.
	inEdges := make(map[string][]domain.FormulaEdge)
	for _, e := range graph.Edges {
		inEdges[e.Target] = append(inEdges[e.Target], e)
	}

	rootID := graph.Outputs[0]
	return dagToASTWalk(rootID, nodeMap, inEdges)
}

func dagToASTWalk(
	nodeID string,
	nodeMap map[string]*domain.FormulaNode,
	inEdges map[string][]domain.FormulaEdge,
) (*ASTNode, error) {
	fn, ok := nodeMap[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %s not found in graph", nodeID)
	}

	switch fn.Type {
	case domain.NodeConstant:
		var cfg domain.ConstantConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad constant config: %w", nodeID, err)
		}
		return &ASTNode{Kind: KindLiteral, Value: cfg.Value}, nil

	case domain.NodeVariable:
		var cfg domain.VariableConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad variable config: %w", nodeID, err)
		}
		return &ASTNode{Kind: KindVariable, Value: cfg.Name}, nil

	case domain.NodeOperator:
		var cfg domain.OperatorConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad operator config: %w", nodeID, err)
		}
		edges := inEdges[nodeID]
		if cfg.Op == "negate" {
			operandID := findEdgeSource(edges, "left")
			if operandID == "" {
				operandID = findEdgeSource(edges, "operand")
			}
			if operandID == "" && len(edges) > 0 {
				operandID = edges[0].Source
			}
			child, err := dagToASTWalk(operandID, nodeMap, inEdges)
			if err != nil {
				return nil, err
			}
			return &ASTNode{Kind: KindUnaryOp, Op: "-", Children: []*ASTNode{child}}, nil
		}
		leftID := findEdgeSource(edges, "left")
		rightID := findEdgeSource(edges, "right")
		if leftID == "" || rightID == "" {
			return nil, fmt.Errorf("node %s: operator missing left or right input", nodeID)
		}
		left, err := dagToASTWalk(leftID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}
		right, err := dagToASTWalk(rightID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}
		return &ASTNode{
			Kind:     KindBinaryOp,
			Op:       binaryOpSymbol(cfg.Op),
			Children: []*ASTNode{left, right},
		}, nil

	case domain.NodeFunction:
		var cfg domain.FunctionConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad function config: %w", nodeID, err)
		}
		edges := inEdges[nodeID]
		node := &ASTNode{Kind: KindFunctionCall, FuncName: cfg.Fn}
		// Collect args: try arg0/arg1 ports first (legacy), then left/right for min/max, then "in".
		argEdges := sortArgEdges(edges)
		if len(argEdges) == 0 {
			// New-style port names: left/right for min/max, "in" for single-input functions.
			fnLower := strings.ToLower(cfg.Fn)
			if fnLower == "min" || fnLower == "max" {
				if leftID := findEdgeSource(edges, "left"); leftID != "" {
					argEdges = append(argEdges, domain.FormulaEdge{Source: leftID, Target: nodeID, TargetPort: "left"})
				}
				if rightID := findEdgeSource(edges, "right"); rightID != "" {
					argEdges = append(argEdges, domain.FormulaEdge{Source: rightID, Target: nodeID, TargetPort: "right"})
				}
			} else {
				for _, e := range edges {
					if e.TargetPort == "in" {
						argEdges = append(argEdges, e)
					}
				}
			}
		}
		for _, e := range argEdges {
			child, err := dagToASTWalk(e.Source, nodeMap, inEdges)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
		}
		// If the function has a places arg, inject it as a literal child.
		if places, ok := cfg.Args["places"]; ok {
			node.Children = append(node.Children, &ASTNode{Kind: KindLiteral, Value: places})
		}
		return node, nil

	case domain.NodeSubFormula:
		var cfg domain.SubFormulaConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad sub-formula config: %w", nodeID, err)
		}
		edges := inEdges[nodeID]
		node := &ASTNode{Kind: KindFunctionCall, FuncName: "subFormula"}
		node.Children = append(node.Children, &ASTNode{Kind: KindVariable, Value: cfg.FormulaID})
		if inputID := findEdgeSource(edges, "in"); inputID != "" {
			child, err := dagToASTWalk(inputID, nodeMap, inEdges)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
		}
		return node, nil

	case domain.NodeConditional:
		var cfg domain.ConditionalConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad conditional config: %w", nodeID, err)
		}
		edges := inEdges[nodeID]

		// Merged conditional node: comparator + condition/conditionRight/thenValue/elseValue.
		condLeftID := findEdgeSource(edges, "condition")
		condRightID := findEdgeSource(edges, "conditionRight")
		thenID := findEdgeSource(edges, "thenValue")
		elseID := findEdgeSource(edges, "elseValue")
		if condLeftID == "" || condRightID == "" || thenID == "" || elseID == "" {
			return nil, fmt.Errorf("node %s: conditional missing condition/conditionRight/thenValue/elseValue", nodeID)
		}

		condLeft, err := dagToASTWalk(condLeftID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}
		condRight, err := dagToASTWalk(condRightID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}
		thenNode, err := dagToASTWalk(thenID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}
		elseNode, err := dagToASTWalk(elseID, nodeMap, inEdges)
		if err != nil {
			return nil, err
		}

		comparison := &ASTNode{
			Kind:     KindComparison,
			Op:       comparatorSymbol(cfg.Comparator),
			Children: []*ASTNode{condLeft, condRight},
		}
		return &ASTNode{
			Kind:     KindConditional,
			Children: []*ASTNode{comparison, thenNode, elseNode},
		}, nil

	case domain.NodeTableLookup:
		var cfg domain.TableLookupConfig
		if err := json.Unmarshal(fn.Config, &cfg); err != nil {
			return nil, fmt.Errorf("node %s: bad table lookup config: %w", nodeID, err)
		}
		edges := inEdges[nodeID]
		node := &ASTNode{Kind: KindFunctionCall, FuncName: "lookup"}
		// First arg: table name as a variable reference.
		node.Children = append(node.Children, &ASTNode{Kind: KindVariable, Value: cfg.TableID})
		// Second arg: the key expression.
		keyID := findEdgeSource(edges, "key")
		if keyID != "" {
			keyNode, err := dagToASTWalk(keyID, nodeMap, inEdges)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, keyNode)
		}
		// Third arg: column name if present.
		if cfg.Column != "" {
			node.Children = append(node.Children, &ASTNode{Kind: KindVariable, Value: cfg.Column})
		}
		return node, nil

	default:
		return nil, fmt.Errorf("unsupported node type %s for DAG-to-AST conversion", fn.Type)
	}
}

func findEdgeSource(edges []domain.FormulaEdge, targetPort string) string {
	for _, e := range edges {
		if e.TargetPort == targetPort {
			return e.Source
		}
	}
	return ""
}

// sortArgEdges returns edges sorted by their target port name (arg0, arg1, ...).
func sortArgEdges(edges []domain.FormulaEdge) []domain.FormulaEdge {
	// Collect only edges with "arg" prefix, sort by suffix.
	var result []domain.FormulaEdge
	for i := 0; ; i++ {
		port := fmt.Sprintf("arg%d", i)
		found := false
		for _, e := range edges {
			if e.TargetPort == port {
				result = append(result, e)
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return result
}

// ASTToText pretty-prints an AST back to a formula text expression.
// It emits minimal parentheses by respecting operator precedence.
func ASTToText(node *ASTNode) string {
	var sb strings.Builder
	writeNode(&sb, node, precNone)
	return sb.String()
}

func writeNode(sb *strings.Builder, node *ASTNode, parentPrec int) {
	switch node.Kind {
	case KindLiteral:
		sb.WriteString(node.Value)

	case KindVariable:
		sb.WriteString(node.Value)

	case KindBinaryOp:
		prec := opPrecedence(node.Op)
		needParens := prec < parentPrec
		if needParens {
			sb.WriteByte('(')
		}
		// Left child: same precedence is fine (left-associative), except for power.
		leftChildPrec := prec
		if node.Op == "^" {
			// Power is right-associative, so the left child needs parens at same precedence.
			leftChildPrec = prec + 1
		}
		writeNode(sb, node.Children[0], leftChildPrec)
		sb.WriteByte(' ')
		sb.WriteString(node.Op)
		sb.WriteByte(' ')
		// Right child: for left-associative ops, the right side needs higher prec to avoid parens.
		rightChildPrec := prec + 1
		if node.Op == "^" {
			// Right-associative: right child at same precedence is fine.
			rightChildPrec = prec
		}
		writeNode(sb, node.Children[1], rightChildPrec)
		if needParens {
			sb.WriteByte(')')
		}

	case KindComparison:
		prec := precComparison
		needParens := prec < parentPrec
		if needParens {
			sb.WriteByte('(')
		}
		writeNode(sb, node.Children[0], prec+1)
		sb.WriteByte(' ')
		sb.WriteString(node.Op)
		sb.WriteByte(' ')
		writeNode(sb, node.Children[1], prec+1)
		if needParens {
			sb.WriteByte(')')
		}

	case KindUnaryOp:
		needParens := precUnary < parentPrec
		if needParens {
			sb.WriteByte('(')
		}
		sb.WriteByte('-')
		writeNode(sb, node.Children[0], precUnary)
		if needParens {
			sb.WriteByte(')')
		}

	case KindFunctionCall:
		sb.WriteString(node.FuncName)
		sb.WriteByte('(')
		for i, child := range node.Children {
			if i > 0 {
				sb.WriteString(", ")
			}
			writeNode(sb, child, precNone)
		}
		sb.WriteByte(')')

	case KindConditional:
		needParens := precConditional < parentPrec
		if needParens {
			sb.WriteByte('(')
		}
		sb.WriteString("if ")
		writeNode(sb, node.Children[0], precConditional+1)
		sb.WriteString(" then ")
		writeNode(sb, node.Children[1], precConditional+1)
		sb.WriteString(" else ")
		writeNode(sb, node.Children[2], precConditional+1)
		if needParens {
			sb.WriteByte(')')
		}
	}
}

func opPrecedence(op string) int {
	switch op {
	case "+", "-":
		return precAddSub
	case "*", "/", "%":
		return precMulDiv
	case "^":
		return precPower
	default:
		return precNone
	}
}
