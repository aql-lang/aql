package engine

import (
	"testing"
)

// --- Type system tests ---

func TestTypeMatches(t *testing.T) {
	tests := []struct {
		name    string
		typ     Type
		pattern Type
		want    bool
	}{
		{"exact match", TStringProper, TStringProper, true},
		{"child matches parent", TStringProper, TString, true},
		{"parent does not match child", TString, TStringProper, false},
		{"any matches string", TStringProper, TAny, true},
		{"any matches integer", TInteger, TAny, true},
		{"integer does not match string", TInteger, TString, false},
		{"string/empty matches string", TStringEmpty, TString, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.typ.Matches(tt.pattern)
			if got != tt.want {
				t.Errorf("%s.Matches(%s) = %v, want %v", tt.typ, tt.pattern, got, tt.want)
			}
		})
	}
}

// --- Value constructor tests ---

func TestNewString(t *testing.T) {
	v := NewString("hello")
	if !v.VType.Equal(TStringProper) {
		t.Errorf("type = %s, want string/proper", v.VType)
	}
	if v.AsString() != "hello" {
		t.Errorf("data = %q, want %q", v.AsString(), "hello")
	}

	empty := NewString("")
	if !empty.VType.Equal(TStringEmpty) {
		t.Errorf("empty type = %s, want string/empty", empty.VType)
	}
}

func TestNewInteger(t *testing.T) {
	v := NewInteger(42)
	if !v.VType.Equal(TInteger) {
		t.Errorf("type = %s, want number/integer", v.VType)
	}
	if v.AsInteger() != 42 {
		t.Errorf("data = %d, want 42", v.AsInteger())
	}
}

func TestNewWord(t *testing.T) {
	v := NewWord("upper")
	if !v.IsWord() {
		t.Errorf("IsWord() = false")
	}
	if v.AsWord().Name != "upper" {
		t.Errorf("name = %q, want %q", v.AsWord().Name, "upper")
	}
}

// --- Engine tests: literals ---

func TestLiteralSelfInsert(t *testing.T) {
	e := New(DefaultRegistry())

	tests := []struct {
		name  string
		input []Value
		want  string // expected string representation of the single result
	}{
		{"integer", []Value{NewInteger(42)}, "42"},
		{"string", []Value{NewString("hello")}, "'hello'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1", len(result))
			}
			if result[0].String() != tt.want {
				t.Errorf("got %s, want %s", result[0].String(), tt.want)
			}
		})
	}
}

// --- Engine tests: prefix functions ---

func TestPrefixUpper(t *testing.T) {
	// a upper -> 'A'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("a"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "A" {
		t.Errorf("got %q, want %q", result[0].AsString(), "A")
	}
}

func TestPrefixLower(t *testing.T) {
	// C lower -> 'c'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("C"), NewWord("lower")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "c" {
		t.Errorf("got %q, want %q", result[0].AsString(), "c")
	}
}

// --- Engine tests: suffix (forward) functions ---

func TestSuffixLower(t *testing.T) {
	// lower B -> 'b'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewWord("lower"), NewString("B")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "b" {
		t.Errorf("got %q, want %q", result[0].AsString(), "b")
	}
}

// --- Engine tests: signature error ---

func TestSignatureError(t *testing.T) {
	// 99 lower -> signature error (integer doesn't match string)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewInteger(99), NewWord("lower")})
	if err == nil {
		t.Fatal("expected signature error, got nil")
	}
}

// --- Engine tests: forth primitives ---

func TestDup(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewInteger(1), NewWord("dup")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsInteger() != 1 || result[1].AsInteger() != 1 {
		t.Errorf("got [%v, %v], want [1, 1]", result[0], result[1])
	}
}

func TestSwap(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("swap")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsInteger() != 2 || result[1].AsInteger() != 1 {
		t.Errorf("got [%v, %v], want [2, 1]", result[0], result[1])
	}
}

func TestDrop(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewInteger(1), NewWord("drop")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

// --- Engine tests: modifier forcing ---

func TestForceSuffix(t *testing.T) {
	// lower= E -> 'e' (force suffix even though prefix exists)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWordModified("lower", -1, false, true),
		NewString("E"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "e" {
		t.Errorf("got %q, want %q", result[0].AsString(), "e")
	}
}

func TestForcePrefix(t *testing.T) {
	// F =lower -> 'f' (force prefix, no suffix considered)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("F"),
		NewWordModified("lower", -1, true, false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "f" {
		t.Errorf("got %q, want %q", result[0].AsString(), "f")
	}
}

func TestArgCountSuffix(t *testing.T) {
	// lower/1 D -> 'd' (arg count 1 picks the suffix signature)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWordModified("lower", 1, false, true),
		NewString("D"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "d" {
		t.Errorf("got %q, want %q", result[0].AsString(), "d")
	}
}

// --- Engine tests: unknown word ---

func TestUnknownWordBecomesString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewWord("foo")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	if result[0].AsString() != "foo" {
		t.Errorf("got %q, want %q", result[0].AsString(), "foo")
	}
}

// --- Engine tests: multiple operations ---

func TestChainedOps(t *testing.T) {
	// a upper dup -> ['A', 'A']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("a"),
		NewWord("upper"),
		NewWord("dup"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsString() != "A" || result[1].AsString() != "A" {
		t.Errorf("got [%v, %v], want ['A', 'A']", result[0], result[1])
	}
}
