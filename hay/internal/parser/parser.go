package parser

import (
	"github.com/metsitaba/voxgig-exp/hay/internal/ast"
	"github.com/metsitaba/voxgig-exp/hay/internal/lexer"
	"github.com/metsitaba/voxgig-exp/hay/internal/token"
)

// Parser produces an AST from a sequence of tokens.
type Parser struct {
	l         *lexer.Lexer
	curToken  token.Token
	peekToken token.Token
	errors    []string
}

// New creates a new Parser for the given Lexer.
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}
	// Read two tokens so curToken and peekToken are both set.
	p.nextToken()
	p.nextToken()
	return p
}

// Errors returns the list of parse errors.
func (p *Parser) Errors() []string {
	return p.errors
}

// ParseProgram parses the entire token stream and returns a Program AST.
// Stub: returns an empty program.
func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}
	return program
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}
