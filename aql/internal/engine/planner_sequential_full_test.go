package engine

import (
	"fmt"
	"testing"
)

// TestSequentialPlanner_FullEngineTests runs a broad set of engine integration
// tests with the sequential planner enabled. This reports pass/fail without
// attempting fixes.
func TestSequentialPlanner_FullEngineTests(t *testing.T) {
	tests := []struct {
		name   string
		tokens []Value
		want   string // expected result as string, "" means just check no error
	}{
		// Basic arithmetic
		{"add_forward", toks("add", 2, 3), "5"},
		{"add_infix", toks(2, "add", 3), "5"},
		{"add_prefix", toks(2, 3, "add"), "5"},
		{"mul_forward", toks("mul", 3, 4), "12"},
		{"sub_infix", toks(10, "sub", 3), "7"},

		// def
		{"def_word_int", toks("def", "x", 42, "x"), "42"},
		{"def_string_body", toks("def", "greeting", "hello", "greeting"), "hello"},
		{"def_list_body", toks("def", "pair", list(1, 2), "pair"), "1 2"},

		// undef
		{"undef_basic", toks("def", "a", 1, "undef", "a", "a"), "a"},
		{"undef_stacked", toks("def", "a", 1, "def", "a", 2, "a", "undef", "a", "a"), "2 1"},

		// set/get
		{"set_get_word", toks("set", "k", 99, "get", "k"), "99"},
		{"set_get_string", toks("set", "hello", 7, "get", "hello"), "7"},

		// quote
		{"quote_word", toks("quote", "hello"), "hello"},
		{"quote_int", toks("quote", 42), "42"},

		// dup/swap/drop
		{"dup", toks(5, "dup"), "5 5"},
		{"swap", toks(1, 2, "swap"), "2 1"},
		{"drop", toks(1, 2, "drop"), "1"},

		// Comparisons
		{"lt_true", toks(1, "lt", 2), "true"},
		{"gt_false", toks(1, "gt", 2), "false"},
		{"eq_true", toks(3, "eq", 3), "true"},

		// Nested forward: def foo (add 1 2) foo
		{"nested_forward", toks("def", "bar", paren("add", 1, 2), "bar"), "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			// sequential planner is now the default
			e := NewTop(r)
			out, err := e.Run(tt.tokens)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if tt.want != "" {
				got := resultString(out)
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}

// --- Helpers ---

func toks(items ...any) []Value {
	var vals []Value
	for _, item := range items {
		switch v := item.(type) {
		case int:
			vals = append(vals, NewInteger(int64(v)))
		case int64:
			vals = append(vals, NewInteger(v))
		case string:
			// Check if it's a known word or should be a string
			if isQuoted(v) {
				vals = append(vals, NewString(v[1:len(v)-1]))
			} else {
				vals = append(vals, NewWord(v))
			}
		case Value:
			vals = append(vals, v)
		case []Value:
			vals = append(vals, v...)
		}
	}
	return vals
}

func list(items ...any) Value {
	return NewList(toks(items...))
}

func paren(items ...any) []Value {
	inner := toks(items...)
	result := []Value{NewOpenParen()}
	result = append(result, inner...)
	result = append(result, NewWord(")"))
	return result
}

func isQuoted(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"'
}

func resultString(vals []Value) string {
	s := ""
	for i, v := range vals {
		if i > 0 {
			s += " "
		}
		s += v.String()
	}
	return s
}

// Verify the helper produces correct tokens.
func TestSequentialPlanner_TokenHelper(t *testing.T) {
	tokens := toks("add", 2, 3)
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if !tokens[0].IsWord() {
		t.Error("expected word")
	}
	fmt.Printf("tokens: %v\n", tokens)
}
