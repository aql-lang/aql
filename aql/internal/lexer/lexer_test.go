package lexer

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/token"
)

func TestNextToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token.Token
	}{
		{
			name:  "empty input returns EOF",
			input: "",
			expected: []token.Token{
				{Type: token.EOF, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				got := l.NextToken()
				if got.Type != exp.Type {
					t.Errorf("token[%d] type = %q, want %q", i, got.Type, exp.Type)
				}
				if got.Literal != exp.Literal {
					t.Errorf("token[%d] literal = %q, want %q", i, got.Literal, exp.Literal)
				}
			}
		})
	}
}
