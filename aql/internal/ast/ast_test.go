package ast

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/token"
)

func TestProgramTokenLiteral(t *testing.T) {
	// Empty program
	p := &Program{}
	if got := p.TokenLiteral(); got != "" {
		t.Errorf("empty program TokenLiteral() = %q, want %q", got, "")
	}

	// Program with statements
	p = &Program{
		Statements: []Statement{
			&ExpressionStatement{
				Token: token.Token{Type: token.IDENT, Literal: "foo"},
			},
		},
	}
	if got := p.TokenLiteral(); got != "foo" {
		t.Errorf("TokenLiteral() = %q, want %q", got, "foo")
	}
}

func TestExpressionStatementTokenLiteral(t *testing.T) {
	es := &ExpressionStatement{
		Token: token.Token{Type: token.IDENT, Literal: "bar"},
	}
	if got := es.TokenLiteral(); got != "bar" {
		t.Errorf("TokenLiteral() = %q, want %q", got, "bar")
	}
	// statementNode is a marker — just call it for coverage
	es.statementNode()
}

func TestExpressionStatementStringNilExpr(t *testing.T) {
	es := &ExpressionStatement{
		Token:      token.Token{Type: token.IDENT, Literal: "x"},
		Expression: nil,
	}
	if got := es.String(); got != "" {
		t.Errorf("String() with nil expression = %q, want %q", got, "")
	}
}

func TestIdentifier(t *testing.T) {
	id := &Identifier{
		Token: token.Token{Type: token.IDENT, Literal: "myvar"},
		Value: "myvar",
	}
	// expressionNode marker
	id.expressionNode()

	if got := id.TokenLiteral(); got != "myvar" {
		t.Errorf("TokenLiteral() = %q, want %q", got, "myvar")
	}
	if got := id.String(); got != "myvar" {
		t.Errorf("String() = %q, want %q", got, "myvar")
	}
}

func TestProgramString(t *testing.T) {
	tests := []struct {
		name     string
		program  *Program
		expected string
	}{
		{
			name:     "empty program",
			program:  &Program{},
			expected: "",
		},
		{
			name: "single identifier expression",
			program: &Program{
				Statements: []Statement{
					&ExpressionStatement{
						Token: token.Token{Type: token.IDENT, Literal: "x"},
						Expression: &Identifier{
							Token: token.Token{Type: token.IDENT, Literal: "x"},
							Value: "x",
						},
					},
				},
			},
			expected: "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.program.String()
			if got != tt.expected {
				t.Errorf("Program.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
