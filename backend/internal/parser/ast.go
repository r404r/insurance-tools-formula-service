package parser

// NodeKind classifies AST nodes by their role in a formula expression.
type NodeKind int

const (
	KindLiteral      NodeKind = iota // numeric constant
	KindVariable                     // named input variable
	KindBinaryOp                     // +, -, *, /, ^, %
	KindUnaryOp                      // - (negation)
	KindFunctionCall                 // round(), min(), max(), abs(), sqrt(), ln(), exp(), lookup(), floor(), ceil()
	KindConditional                  // if/then/else
	KindComparison                   // >, <, >=, <=, ==, !=
)

// ASTNode is a node in the abstract syntax tree produced by the parser.
type ASTNode struct {
	Kind     NodeKind   // what kind of node this is
	Value    string     // for literals: the numeric text; for variables: the name
	Op       string     // for binary/unary/comparison ops: the operator symbol
	FuncName string     // for function calls: the function name
	Children []*ASTNode // operands or arguments
}
