package parser

import (
	"fmt"
)

// precedence levels for operator-precedence (Pratt) parsing
const (
	precNone       = 0
	precConditional = 1 // if/then/else
	precComparison = 2  // >, <, >=, <=, ==, !=
	precAddSub     = 3  // + -
	precMulDiv     = 4  // * / %
	precPower      = 5  // ^ (right-associative)
	precUnary      = 6  // unary -
	precCall       = 7  // function calls, grouped expressions
)

// Parser implements a Pratt (operator-precedence) recursive descent parser
// for insurance formula text expressions.
type Parser struct {
	lexer *Lexer
	cur   Token // current token
	peek  Token // lookahead token
}

// NewParser creates a new Parser for the given formula text.
func NewParser(input string) *Parser {
	p := &Parser{lexer: NewLexer(input)}
	// Prime both cur and peek.
	p.cur = p.lexer.NextToken()
	p.peek = p.lexer.NextToken()
	return p
}

func (p *Parser) advance() {
	p.cur = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) expect(tt TokenType) error {
	if p.cur.Type != tt {
		return p.errorf("expected %s but got %s", tokenNames[tt], p.cur)
	}
	p.advance()
	return nil
}

func (p *Parser) errorf(format string, args ...interface{}) error {
	prefix := fmt.Sprintf("parse error at position %d: ", p.cur.Pos)
	return fmt.Errorf(prefix+format, args...)
}

// Parse parses the complete input and returns the root AST node.
func (p *Parser) Parse() (*ASTNode, error) {
	node, err := p.parseExpression(precNone)
	if err != nil {
		return nil, err
	}
	if p.cur.Type != TokenEOF {
		return nil, p.errorf("unexpected token %s after expression", p.cur)
	}
	return node, nil
}

// parseExpression is the core Pratt parsing loop. It parses everything at
// the given minimum precedence level or higher.
func (p *Parser) parseExpression(minPrec int) (*ASTNode, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}

	for {
		prec := infixPrecedence(p.cur.Type)
		if prec <= minPrec {
			break
		}
		left, err = p.parseInfix(left, prec)
		if err != nil {
			return nil, err
		}
	}

	return left, nil
}

// parsePrefix handles tokens that appear at the start of an expression:
// numbers, variables, function calls, parenthesized groups, unary minus, if.
func (p *Parser) parsePrefix() (*ASTNode, error) {
	switch p.cur.Type {
	case TokenNumber:
		return p.parseNumber()
	case TokenIdentifier:
		return p.parseIdentifierOrCall()
	case TokenLParen:
		return p.parseGrouped()
	case TokenMinus:
		return p.parseUnaryMinus()
	case TokenIf:
		return p.parseConditional()
	default:
		return nil, p.errorf("unexpected token %s", p.cur)
	}
}

func (p *Parser) parseNumber() (*ASTNode, error) {
	node := &ASTNode{Kind: KindLiteral, Value: p.cur.Text}
	p.advance()
	return node, nil
}

func (p *Parser) parseIdentifierOrCall() (*ASTNode, error) {
	name := p.cur.Text
	p.advance()

	// If followed by '(' this is a function call.
	if p.cur.Type == TokenLParen {
		return p.parseFunctionCall(name)
	}

	return &ASTNode{Kind: KindVariable, Value: name}, nil
}

func (p *Parser) parseFunctionCall(name string) (*ASTNode, error) {
	// consume '('
	p.advance()

	node := &ASTNode{Kind: KindFunctionCall, FuncName: name}

	// Handle empty argument list.
	if p.cur.Type == TokenRParen {
		p.advance()
		return node, nil
	}

	for {
		arg, err := p.parseExpression(precNone)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, arg)

		if p.cur.Type != TokenComma {
			break
		}
		p.advance() // consume ','
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return node, nil
}

func (p *Parser) parseGrouped() (*ASTNode, error) {
	p.advance() // consume '('
	node, err := p.parseExpression(precNone)
	if err != nil {
		return nil, err
	}
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return node, nil
}

func (p *Parser) parseUnaryMinus() (*ASTNode, error) {
	p.advance() // consume '-'
	operand, err := p.parseExpression(precUnary)
	if err != nil {
		return nil, err
	}
	return &ASTNode{Kind: KindUnaryOp, Op: "-", Children: []*ASTNode{operand}}, nil
}

// parseConditional parses: if <cond> then <consequent> else <alternate>
func (p *Parser) parseConditional() (*ASTNode, error) {
	p.advance() // consume 'if'

	cond, err := p.parseExpression(precConditional)
	if err != nil {
		return nil, err
	}

	if p.cur.Type != TokenThen {
		return nil, p.errorf("expected 'then' but got %s", p.cur)
	}
	p.advance()

	consequent, err := p.parseExpression(precConditional)
	if err != nil {
		return nil, err
	}

	if p.cur.Type != TokenElse {
		return nil, p.errorf("expected 'else' but got %s", p.cur)
	}
	p.advance()

	alternate, err := p.parseExpression(precConditional)
	if err != nil {
		return nil, err
	}

	return &ASTNode{
		Kind:     KindConditional,
		Children: []*ASTNode{cond, consequent, alternate},
	}, nil
}

// parseInfix handles binary operators and comparison operators.
func (p *Parser) parseInfix(left *ASTNode, prec int) (*ASTNode, error) {
	tok := p.cur
	p.advance()

	// Power is right-associative: use prec-1 so that the right side binds tighter.
	rightPrec := prec
	if tok.Type == TokenCaret {
		rightPrec = prec - 1
	}

	right, err := p.parseExpression(rightPrec)
	if err != nil {
		return nil, err
	}

	if isComparisonToken(tok.Type) {
		return &ASTNode{
			Kind:     KindComparison,
			Op:       tok.Text,
			Children: []*ASTNode{left, right},
		}, nil
	}

	return &ASTNode{
		Kind:     KindBinaryOp,
		Op:       tok.Text,
		Children: []*ASTNode{left, right},
	}, nil
}

// infixPrecedence returns the binding power for an infix operator token,
// or 0 if the token is not an infix operator.
func infixPrecedence(tt TokenType) int {
	switch tt {
	case TokenPlus, TokenMinus:
		return precAddSub
	case TokenStar, TokenSlash, TokenPercent:
		return precMulDiv
	case TokenCaret:
		return precPower
	case TokenGT, TokenLT, TokenGE, TokenLE, TokenEQ, TokenNE:
		return precComparison
	default:
		return precNone
	}
}

func isComparisonToken(tt TokenType) bool {
	switch tt {
	case TokenGT, TokenLT, TokenGE, TokenLE, TokenEQ, TokenNE:
		return true
	default:
		return false
	}
}
