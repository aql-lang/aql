package lexer

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/lang/internal/token"
)

func TestTokenize(t *testing.T) {
	l := New("hello")
	tokens := l.Tokenize()
	// Since NextToken is a stub returning EOF, we should get exactly 1 EOF token
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != token.EOF {
		t.Errorf("expected EOF token, got %s", tokens[0].Type)
	}
}

func TestTokenizeEmpty(t *testing.T) {
	l := New("")
	tokens := l.Tokenize()
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != token.EOF {
		t.Errorf("expected EOF token, got %s", tokens[0].Type)
	}
}

func TestLexerReadChar(t *testing.T) {
	l := New("ab")
	if l.ch != 'a' {
		t.Errorf("expected 'a', got %c", l.ch)
	}
	l.readChar()
	if l.ch != 'b' {
		t.Errorf("expected 'b', got %c", l.ch)
	}
	l.readChar()
	if l.ch != 0 {
		t.Errorf("expected 0 (EOF), got %c", l.ch)
	}
}

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
