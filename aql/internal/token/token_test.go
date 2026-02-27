package token

import "testing"

func TestLookupIdent(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
	}{
		{"let", LET},
		{"fn", FUNCTION},
		{"true", TRUE},
		{"false", FALSE},
		{"if", IF},
		{"else", ELSE},
		{"return", RETURN},
		{"foobar", IDENT},
		{"x", IDENT},
	}

	for _, tt := range tests {
		got := LookupIdent(tt.input)
		if got != tt.expected {
			t.Errorf("LookupIdent(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
