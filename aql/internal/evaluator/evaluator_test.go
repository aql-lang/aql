package evaluator

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/lexer"
	"github.com/metsitaba/voxgig-exp/aql/internal/object"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
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
