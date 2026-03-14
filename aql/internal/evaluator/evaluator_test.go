package evaluator

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/ast"
	"github.com/metsitaba/voxgig-exp/aql/internal/lexer"
	"github.com/metsitaba/voxgig-exp/aql/internal/object"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
	"github.com/metsitaba/voxgig-exp/aql/internal/token"
)

func testEval(input string) object.Object {
	l := lexer.New(input)
	p := parser.New(l)
	program := p.ParseProgram()
	return Eval(program)
}

func TestEval(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType object.ObjectType
	}{
		{
			name:         "empty input returns null",
			input:        "",
			expectedType: object.NULL_OBJ,
		},
		{
			name:         "identifier expression returns null",
			input:        "foo",
			expectedType: object.NULL_OBJ,
		},
		{
			name:         "multiple statements returns last",
			input:        "foo bar",
			expectedType: object.NULL_OBJ,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testEval(tt.input)
			if result.Type() != tt.expectedType {
				t.Errorf("got type %q, want %q", result.Type(), tt.expectedType)
			}
		})
	}
}

func TestEvalNonProgramNode(t *testing.T) {
	// Passing a non-Program node should return NULL.
	result := Eval(&ast.ExpressionStatement{
		Token: token.Token{Type: token.IDENT, Literal: "x"},
	})
	if result != NULL {
		t.Errorf("expected NULL, got %v", result)
	}
}

func TestEvalProgramWithStatements(t *testing.T) {
	// Directly construct a Program with statements to cover the evalProgram loop.
	program := &ast.Program{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Token: token.Token{Type: token.IDENT, Literal: "a"},
			},
			&ast.ExpressionStatement{
				Token: token.Token{Type: token.IDENT, Literal: "b"},
			},
		},
	}
	result := Eval(program)
	if result != NULL {
		t.Errorf("expected NULL, got %v", result)
	}
}
