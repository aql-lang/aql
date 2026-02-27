package ast

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/token"
)

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
