package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType identifies the kind of lexical token.
type TokenType int

const (
	TokenNumber     TokenType = iota // integer or decimal literal
	TokenIdentifier                  // variable or function name
	TokenPlus                        // +
	TokenMinus                       // -
	TokenStar                        // *
	TokenSlash                       // /
	TokenCaret                       // ^
	TokenPercent                     // %
	TokenLParen                      // (
	TokenRParen                      // )
	TokenComma                       // ,
	TokenGT                          // >
	TokenLT                          // <
	TokenGE                          // >=
	TokenLE                          // <=
	TokenEQ                          // ==
	TokenNE                          // !=
	TokenIf                          // keyword if
	TokenThen                        // keyword then
	TokenElse                        // keyword else
	TokenEOF                         // end of input
)

var tokenNames = map[TokenType]string{
	TokenNumber:     "Number",
	TokenIdentifier: "Identifier",
	TokenPlus:       "+",
	TokenMinus:      "-",
	TokenStar:       "*",
	TokenSlash:      "/",
	TokenCaret:      "^",
	TokenPercent:    "%",
	TokenLParen:     "(",
	TokenRParen:     ")",
	TokenComma:      ",",
	TokenGT:         ">",
	TokenLT:         "<",
	TokenGE:         ">=",
	TokenLE:         "<=",
	TokenEQ:         "==",
	TokenNE:         "!=",
	TokenIf:         "if",
	TokenThen:       "then",
	TokenElse:       "else",
	TokenEOF:        "EOF",
}

// Token is a single lexical unit from the formula input.
type Token struct {
	Type TokenType
	Text string
	Pos  int // byte offset in source string
}

// String returns a human-readable representation of the token.
func (t Token) String() string {
	name, ok := tokenNames[t.Type]
	if !ok {
		name = "?"
	}
	if t.Type == TokenNumber || t.Type == TokenIdentifier {
		return fmt.Sprintf("%s(%s)@%d", name, t.Text, t.Pos)
	}
	return fmt.Sprintf("%s@%d", name, t.Pos)
}

// Lexer tokenizes a formula text expression.
type Lexer struct {
	input []rune
	pos   int  // current index into input
	ch    rune // current rune; 0 when past end
}

// NewLexer creates a new Lexer for the given input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: []rune(input)}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.pos]
	}
}

func (l *Lexer) advance() {
	l.pos++
	l.readChar()
}

func (l *Lexer) peek() rune {
	if l.pos+1 >= len(l.input) {
		return 0
	}
	return l.input[l.pos+1]
}

func (l *Lexer) skipWhitespace() {
	for l.ch != 0 && unicode.IsSpace(l.ch) {
		l.advance()
	}
}

// NextToken scans and returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	startPos := l.pos

	if l.ch == 0 {
		return Token{Type: TokenEOF, Pos: startPos}
	}

	// Two-character operators
	switch l.ch {
	case '>':
		if l.peek() == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenGE, Text: ">=", Pos: startPos}
		}
		l.advance()
		return Token{Type: TokenGT, Text: ">", Pos: startPos}
	case '<':
		if l.peek() == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenLE, Text: "<=", Pos: startPos}
		}
		l.advance()
		return Token{Type: TokenLT, Text: "<", Pos: startPos}
	case '=':
		if l.peek() == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenEQ, Text: "==", Pos: startPos}
		}
	case '!':
		if l.peek() == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenNE, Text: "!=", Pos: startPos}
		}
	}

	// Single-character operators
	switch l.ch {
	case '+':
		l.advance()
		return Token{Type: TokenPlus, Text: "+", Pos: startPos}
	case '-':
		l.advance()
		return Token{Type: TokenMinus, Text: "-", Pos: startPos}
	case '*':
		l.advance()
		return Token{Type: TokenStar, Text: "*", Pos: startPos}
	case '/':
		l.advance()
		return Token{Type: TokenSlash, Text: "/", Pos: startPos}
	case '^':
		l.advance()
		return Token{Type: TokenCaret, Text: "^", Pos: startPos}
	case '%':
		l.advance()
		return Token{Type: TokenPercent, Text: "%", Pos: startPos}
	case '(':
		l.advance()
		return Token{Type: TokenLParen, Text: "(", Pos: startPos}
	case ')':
		l.advance()
		return Token{Type: TokenRParen, Text: ")", Pos: startPos}
	case ',':
		l.advance()
		return Token{Type: TokenComma, Text: ",", Pos: startPos}
	}

	// Numbers: integer or decimal
	if unicode.IsDigit(l.ch) || (l.ch == '.' && l.peek() != 0 && unicode.IsDigit(l.peek())) {
		return l.readNumber(startPos)
	}

	// Identifiers and keywords
	if l.ch == '_' || unicode.IsLetter(l.ch) {
		return l.readIdentifier(startPos)
	}

	// Unknown character
	ch := l.ch
	l.advance()
	return Token{Type: TokenEOF, Text: string(ch), Pos: startPos}
}

func (l *Lexer) readNumber(startPos int) Token {
	var sb strings.Builder
	for l.ch != 0 && unicode.IsDigit(l.ch) {
		sb.WriteRune(l.ch)
		l.advance()
	}
	if l.ch == '.' {
		sb.WriteRune('.')
		l.advance()
		for l.ch != 0 && unicode.IsDigit(l.ch) {
			sb.WriteRune(l.ch)
			l.advance()
		}
	}
	return Token{Type: TokenNumber, Text: sb.String(), Pos: startPos}
}

func (l *Lexer) readIdentifier(startPos int) Token {
	var sb strings.Builder
	for l.ch != 0 && (unicode.IsLetter(l.ch) || unicode.IsDigit(l.ch) || l.ch == '_') {
		sb.WriteRune(l.ch)
		l.advance()
	}
	text := sb.String()

	switch strings.ToLower(text) {
	case "if":
		return Token{Type: TokenIf, Text: text, Pos: startPos}
	case "then":
		return Token{Type: TokenThen, Text: text, Pos: startPos}
	case "else":
		return Token{Type: TokenElse, Text: text, Pos: startPos}
	default:
		return Token{Type: TokenIdentifier, Text: text, Pos: startPos}
	}
}
