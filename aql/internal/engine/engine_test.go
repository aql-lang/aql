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

// --- Engine tests: suffix Forth primitives ---

func TestDupSuffix(t *testing.T) {
	// dup/s 1 → [1, 1]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWordModified("dup", -1, false, true),
		NewInteger(1),
	})
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

func TestSwapSuffix(t *testing.T) {
	// swap/s 1 2 → [2, 1]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWordModified("swap", -1, false, true),
		NewInteger(1), NewInteger(2),
	})
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

func TestSwapInfix(t *testing.T) {
	// 1 swap 2 → error (swap is prefix-only in the new model)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewInteger(1), NewWord("swap"), NewInteger(2),
	})
	if err == nil {
		t.Fatal("expected error for swap infix (swap is prefix-only), got nil")
	}
}

func TestDropSuffix(t *testing.T) {
	// drop/s 1 → []
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWordModified("drop", -1, false, true),
		NewInteger(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

// --- Engine tests: modifier forcing ---

func TestForceSuffix(t *testing.T) {
	// lower/s E -> 'e' (force suffix even though prefix exists)
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
	// F lower/p -> 'f' (force prefix, no suffix considered)
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
		NewWordModified("lower", 1, false, false),
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

// --- Engine tests: arithmetic (prefix / Forth-style) ---

func TestArithmeticPrefix(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 1 2 add → 3
		{"add", []Value{NewInteger(1), NewInteger(2), NewWord("add")}, 3},
		// 10 3 sub → 7
		{"sub", []Value{NewInteger(10), NewInteger(3), NewWord("sub")}, 7},
		// 4 5 mul → 20
		{"mul", []Value{NewInteger(4), NewInteger(5), NewWord("mul")}, 20},
		// 10 3 div → 3
		{"div", []Value{NewInteger(10), NewInteger(3), NewWord("div")}, 3},
		// 10 3 mod → 1
		{"mod", []Value{NewInteger(10), NewInteger(3), NewWord("mod")}, 1},
		// negative: 3 10 sub → -7
		{"sub negative", []Value{NewInteger(3), NewInteger(10), NewWord("sub")}, -7},
		// zero: 0 5 add → 5
		{"add zero", []Value{NewInteger(0), NewInteger(5), NewWord("add")}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

// --- Engine tests: arithmetic (infix via forward mechanism) ---

func TestArithmeticInfix(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 1 add 2 → 3
		{"add", []Value{NewInteger(1), NewWord("add"), NewInteger(2)}, 3},
		// 10 sub 3 → 7
		{"sub", []Value{NewInteger(10), NewWord("sub"), NewInteger(3)}, 7},
		// 4 mul 5 → 20
		{"mul", []Value{NewInteger(4), NewWord("mul"), NewInteger(5)}, 20},
		// 10 div 3 → 3
		{"div", []Value{NewInteger(10), NewWord("div"), NewInteger(3)}, 3},
		// 10 mod 3 → 1
		{"mod", []Value{NewInteger(10), NewWord("mod"), NewInteger(3)}, 1},
		// 1 sub 2 → -1
		{"sub negative", []Value{NewInteger(1), NewWord("sub"), NewInteger(2)}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

// --- Engine tests: arithmetic errors ---

func TestArithmeticErrors(t *testing.T) {
	e := New(DefaultRegistry())

	tests := []struct {
		name  string
		input []Value
	}{
		// division by zero
		{"div by zero prefix", []Value{NewInteger(10), NewInteger(0), NewWord("div")}},
		{"div by zero infix", []Value{NewInteger(10), NewWord("div"), NewInteger(0)}},
		// modulo by zero
		{"mod by zero prefix", []Value{NewInteger(10), NewInteger(0), NewWord("mod")}},
		{"mod by zero infix", []Value{NewInteger(10), NewWord("mod"), NewInteger(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.Run(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// --- Engine tests: arithmetic chaining ---

func TestArithmeticChaining(t *testing.T) {
	e := New(DefaultRegistry())

	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 1 2 add 3 add → 6 (prefix then infix)
		{"prefix then infix", []Value{
			NewInteger(1), NewInteger(2), NewWord("add"),
			NewWord("add"), NewInteger(3),
		}, 6},
		// 2 3 mul 4 add → 10 (prefix mul, then infix add)
		{"mul then add", []Value{
			NewInteger(2), NewInteger(3), NewWord("mul"),
			NewWord("add"), NewInteger(4),
		}, 10},
		// 10 sub 3 → 7, then dup → [7, 7], then mul → 49
		{"infix sub, dup, prefix mul", []Value{
			NewInteger(10), NewWord("sub"), NewInteger(3),
			NewWord("dup"),
			NewWord("mul"),
		}, 49},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

// --- Engine tests: operator precedence ---

func TestPrecedenceMulBeforeAdd(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 2 add 3 mul 4 → 2+(3*4) = 14
		{"add then mul", []Value{
			NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
		}, 14},
		// 2 mul 3 add 4 → (2*3)+4 = 10
		{"mul then add", []Value{
			NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("add"), NewInteger(4),
		}, 10},
		// 1 add 2 mul 3 add 4 → 1+(2*3)+4 = 11
		{"add mul add", []Value{
			NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
			NewWord("add"), NewInteger(4),
		}, 11},
		// 2 add 3 mul 4 mul 5 → 2+(3*4*5) = 62
		{"add mul mul", []Value{
			NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
			NewWord("mul"), NewInteger(5),
		}, 62},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestPrecedenceSameLevel(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 10 sub 3 sub 1 → (10-3)-1 = 6 (left-to-right)
		{"sub sub", []Value{
			NewInteger(10), NewWord("sub"), NewInteger(3), NewWord("sub"), NewInteger(1),
		}, 6},
		// 2 mul 6 div 3 → (2*6)/3 = 4 (left-to-right)
		{"mul div", []Value{
			NewInteger(2), NewWord("mul"), NewInteger(6), NewWord("div"), NewInteger(3),
		}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestPrecedencePrefixUnaffected(t *testing.T) {
	// Prefix (Forth-style) should still work: 2 3 mul 4 add → 10
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(2), NewInteger(3), NewWord("mul"),
		NewWord("add"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 10 {
		t.Errorf("got %d, want 10", result[0].AsInteger())
	}
}

// --- Engine tests: storage (set/get) ---

func TestSetGetSuffix(t *testing.T) {
	// set foo 99 end get foo → [99]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("end"),
		NewWord("get"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 99 {
		t.Errorf("got %d, want 99", result[0].AsInteger())
	}
}

func TestSetGetWithoutEnd(t *testing.T) {
	// set foo 99 get foo → [99] (end is optional)
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("get"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 99 {
		t.Errorf("got %d, want 99", result[0].AsInteger())
	}
}

func TestSetGetPrefix(t *testing.T) {
	// "foo" 99 set → stores foo=99, then "foo" get → [99]
	reg := DefaultRegistry()
	e := New(reg)
	_, err := e.Run([]Value{
		NewString("bar"), NewInteger(42), NewWord("set"),
	})
	if err != nil {
		t.Fatalf("unexpected error on set: %v", err)
	}
	result, err := e.Run([]Value{
		NewString("bar"), NewWord("get"),
	})
	if err != nil {
		t.Fatalf("unexpected error on get: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("got %d, want 42", result[0].AsInteger())
	}
}

func TestSetGetString(t *testing.T) {
	// set name hello end get name → ['hello']
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("name"), NewString("hello"),
		NewWord("end"),
		NewWord("get"), NewWord("name"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "hello" {
		t.Errorf("got %q, want %q", result[0].AsString(), "hello")
	}
}

func TestSetOverwrite(t *testing.T) {
	// set x 1 end set x 2 end get x → [2]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("x"), NewInteger(1),
		NewWord("end"),
		NewWord("set"), NewWord("x"), NewInteger(2),
		NewWord("end"),
		NewWord("get"), NewWord("x"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 2 {
		t.Errorf("got %d, want 2", result[0].AsInteger())
	}
}

func TestGetUnknownKey(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("get"), NewWord("missing")})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestEndNoOp(t *testing.T) {
	// 42 end → [42] (end just removes itself)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewInteger(42), NewWord("end")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("got %d, want 42", result[0].AsInteger())
	}
}

func TestEndMultiple(t *testing.T) {
	// 1 end 2 end 3 → [1, 2, 3]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("end"),
		NewInteger(2), NewWord("end"),
		NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, want := range []int64{1, 2, 3} {
		if result[i].AsInteger() != want {
			t.Errorf("result[%d] = %d, want %d", i, result[i].AsInteger(), want)
		}
	}
}

func TestEndTerminatesForward(t *testing.T) {
	// 99 set foo end 88 → stores foo=99, result=[88]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(99), NewWord("set"), NewWord("foo"), NewWord("end"), NewInteger(88),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 88 {
		t.Errorf("got %d, want 88", result[0].AsInteger())
	}
	// Verify the stored value
	val, ok := reg.Store["foo"]
	if !ok {
		t.Fatal("expected store key 'foo' to exist")
	}
	if val.AsInteger() != 99 {
		t.Errorf("store['foo'] = %d, want 99", val.AsInteger())
	}
}

func TestEndTerminatesForwardNoRemainder(t *testing.T) {
	// 99 set foo end → stores foo=99, result=[]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(99), NewWord("set"), NewWord("foo"), NewWord("end"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
	val, ok := reg.Store["foo"]
	if !ok {
		t.Fatal("expected store key 'foo' to exist")
	}
	if val.AsInteger() != 99 {
		t.Errorf("store['foo'] = %d, want 99", val.AsInteger())
	}
}

func TestEndInsufficientArgs(t *testing.T) {
	// set foo end → forward expects 2, collected 1, no prefix → error
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewWord("set"), NewWord("foo"), NewWord("end"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSetGetStorePersistsAcrossRuns(t *testing.T) {
	// Store persists across multiple Run calls on the same registry
	reg := DefaultRegistry()
	e := New(reg)
	_, err := e.Run([]Value{
		NewWord("set"), NewWord("key"), NewInteger(100),
	})
	if err != nil {
		t.Fatalf("unexpected error on set: %v", err)
	}
	result, err := e.Run([]Value{
		NewWord("get"), NewWord("key"),
	})
	if err != nil {
		t.Fatalf("unexpected error on get: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 100 {
		t.Errorf("got %v, want [100]", result)
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

// --- Engine tests: parentheses ---

func TestParenSimpleArithmetic(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// 1 mul (2 add 3) → 1*(2+3) = 5
		{"mul paren add", []Value{
			NewInteger(1), NewWord("mul"),
			NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
		}, 5},
		// (2 add 3) → 5
		{"just paren", []Value{
			NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
		}, 5},
		// (2 mul 3) add 4 → 6+4 = 10
		{"paren mul then add", []Value{
			NewWord("("), NewInteger(2), NewWord("mul"), NewInteger(3), NewWord(")"),
			NewWord("add"), NewInteger(4),
		}, 10},
		// 2 mul (3 add 4) → 2*7 = 14
		{"mul paren add 2", []Value{
			NewInteger(2), NewWord("mul"),
			NewWord("("), NewInteger(3), NewWord("add"), NewInteger(4), NewWord(")"),
		}, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestParenWithSet(t *testing.T) {
	// set foo (1 add 2) end get foo → [3]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("foo"),
		NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
		NewWord("end"),
		NewWord("get"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 3 {
		t.Errorf("got %d, want 3", result[0].AsInteger())
	}
}

func TestParenNested(t *testing.T) {
	// (1 add (2 mul 3)) → 1+6 = 7
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("),
		NewInteger(1), NewWord("add"),
		NewWord("("), NewInteger(2), NewWord("mul"), NewInteger(3), NewWord(")"),
		NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 7 {
		t.Errorf("got %d, want 7", result[0].AsInteger())
	}
}

func TestParenLiteral(t *testing.T) {
	// (42) → [42]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(42), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("got %d, want 42", result[0].AsInteger())
	}
}

func TestParenUnmatchedOpen(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewWord("("), NewInteger(1),
	})
	if err == nil {
		t.Fatal("expected error for unmatched open paren, got nil")
	}
}

func TestParenUnmatchedClose(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewInteger(1), NewWord(")"),
	})
	if err == nil {
		t.Fatal("expected error for unmatched close paren, got nil")
	}
}

func TestParenWithPrecedence(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// (1 add 2) mul 3 → 3*3 = 9 (parens override precedence)
		{"paren overrides precedence", []Value{
			NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
			NewWord("mul"), NewInteger(3),
		}, 9},
		// 3 mul (1 add 2) → 3*3 = 9
		{"mul paren overrides", []Value{
			NewInteger(3), NewWord("mul"),
			NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
		}, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

// ==========================================================================
// Edge case tests — exhaustive coverage of all language elements
// ==========================================================================

// --- Edge: type system ---

func TestEdgeTypeAnyDoesNotMatchWord(t *testing.T) {
	if TWord.Matches(TAny) {
		t.Error("word should not match any")
	}
}

func TestEdgeTypeAnyDoesNotMatchForward(t *testing.T) {
	if TForward.Matches(TAny) {
		t.Error("forward should not match any")
	}
}

func TestEdgeTypeAnyDoesNotMatchOpenParen(t *testing.T) {
	if TOpenParen.Matches(TAny) {
		t.Error("paren/open should not match any")
	}
}

func TestEdgeTypeWordMatchesItself(t *testing.T) {
	if !TWord.Matches(TWord) {
		t.Error("word should match word")
	}
}

func TestEdgeTypeForwardMatchesItself(t *testing.T) {
	if !TForward.Matches(TForward) {
		t.Error("forward should match forward")
	}
}

func TestEdgeTypeOpenParenMatchesParen(t *testing.T) {
	// paren/open should match pattern "paren"
	tParen := NewType("paren")
	if !TOpenParen.Matches(tParen) {
		t.Error("paren/open should match paren")
	}
}

func TestEdgeTypeEmptyStringMatchesAny(t *testing.T) {
	if !TStringEmpty.Matches(TAny) {
		t.Error("string/empty should match any")
	}
}

func TestEdgeTypeUnrelatedTypes(t *testing.T) {
	tFoo := NewType("foo/bar")
	tBaz := NewType("baz")
	if tFoo.Matches(tBaz) {
		t.Error("foo/bar should not match baz")
	}
}

func TestEdgeTypeDeeplyNested(t *testing.T) {
	tDeep := NewType("a/b/c/d")
	tShallow := NewType("a/b")
	if !tDeep.Matches(tShallow) {
		t.Error("a/b/c/d should match a/b")
	}
	if tShallow.Matches(tDeep) {
		t.Error("a/b should not match a/b/c/d")
	}
}

func TestEdgeTypeSelfMatch(t *testing.T) {
	types := []Type{TAny, TString, TStringProper, TStringEmpty, TInteger}
	for _, typ := range types {
		if !typ.Matches(typ) {
			t.Errorf("%s should match itself", typ)
		}
	}
}

// --- Edge: value constructors ---

func TestEdgeNewIntegerZero(t *testing.T) {
	v := NewInteger(0)
	if v.AsInteger() != 0 {
		t.Errorf("got %d, want 0", v.AsInteger())
	}
}

func TestEdgeNewIntegerNegative(t *testing.T) {
	v := NewInteger(-999)
	if v.AsInteger() != -999 {
		t.Errorf("got %d, want -999", v.AsInteger())
	}
}

func TestEdgeNewIntegerMaxMin(t *testing.T) {
	vMax := NewInteger(9223372036854775807) // max int64
	if vMax.AsInteger() != 9223372036854775807 {
		t.Errorf("got %d, want max int64", vMax.AsInteger())
	}
	vMin := NewInteger(-9223372036854775808) // min int64
	if vMin.AsInteger() != -9223372036854775808 {
		t.Errorf("got %d, want min int64", vMin.AsInteger())
	}
}

func TestEdgeNewStringSpecialChars(t *testing.T) {
	v := NewString("hello\nworld\ttab")
	if v.AsString() != "hello\nworld\ttab" {
		t.Errorf("got %q, want string with newline and tab", v.AsString())
	}
}

func TestEdgeNewOpenParen(t *testing.T) {
	v := NewOpenParen()
	if !v.IsOpenParen() {
		t.Error("IsOpenParen() = false for NewOpenParen()")
	}
	if v.IsWord() {
		t.Error("IsWord() = true for open paren")
	}
	if v.IsForward() {
		t.Error("IsForward() = true for open paren")
	}
	if v.String() != "(" {
		t.Errorf("String() = %q, want '('", v.String())
	}
}

func TestEdgeValueStringRepresentations(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"word", NewWord("test"), "word(test)"},
		{"integer", NewInteger(42), "42"},
		{"string", NewString("hi"), "'hi'"},
		{"empty string", NewString(""), "''"},
		{"open paren", NewOpenParen(), "("},
		{"forward", NewForward(ForwardInfo{FuncName: "add", ExpectedArgs: 1, CollectedArgs: 0}), "forward(add,0/1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Edge: empty input ---

func TestEdgeEmptyInput(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

// --- Edge: multiple literals ---

func TestEdgeMultipleLiterals(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3),
		NewString("a"), NewString("b"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 5 {
		t.Fatalf("got %d values, want 5: %v", len(result), result)
	}
}

// --- Edge: unknown words ---

func TestEdgeMultipleUnknownWords(t *testing.T) {
	// foo bar baz → ['foo', 'bar', 'baz']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("foo"), NewWord("bar"), NewWord("baz"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, want := range []string{"foo", "bar", "baz"} {
		if result[i].AsString() != want {
			t.Errorf("result[%d] = %q, want %q", i, result[i].AsString(), want)
		}
	}
}

func TestEdgeUnknownWordCollectedByForward(t *testing.T) {
	// lower foo → 'foo' (foo becomes string, then lower collects it)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewWord("lower"), NewWord("foo")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsString() != "foo" {
		t.Errorf("got %q, want %q", result[0].AsString(), "foo")
	}
}

func TestEdgeUnknownWordAsSetKey(t *testing.T) {
	// set mykey 42 end get mykey → [42]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("mykey"), NewInteger(42),
		NewWord("end"),
		NewWord("get"), NewWord("mykey"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Edge: upper ---

func TestEdgeUpperAlreadyUpper(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("ABC"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsString() != "ABC" {
		t.Errorf("got %q, want %q", result[0].AsString(), "ABC")
	}
}

func TestEdgeUpperEmptyString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString(""), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsString() != "" {
		t.Errorf("got %q, want empty", result[0].AsString())
	}
}

func TestEdgeUpperOnInteger(t *testing.T) {
	// 42 upper → signature error (integer doesn't match string)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewInteger(42), NewWord("upper")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Edge: lower ---

func TestEdgeLowerAlreadyLower(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("abc"), NewWord("lower")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsString() != "abc" {
		t.Errorf("got %q, want %q", result[0].AsString(), "abc")
	}
}

func TestEdgeLowerSuffixOnInteger(t *testing.T) {
	// lower 42 → signature error (forward can't collect integer for string param)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("lower"), NewInteger(42)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Edge: dup ---

func TestEdgeDupString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("hello"), NewWord("dup")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2", len(result))
	}
	if result[0].AsString() != "hello" || result[1].AsString() != "hello" {
		t.Errorf("got [%v, %v], want ['hello', 'hello']", result[0], result[1])
	}
}

func TestEdgeDupNoArgs(t *testing.T) {
	// dup with nothing on stack → error
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("dup")})
	if err == nil {
		t.Fatal("expected error for dup with no args, got nil")
	}
}

// --- Edge: swap ---

func TestEdgeSwapMixedTypes(t *testing.T) {
	// "hello" 42 swap → [42, 'hello']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("hello"), NewInteger(42), NewWord("swap")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2", len(result))
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("result[0] = %v, want 42", result[0])
	}
	if result[1].AsString() != "hello" {
		t.Errorf("result[1] = %v, want 'hello'", result[1])
	}
}

func TestEdgeSwapNoArgs(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("swap")})
	if err == nil {
		t.Fatal("expected error for swap with no args, got nil")
	}
}

func TestEdgeSwapOneArg(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewInteger(1), NewWord("swap")})
	if err == nil {
		t.Fatal("expected error for swap with one arg, got nil")
	}
}

// --- Edge: drop ---

func TestEdgeDropNoArgs(t *testing.T) {
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("drop")})
	if err == nil {
		t.Fatal("expected error for drop with no args, got nil")
	}
}

func TestEdgeDropString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("gone"), NewWord("drop")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeDropPreservesOthers(t *testing.T) {
	// 1 2 3 drop → [1, 2]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3), NewWord("drop"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsInteger() != 1 || result[1].AsInteger() != 2 {
		t.Errorf("got [%v, %v], want [1, 2]", result[0], result[1])
	}
}

// --- Edge: arithmetic boundary conditions ---

func TestEdgeArithmeticLargeNumbers(t *testing.T) {
	e := New(DefaultRegistry())
	// 1000000 mul 1000000 → 1000000000000
	result, err := e.Run([]Value{
		NewInteger(1000000), NewWord("mul"), NewInteger(1000000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 1000000000000 {
		t.Errorf("got %d, want 1000000000000", result[0].AsInteger())
	}
}

func TestEdgeArithmeticNegativeResults(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// -5 mul 3 → -15
		{"neg mul pos", []Value{NewInteger(-5), NewWord("mul"), NewInteger(3)}, -15},
		// -5 mul -3 → 15
		{"neg mul neg", []Value{NewInteger(-5), NewWord("mul"), NewInteger(-3)}, 15},
		// -10 div 3 → -3 (truncated)
		{"neg div pos", []Value{NewInteger(-10), NewWord("div"), NewInteger(3)}, -3},
		// -10 mod 3 → -1
		{"neg mod pos", []Value{NewInteger(-10), NewWord("mod"), NewInteger(3)}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestEdgeArithmeticZeroOperations(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		{"zero add zero", []Value{NewInteger(0), NewWord("add"), NewInteger(0)}, 0},
		{"zero sub zero", []Value{NewInteger(0), NewWord("sub"), NewInteger(0)}, 0},
		{"zero mul anything", []Value{NewInteger(0), NewWord("mul"), NewInteger(999)}, 0},
		{"anything mul zero", []Value{NewInteger(999), NewWord("mul"), NewInteger(0)}, 0},
		{"zero div anything", []Value{NewInteger(0), NewWord("div"), NewInteger(5)}, 0},
		{"zero mod anything", []Value{NewInteger(0), NewWord("mod"), NewInteger(5)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestEdgeArithmeticIdentity(t *testing.T) {
	e := New(DefaultRegistry())
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		{"add identity", []Value{NewInteger(42), NewWord("add"), NewInteger(0)}, 42},
		{"sub identity", []Value{NewInteger(42), NewWord("sub"), NewInteger(0)}, 42},
		{"mul identity", []Value{NewInteger(42), NewWord("mul"), NewInteger(1)}, 42},
		{"div identity", []Value{NewInteger(42), NewWord("div"), NewInteger(1)}, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result[0].AsInteger() != tt.want {
				t.Errorf("got %d, want %d", result[0].AsInteger(), tt.want)
			}
		})
	}
}

func TestEdgeArithmeticNoArgs(t *testing.T) {
	// add with no args → error
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord("add")})
	if err == nil {
		t.Fatal("expected error for add with no args, got nil")
	}
}

func TestEdgeArithmeticOneArg(t *testing.T) {
	// 1 add → should use suffix signature and wait for arg
	// Since there's no next arg, it should be an orphaned forward error
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewInteger(1), NewWord("add")})
	if err == nil {
		t.Fatal("expected error for add with one arg and no suffix arg, got nil")
	}
}

func TestEdgeArithmeticStringOperands(t *testing.T) {
	// "hello" add "world" → "helloworld" (string concatenation)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("hello"), NewWord("add"), NewString("world")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "helloworld" {
		t.Fatalf("got %v, want 'helloworld'", result)
	}
}

// --- Edge: long arithmetic chains ---

func TestEdgeLongInfixChain(t *testing.T) {
	// 1 add 2 add 3 add 4 add 5 → 15 (left-to-right)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2),
		NewWord("add"), NewInteger(3),
		NewWord("add"), NewInteger(4),
		NewWord("add"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 15 {
		t.Errorf("got %d, want 15", result[0].AsInteger())
	}
}

func TestEdgeLongMixedPrecedence(t *testing.T) {
	// 1 add 2 mul 3 add 4 mul 5 → 1+(2*3)+(4*5) = 1+6+20 = 27
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
		NewWord("add"), NewInteger(4), NewWord("mul"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 27 {
		t.Errorf("got %d, want 27", result[0].AsInteger())
	}
}

func TestEdgePrefixChain(t *testing.T) {
	// 1 2 add 3 4 add mul → (1+2)*(3+4) = 3*7 = 21
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewWord("add"),
		NewInteger(3), NewInteger(4), NewWord("add"),
		NewWord("mul"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	if result[0].AsInteger() != 21 {
		t.Errorf("got %d, want 21", result[0].AsInteger())
	}
}

// --- Edge: modifiers ---

func TestEdgeForcePrefixOnSuffixOnlyLower(t *testing.T) {
	// lower/p with no prefix arg → error (force prefix but no string on stack)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewWordModified("lower", -1, true, false),
		NewString("X"),
	})
	if err == nil {
		t.Fatal("expected error for force prefix with no prefix arg, got nil")
	}
}

func TestEdgeForceSuffixWithPrefixAvailable(t *testing.T) {
	// "A" lower/s "B" → should use suffix, returning 'b', with 'a' remaining
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("A"),
		NewWordModified("lower", -1, false, true),
		NewString("B"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsString() != "A" {
		t.Errorf("result[0] = %q, want 'A'", result[0].AsString())
	}
	if result[1].AsString() != "b" {
		t.Errorf("result[1] = %q, want 'b'", result[1].AsString())
	}
}

func TestEdgeArgCountMismatch(t *testing.T) {
	// lower/2 "X" → error (no signature with 2 args)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewString("X"),
		NewWordModified("lower", 2, false, false),
	})
	if err == nil {
		t.Fatal("expected error for arg count mismatch, got nil")
	}
}

func TestEdgeForcePrefixAdd(t *testing.T) {
	// 1 2 add/p → 3 (force prefix on add)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2),
		NewWordModified("add", -1, true, false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

// --- Edge: end keyword ---

func TestEdgeEndAtStart(t *testing.T) {
	// end → [] (no forward, no-op, removes itself)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewWord("end")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeEndConsecutive(t *testing.T) {
	// end end end → []
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("end"), NewWord("end"), NewWord("end"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeEndTerminatesGetForward(t *testing.T) {
	// "mykey" 42 set end → stores, then get mykey end → [42]
	reg := DefaultRegistry()
	e := New(reg)
	_, err := e.Run([]Value{
		NewString("mykey"), NewInteger(42), NewWord("set"),
	})
	if err != nil {
		t.Fatalf("unexpected error on set: %v", err)
	}

	result, err := e.Run([]Value{
		NewWord("get"), NewWord("mykey"), NewWord("end"),
	})
	if err != nil {
		t.Fatalf("unexpected error on get: %v", err)
	}
	// get collects 1 suffix arg, then end should be no-op since forward is done
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeEndWithMultipleForwards(t *testing.T) {
	// 99 set a end 88 set b end (get a) (get b) → [99, 88]
	// Parentheses isolate each get so the first result doesn't become
	// a prefix argument for the second get.
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(99), NewWord("set"), NewWord("a"), NewWord("end"),
		NewInteger(88), NewWord("set"), NewWord("b"), NewWord("end"),
		NewWord("("), NewWord("get"), NewWord("a"), NewWord(")"),
		NewWord("("), NewWord("get"), NewWord("b"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsInteger() != 99 {
		t.Errorf("result[0] = %d, want 99", result[0].AsInteger())
	}
	if result[1].AsInteger() != 88 {
		t.Errorf("result[1] = %d, want 88", result[1].AsInteger())
	}
}

func TestEdgeEndBetweenLiterals(t *testing.T) {
	// 1 2 end 3 → [1, 2, 3]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewWord("end"), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
}

// --- Edge: set/get ---

func TestEdgeSetWithIntegerKey(t *testing.T) {
	// 42 100 set → uses integer as key
	reg := DefaultRegistry()
	e := New(reg)
	_, err := e.Run([]Value{
		NewInteger(42), NewInteger(100), NewWord("set"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// get 42 → 100
	result, err := e.Run([]Value{
		NewInteger(42), NewWord("get"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 100 {
		t.Errorf("got %v, want [100]", result)
	}
}

func TestEdgeSetEmptyString(t *testing.T) {
	reg := DefaultRegistry()
	e := New(reg)
	_, err := e.Run([]Value{
		NewString(""), NewInteger(1), NewWord("set"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := e.Run([]Value{
		NewString(""), NewWord("get"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 1 {
		t.Errorf("got %d, want 1", result[0].AsInteger())
	}
}

func TestEdgeSetValueIsString(t *testing.T) {
	// set greeting hello end get greeting → ['hello']
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("greeting"), NewString("hello"),
		NewWord("end"),
		NewWord("get"), NewWord("greeting"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsString() != "hello" {
		t.Errorf("got %q, want 'hello'", result[0].AsString())
	}
}

func TestEdgeSetThenUseValue(t *testing.T) {
	// set x 10 end get x add 5 → [15]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("x"), NewInteger(10),
		NewWord("end"),
		NewWord("get"), NewWord("x"),
		NewWord("add"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 15 {
		t.Errorf("got %v, want [15]", result)
	}
}

func TestEdgeSetComputedValue(t *testing.T) {
	// set total (3 mul 7) end get total → [21]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("total"),
		NewWord("("), NewInteger(3), NewWord("mul"), NewInteger(7), NewWord(")"),
		NewWord("end"),
		NewWord("get"), NewWord("total"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 21 {
		t.Errorf("got %v, want [21]", result)
	}
}

// --- Edge: precedence interactions ---

func TestEdgePrecedenceSubMul(t *testing.T) {
	// 10 sub 2 mul 3 → 10-(2*3) = 4
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(10), NewWord("sub"), NewInteger(2), NewWord("mul"), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 4 {
		t.Errorf("got %d, want 4", result[0].AsInteger())
	}
}

func TestEdgePrecedenceMulSub(t *testing.T) {
	// 2 mul 3 sub 1 → (2*3)-1 = 5
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("sub"), NewInteger(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 5 {
		t.Errorf("got %d, want 5", result[0].AsInteger())
	}
}

func TestEdgePrecedenceDivAdd(t *testing.T) {
	// 1 add 10 div 2 → 1+(10/2) = 6
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(10), NewWord("div"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 6 {
		t.Errorf("got %d, want 6", result[0].AsInteger())
	}
}

func TestEdgePrecedenceModAdd(t *testing.T) {
	// 1 add 10 mod 3 → 1+(10%3) = 1+1 = 2
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(10), NewWord("mod"), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 2 {
		t.Errorf("got %d, want 2", result[0].AsInteger())
	}
}

func TestEdgePrecedenceAllOps(t *testing.T) {
	// 1 add 2 mul 3 sub 4 div 2 → 1+(2*3)-(4/2) = 1+6-2 = 5
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
		NewWord("sub"), NewInteger(4), NewWord("div"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsInteger() != 5 {
		t.Errorf("got %d, want 5", result[0].AsInteger())
	}
}

// --- Edge: parentheses ---

func TestEdgeEmptyParens(t *testing.T) {
	// () → [] (empty parens produce no values)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeParenMultipleValues(t *testing.T) {
	// (1 2 3) → [1, 2, 3]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(1), NewInteger(2), NewInteger(3), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, want := range []int64{1, 2, 3} {
		if result[i].AsInteger() != want {
			t.Errorf("result[%d] = %d, want %d", i, result[i].AsInteger(), want)
		}
	}
}

func TestEdgeParenDeeplyNested(t *testing.T) {
	// ((( 5 ))) → [5]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewWord("("), NewWord("("),
		NewInteger(5),
		NewWord(")"), NewWord(")"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEdgeParenNestedArithmetic(t *testing.T) {
	// ((2 add 3) mul (4 sub 1)) → (5 * 3) = 15
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("),
		NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
		NewWord("mul"),
		NewWord("("), NewInteger(4), NewWord("sub"), NewInteger(1), NewWord(")"),
		NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 15 {
		t.Errorf("got %v, want [15]", result)
	}
}

func TestEdgeParenWithFunction(t *testing.T) {
	// (hello upper) → ['HELLO']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewString("hello"), NewWord("upper"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "HELLO" {
		t.Errorf("got %v, want ['HELLO']", result)
	}
}

func TestEdgeParenWithDup(t *testing.T) {
	// (1 dup) → [1, 1]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(1), NewWord("dup"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
}

func TestEdgeParenAfterLiteral(t *testing.T) {
	// 10 (5) → [10, 5]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(10), NewWord("("), NewInteger(5), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsInteger() != 10 || result[1].AsInteger() != 5 {
		t.Errorf("got %v, want [10, 5]", result)
	}
}

func TestEdgeParenCloseWithNoOpen(t *testing.T) {
	// ) → error
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{NewWord(")")})
	if err == nil {
		t.Fatal("expected error for ) with no (, got nil")
	}
}

func TestEdgeParenMultipleOpenUnmatched(t *testing.T) {
	// (( 1 ) → error (one ( left unmatched)
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewWord("("), NewWord("("), NewInteger(1), NewWord(")"),
	})
	if err == nil {
		t.Fatal("expected error for unmatched (, got nil")
	}
}

func TestEdgeParenConsecutive(t *testing.T) {
	// (1) (2) add → 1+2 = 3
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(1), NewWord(")"),
		NewWord("("), NewInteger(2), NewWord(")"),
		NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeParenWithUnknownWord(t *testing.T) {
	// (foo) → ['foo']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewWord("foo"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "foo" {
		t.Errorf("got %v, want ['foo']", result)
	}
}

func TestEdgeParenOrphanedForwardInside(t *testing.T) {
	// (add 1) → error: add creates forward inside paren, but only 1 arg collected
	// There's not enough to resolve
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewWord("("), NewWord("add"), NewInteger(1), NewWord(")"),
	})
	if err == nil {
		t.Fatal("expected error for orphaned forward inside parens, got nil")
	}
}

func TestEdgeParenBarrierStopsForwardSearch(t *testing.T) {
	// 1 add (2) → the forward for add should not cross the paren barrier.
	// Instead, (2) resolves to 2, which then gets collected by add's forward.
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"),
		NewWord("("), NewInteger(2), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeParenWithEndNoOp(t *testing.T) {
	// (1 end) → end acts as no-op inside parens (no forward), yields [1]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(1), NewWord("end"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("got %v, want [1]", result)
	}
}

func TestEdgeParenComplexExpression(t *testing.T) {
	// 2 mul (3 add 4 mul 5) → 2*(3+(4*5)) = 2*(3+20) = 2*23 = 46
	// Inside parens: precedence still applies: 3 add 4 mul 5 → 3+(4*5) = 23
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("mul"),
		NewWord("("), NewInteger(3), NewWord("add"), NewInteger(4), NewWord("mul"), NewInteger(5), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 46 {
		t.Errorf("got %v, want [46]", result)
	}
}

func TestEdgeParenSiblingExpressions(t *testing.T) {
	// (1 add 2) mul (3 add 4) → 3*7 = 21
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
		NewWord("mul"),
		NewWord("("), NewInteger(3), NewWord("add"), NewInteger(4), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 21 {
		t.Errorf("got %v, want [21]", result)
	}
}

// --- Edge: combined features ---

func TestEdgeSetGetComputedKeyAndValue(t *testing.T) {
	// set (lower KEY) (2 add 3) end get key → [5]
	// (lower KEY) → 'key', (2 add 3) → 5
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"),
		NewWord("("), NewWord("lower"), NewString("KEY"), NewWord(")"),
		NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
		NewWord("end"),
		NewWord("get"), NewWord("key"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEdgeDupThenAdd(t *testing.T) {
	// 5 dup add → 5+5 = 10
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(5), NewWord("dup"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestEdgeSwapThenSub(t *testing.T) {
	// 3 10 swap sub → 10-3 = 7 (swap makes 10 first arg)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(3), NewInteger(10), NewWord("swap"), NewWord("sub"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 7 {
		t.Errorf("got %v, want [7]", result)
	}
}

func TestEdgeDropThenOp(t *testing.T) {
	// 1 2 3 drop add → 1+2 = 3
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3), NewWord("drop"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeUpperInParens(t *testing.T) {
	// (abc upper) → ['ABC']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewString("abc"), NewWord("upper"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "ABC" {
		t.Errorf("got %v, want ['ABC']", result)
	}
}

func TestEdgeMixedStringAndIntOnStack(t *testing.T) {
	// "hello" 42 "world" → ['hello', 42, 'world']
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("hello"), NewInteger(42), NewString("world"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
}

func TestEdgeChainUpperLower(t *testing.T) {
	// "Hello" upper lower → 'hello'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("Hello"), NewWord("upper"), NewWord("lower"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "hello" {
		t.Errorf("got %v, want ['hello']", result)
	}
}

func TestEdgeSuffixUpperThenLower(t *testing.T) {
	// lower (upper abc) → lower 'ABC' → 'abc'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("lower"),
		NewWord("("), NewString("abc"), NewWord("upper"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "abc" {
		t.Errorf("got %v, want ['abc']", result)
	}
}

// --- Edge: signature matching specifics ---

func TestEdgeAddWithStringAndInt(t *testing.T) {
	// "hello" 1 add → "hello1" (string concatenation via scalar+scalar)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("hello"), NewInteger(1), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "hello1" {
		t.Fatalf("got %v, want 'hello1'", result)
	}
}

func TestEdgePrefixMatchSpecificity(t *testing.T) {
	// Verify that more specific signatures win
	// upper takes [string], which matches "hello" (string/proper)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("test"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].AsString() != "TEST" {
		t.Errorf("got %q, want 'TEST'", result[0].AsString())
	}
}

// --- Edge: effectiveResolved scoping ---

func TestEdgePrefixMatchDoesNotCrossParen(t *testing.T) {
	// 1 ( 2 add ) → error: inside paren, add sees only [2] as prefix, needs 2 ints
	// Actually 2 add: prefix [int,int] needs 2 ints, but only 1 inside paren.
	// So it falls through to suffix (infix) match: [int|int], but then needs suffix arg.
	// ')' closes paren, orphaned forward error.
	e := New(DefaultRegistry())
	_, err := e.Run([]Value{
		NewInteger(1),
		NewWord("("), NewInteger(2), NewWord("add"), NewWord(")"),
	})
	if err == nil {
		t.Fatal("expected error for add with insufficient args in paren scope, got nil")
	}
}

// --- Edge: registry ---

func TestEdgeLookupUnknown(t *testing.T) {
	r := DefaultRegistry()
	fn := r.Lookup("nonexistent")
	if fn != nil {
		t.Errorf("expected nil for unknown function, got %v", fn)
	}
}

func TestEdgeEmptyRegistry(t *testing.T) {
	r := NewRegistry()
	e := New(r)
	// Everything becomes unknown word → string
	result, err := e.Run([]Value{NewWord("foo"), NewWord("bar")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	if result[0].AsString() != "foo" || result[1].AsString() != "bar" {
		t.Errorf("got %v, want ['foo', 'bar']", result)
	}
}

func TestEdgeEmptyRegistryEndStillWorks(t *testing.T) {
	r := NewRegistry()
	e := New(r)
	result, err := e.Run([]Value{NewInteger(1), NewWord("end")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("got %v, want [1]", result)
	}
}

func TestEdgeEmptyRegistryParensStillWork(t *testing.T) {
	r := NewRegistry()
	e := New(r)
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(42), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Edge: function results re-examination ---

func TestEdgeResultCollectedByPendingForward(t *testing.T) {
	// lower (upper abc) → forward for lower should collect result of (upper abc)
	// (upper abc) → 'ABC', then lower's forward collects it → 'abc'
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("lower"),
		NewWord("("), NewString("abc"), NewWord("upper"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "abc" {
		t.Errorf("got %v, want ['abc']", result)
	}
}

func TestEdgePrefixResultFeedsInfix(t *testing.T) {
	// 2 3 add add 4 → (2+3) produces 5 via prefix, then 5 add 4 → but wait,
	// the second add sees 5 on stack as prefix match [int], and sets up forward for 4
	// Result: 9
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(2), NewInteger(3), NewWord("add"),
		NewWord("add"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 9 {
		t.Errorf("got %v, want [9]", result)
	}
}

// --- Edge: store isolation ---

func TestEdgeStoreIsolationBetweenRegistries(t *testing.T) {
	// Two different registries should have separate stores
	reg1 := DefaultRegistry()
	reg2 := DefaultRegistry()
	e1 := New(reg1)
	e2 := New(reg2)

	_, err := e1.Run([]Value{
		NewWord("set"), NewWord("key"), NewInteger(111),
	})
	if err != nil {
		t.Fatalf("unexpected error on set: %v", err)
	}

	_, err = e2.Run([]Value{
		NewWord("get"), NewWord("key"),
	})
	if err == nil {
		t.Fatal("expected error: key should not exist in separate registry")
	}
}

// --- Edge: single-element inputs ---

func TestEdgeSingleInteger(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewInteger(0)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 0 {
		t.Errorf("got %v, want [0]", result)
	}
}

func TestEdgeSingleString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("x")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "x" {
		t.Errorf("got %v, want ['x']", result)
	}
}

func TestEdgeSingleEmptyString(t *testing.T) {
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{NewString("")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "" {
		t.Errorf("got %v, want ['']", result)
	}
}

// --- Edge: forward details ---

func TestEdgeForwardInfoFields(t *testing.T) {
	info := ForwardInfo{
		FuncName:      "test",
		ExpectedArgs:  3,
		CollectedArgs: 1,
		FuncIndex:     5,
		Precedence:    2,
	}
	v := NewForward(info)
	got := v.AsForward()
	if got.FuncName != "test" {
		t.Errorf("FuncName = %q, want 'test'", got.FuncName)
	}
	if got.ExpectedArgs != 3 {
		t.Errorf("ExpectedArgs = %d, want 3", got.ExpectedArgs)
	}
	if got.CollectedArgs != 1 {
		t.Errorf("CollectedArgs = %d, want 1", got.CollectedArgs)
	}
	if got.FuncIndex != 5 {
		t.Errorf("FuncIndex = %d, want 5", got.FuncIndex)
	}
	if got.Precedence != 2 {
		t.Errorf("Precedence = %d, want 2", got.Precedence)
	}
}

// --- Edge: signature edge cases ---

func TestEdgeSignatureNoPrefix(t *testing.T) {
	// A function with only suffix should work when called with no prefix stack
	r := NewRegistry()
	r.Register("echo", Signature{
		Args:    []Type{TAny},
		Handler: func(args []Value) ([]Value, error) { return args, nil },
	})
	e := New(r)
	result, err := e.Run([]Value{NewWord("echo"), NewInteger(42)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeSignatureMultipleSuffix(t *testing.T) {
	// A function that takes 2 suffix args
	r := NewRegistry()
	r.Register("pair", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return args, nil
		},
	})
	e := New(r)
	result, err := e.Run([]Value{
		NewWord("pair"), NewInteger(1), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
}

func TestEdgeSignatureReturnsMultiple(t *testing.T) {
	// A function that returns multiple values
	r := NewRegistry()
	r.Register("triple", Signature{
		Args: []Type{TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[0], args[0]}, nil
		},
	})
	e := New(r)
	result, err := e.Run([]Value{NewInteger(7), NewWord("triple")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, v := range result {
		if v.AsInteger() != 7 {
			t.Errorf("result[%d] = %d, want 7", i, v.AsInteger())
		}
	}
}

func TestEdgeSignatureReturnsNothing(t *testing.T) {
	// A function that returns nothing (like drop)
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewWord("drop"), NewWord("drop"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want []", result)
	}
}

// --- Edge: interactions between end and parens ---

func TestEdgeEndInsideParenNoForward(t *testing.T) {
	// (42 end) → end is no-op inside parens, gives [42]
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewWord("("), NewInteger(42), NewWord("end"), NewWord(")"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeEndOutsideParenDoesNotCrossBarrier(t *testing.T) {
	// set a (1 add 2) end get a → set has forward, (1 add 2)=3 is collected,
	// end terminates the forward for set, get a → [3]
	reg := DefaultRegistry()
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("set"), NewWord("a"),
		NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
		NewWord("end"),
		NewWord("get"), NewWord("a"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

// --- Engine tests: def (word definition) ---

func TestDefBasicListBody(t *testing.T) {
	// def increment [1 add]  2 increment → 3
	reg := DefaultRegistry()
	e := New(reg)

	// First run: define increment
	_, err := e.Run([]Value{
		NewWord("def"), NewWord("increment"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	// Second run: use increment
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("increment"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestDefScalarBody(t *testing.T) {
	// def myval 42  myval → 42
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("myval"), NewInteger(42),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewWord("myval"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestDefStringName(t *testing.T) {
	// def "double" [dup add]  5 double → 10
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewString("double"),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(5), NewWord("double"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestDefPrefixBodyStringName(t *testing.T) {
	// [1 add] def "inc" 10 inc → 11
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewWord("def"), NewString("inc"),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(10), NewWord("inc"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 11 {
		t.Errorf("got %v, want [11]", result)
	}
}

func TestDefPrefixBody(t *testing.T) {
	// [1 sub] def decrement  3 decrement → 2
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewList([]Value{NewInteger(1), NewWord("sub")}),
		NewWord("def"), NewWord("decrement"),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("decrement"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("got %v, want [2]", result)
	}
}

func TestDefAndUseSameRun(t *testing.T) {
	// def triple [dup dup add add] 4 triple → 12
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("triple"),
		NewList([]Value{NewWord("dup"), NewWord("dup"), NewWord("add"), NewWord("add")}),
		NewInteger(4), NewWord("triple"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefDoesNotBreakExistingWordCoercion(t *testing.T) {
	// Unknown words without a pending TWord forward still coerce to strings.
	// a upper → "A"
	e := New(DefaultRegistry())
	result, err := e.Run([]Value{
		NewString("a"), NewWord("upper"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "A" {
		t.Errorf("got %v, want ['A']", result)
	}
}

func TestDefUndefinedWordAcceptedByTWord(t *testing.T) {
	// Undefined word "foo" is preserved as TWord when def's forward expects it.
	// def foo 99  foo → 99
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(99),
		NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Errorf("got %v, want [99]", result)
	}
}

func TestDefStringBody(t *testing.T) {
	// def greeting "hello"  greeting → "hello"
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("greeting"), NewString("hello"),
		NewWord("greeting"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "hello" {
		t.Errorf("got %v, want ['hello']", result)
	}
}

func TestDefUsedMultipleTimes(t *testing.T) {
	// def inc [1 add]  1 inc inc inc → 4
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewInteger(1), NewWord("inc"), NewWord("inc"), NewWord("inc"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 4 {
		t.Errorf("got %v, want [4]", result)
	}
}

// --- Engine tests: def (traditional Forth-style word definitions) ---

func TestDefForthSquare(t *testing.T) {
	// : square dup mul ;
	// Classic Forth square: duplicates top of stack and multiplies.
	// def square [dup mul]  5 square → 25
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
		NewInteger(5), NewWord("square"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 25 {
		t.Errorf("got %v, want [25]", result)
	}
}

func TestDefForthNegate(t *testing.T) {
	// : negate 0 swap sub ;
	// Negates a number: 0 - n.
	// def negate [0 swap sub]  7 negate → -7
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("negate"),
		NewList([]Value{NewInteger(0), NewWord("swap"), NewWord("sub")}),
		NewInteger(7), NewWord("negate"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != -7 {
		t.Errorf("got %v, want [-7]", result)
	}
}

func TestDefForthOver(t *testing.T) {
	// : over swap dup rot ;
	// In standard Forth, over copies the second item to the top.
	// Without rot, we simulate: over = [swap dup] gives (a b → b a a)
	// which isn't over. Instead test the concept of building combinators.
	// def dup2 [dup dup]  3 dup2 → 3 3 3
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("dup2"),
		NewList([]Value{NewWord("dup"), NewWord("dup")}),
		NewInteger(3), NewWord("dup2"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, v := range result {
		if v.AsInteger() != 3 {
			t.Errorf("result[%d] = %d, want 3", i, v.AsInteger())
		}
	}
}

func TestDefForthComposition(t *testing.T) {
	// Define words in terms of other defined words.
	// : double dup add ;
	// : quadruple double double ;
	// 3 quadruple → 12
	reg := DefaultRegistry()
	e := New(reg)

	// Define double
	_, err := e.Run([]Value{
		NewWord("def"), NewWord("double"),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def double: %v", err)
	}

	// Define quadruple in terms of double
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("quadruple"),
		NewList([]Value{NewWord("double"), NewWord("double")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def quadruple: %v", err)
	}

	// Use quadruple
	result, err := e.Run([]Value{
		NewInteger(3), NewWord("quadruple"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefForthThreeDeepComposition(t *testing.T) {
	// : inc 1 add ;
	// : inc2 inc inc ;
	// : inc6 inc2 inc2 inc2 ;
	// 0 inc6 → 6
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("inc2"),
		NewList([]Value{NewWord("inc"), NewWord("inc")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc2: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("inc6"),
		NewList([]Value{NewWord("inc2"), NewWord("inc2"), NewWord("inc2")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc6: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(0), NewWord("inc6"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 6 {
		t.Errorf("got %v, want [6]", result)
	}
}

func TestDefForthSumOfSquares(t *testing.T) {
	// : square dup mul ;
	// 3 square 4 square add → 9 + 16 = 25
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("square"),
		NewInteger(4), NewWord("square"),
		NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 25 {
		t.Errorf("got %v, want [25]", result)
	}
}

func TestDefForthCube(t *testing.T) {
	// : square dup mul ;
	// : cube dup square mul ;
	// Note: cube duplicates n, squares one copy, then multiplies: n * n^2 = n^3
	// 3 cube → 27
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def square: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("cube"),
		NewList([]Value{NewWord("dup"), NewWord("square"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def cube: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("cube"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 27 {
		t.Errorf("got %v, want [27]", result)
	}
}

func TestDefForthWithInfixOps(t *testing.T) {
	// Defined words work with infix operators from the calling context.
	// : double dup add ;
	// 3 double mul 2 → 6 * 2 = 12
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("double"),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("double"),
		NewWord("mul"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefForthConstant(t *testing.T) {
	// : pi 3 ;   (Forth-style constant as a word that pushes a value)
	// pi pi add → 6
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("pi"), NewInteger(3),
		NewWord("pi"), NewWord("pi"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 6 {
		t.Errorf("got %v, want [6]", result)
	}
}

func TestDefForthStackEffectMultipleValues(t *testing.T) {
	// A word that pushes multiple values onto the stack.
	// : pair1 1 2 ;
	// pair1 add → 3
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("pair1"),
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewWord("pair1"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestDefForthSwapSub(t *testing.T) {
	// : nip swap drop ;
	// Nip removes second element: (a b → b)
	// 10 20 nip → 20
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("nip"),
		NewList([]Value{NewWord("swap"), NewWord("drop")}),
		NewInteger(10), NewInteger(20), NewWord("nip"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 20 {
		t.Errorf("got %v, want [20]", result)
	}
}

func TestDefForthAbsDiff(t *testing.T) {
	// Absolute difference using two defined words.
	// : monus swap sub ;  (reversed subtraction: a b → b-a)
	// Then compute |3-7| by choosing the larger minus smaller.
	// 7 3 monus → 7 - 3 = 4  (swap makes it 3 7, then sub gives 7-3=4)
	// Wait: swap sub on (7, 3) → swap gives (3, 7), sub gives 3-7=-4.
	// Actually sub is prefix [a, b] → a - b, so (3, 7) → 3 - 7 = -4.
	// Let me reconsider: just demonstrate sub with swap.
	// def rsub [swap sub]  3 7 rsub → swap gives (7,3), sub gives 7-3=4
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("rsub"),
		NewList([]Value{NewWord("swap"), NewWord("sub")}),
		NewInteger(3), NewInteger(7), NewWord("rsub"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 4 {
		t.Errorf("got %v, want [4]", result)
	}
}

func TestDefForthMultipleDefsInSameRun(t *testing.T) {
	// Define multiple words in a single run, then use them together.
	// : inc 1 add ;
	// : dec 1 sub ;
	// 10 inc inc dec → 11
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewWord("def"), NewWord("dec"),
		NewList([]Value{NewInteger(1), NewWord("sub")}),
		NewInteger(10), NewWord("inc"), NewWord("inc"), NewWord("dec"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 11 {
		t.Errorf("got %v, want [11]", result)
	}
}

func TestDefForthStringWord(t *testing.T) {
	// A word that pushes a string and operates on it.
	// : shout upper ;
	// "hello" shout → "HELLO"
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("shout"),
		NewList([]Value{NewWord("upper")}),
		NewString("hello"), NewWord("shout"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "HELLO" {
		t.Errorf("got %v, want ['HELLO']", result)
	}
}

func TestDefForthPersistsAcrossRuns(t *testing.T) {
	// Definitions persist in the registry across Run calls,
	// mirroring how Forth words persist once defined.
	reg := DefaultRegistry()
	e := New(reg)

	// Run 1: define square
	_, err := e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	// Run 2: define cube using square
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("cube"),
		NewList([]Value{NewWord("dup"), NewWord("square"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def cube: %v", err)
	}

	// Run 3: use cube (tests both definitions persisted)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("cube"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 8 {
		t.Errorf("got %v, want [8]", result)
	}

	// Run 4: use square independently
	result, err = e.Run([]Value{
		NewInteger(7), NewWord("square"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 49 {
		t.Errorf("got %v, want [49]", result)
	}
}

func TestDefForthDefWithEnd(t *testing.T) {
	// Using end to terminate def's suffix collection early,
	// with the body coming from the prefix stack.
	// [dup add] def double end 5 double → 10
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewList([]Value{NewWord("dup"), NewWord("add")}),
		NewWord("def"), NewWord("double"), NewWord("end"),
		NewInteger(5), NewWord("double"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestDefForthFactorial5(t *testing.T) {
	// Compute 5! = 120 iteratively using defined words.
	// Without loops, we manually unroll:
	// : mul5 5 mul ;    (just multiply by a constant)
	// 1 mul5 → 5, then 4 mul → 20, then 3 mul → 60, then 2 mul → 120
	// This tests defined words mixed with inline operations.
	reg := DefaultRegistry()
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("mul5"),
		NewList([]Value{NewInteger(5), NewWord("mul")}),
		NewInteger(1), NewWord("mul5"),
		NewInteger(4), NewWord("mul"),
		NewInteger(3), NewWord("mul"),
		NewInteger(2), NewWord("mul"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 120 {
		t.Errorf("got %v, want [120]", result)
	}
}

func TestDefForthDefInteractsWithStore(t *testing.T) {
	// Defined words that use set/get to interact with the store.
	// : save-x set x end ;
	// : load-x get x ;
	// 42 save-x load-x → 42
	reg := DefaultRegistry()
	e := New(reg)

	_, err := e.Run([]Value{
		NewWord("def"), NewWord("save-x"),
		NewList([]Value{NewWord("set"), NewWord("x"), NewWord("end")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def save-x: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("load-x"),
		NewList([]Value{NewWord("get"), NewWord("x")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def load-x: %v", err)
	}

	_, err = e.Run([]Value{
		NewInteger(42), NewWord("save-x"),
	})
	if err != nil {
		t.Fatalf("unexpected error on save-x: %v", err)
	}

	result, err := e.Run([]Value{
		NewWord("load-x"),
	})
	if err != nil {
		t.Fatalf("unexpected error on load-x: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Stack helper method tests ---

func TestStackInsert(t *testing.T) {
	tests := []struct {
		name  string
		start []int
		idx   int
		val   int
		want  []int
	}{
		{"insert at start", []int{2, 3}, 0, 1, []int{1, 2, 3}},
		{"insert at middle", []int{1, 3}, 1, 2, []int{1, 2, 3}},
		{"insert at end", []int{1, 2}, 2, 3, []int{1, 2, 3}},
		{"insert into empty", []int{}, 0, 1, []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{}
			e.stack = make([]Value, len(tt.start))
			for i, v := range tt.start {
				e.stack[i] = NewInteger(int64(v))
			}
			e.stackInsert(tt.idx, NewInteger(int64(tt.val)))
			if len(e.stack) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(e.stack), len(tt.want))
			}
			for i, w := range tt.want {
				if e.stack[i].AsInteger() != int64(w) {
					t.Errorf("stack[%d] = %d, want %d", i, e.stack[i].AsInteger(), w)
				}
			}
		})
	}
}

func TestStackRemove(t *testing.T) {
	tests := []struct {
		name  string
		start []int
		idx   int
		want  []int
	}{
		{"remove first", []int{1, 2, 3}, 0, []int{2, 3}},
		{"remove middle", []int{1, 2, 3}, 1, []int{1, 3}},
		{"remove last", []int{1, 2, 3}, 2, []int{1, 2}},
		{"remove only", []int{1}, 0, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{}
			e.stack = make([]Value, len(tt.start))
			for i, v := range tt.start {
				e.stack[i] = NewInteger(int64(v))
			}
			e.stackRemove(tt.idx)
			if len(e.stack) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(e.stack), len(tt.want))
			}
			for i, w := range tt.want {
				if e.stack[i].AsInteger() != int64(w) {
					t.Errorf("stack[%d] = %d, want %d", i, e.stack[i].AsInteger(), w)
				}
			}
		})
	}
}

func TestStackSplice(t *testing.T) {
	tests := []struct {
		name         string
		start        []int
		idx, count   int
		replacements []int
		want         []int
	}{
		{"replace 1 with 1", []int{1, 2, 3}, 1, 1, []int{9}, []int{1, 9, 3}},
		{"shrink", []int{1, 2, 3, 4}, 1, 2, []int{9}, []int{1, 9, 4}},
		{"grow", []int{1, 4}, 1, 1, []int{2, 3}, []int{1, 2, 3}},
		{"remove all", []int{1, 2, 3}, 0, 3, []int{}, []int{}},
		{"insert at start", []int{2, 3}, 0, 0, []int{1}, []int{1, 2, 3}},
		{"insert at end", []int{1, 2}, 2, 0, []int{3}, []int{1, 2, 3}},
		{"replace many with many", []int{1, 2, 3, 4, 5}, 1, 3, []int{8, 9}, []int{1, 8, 9, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{}
			e.stack = make([]Value, len(tt.start))
			for i, v := range tt.start {
				e.stack[i] = NewInteger(int64(v))
			}
			reps := make([]Value, len(tt.replacements))
			for i, v := range tt.replacements {
				reps[i] = NewInteger(int64(v))
			}
			e.stackSplice(tt.idx, tt.count, reps...)
			if len(e.stack) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(e.stack), len(tt.want))
			}
			for i, w := range tt.want {
				if e.stack[i].AsInteger() != int64(w) {
					t.Errorf("stack[%d] = %d, want %d", i, e.stack[i].AsInteger(), w)
				}
			}
		})
	}
}

func TestStackInsertWithHeadroom(t *testing.T) {
	e := &Engine{}
	e.stack = make([]Value, 3, 10)
	for i := range e.stack {
		e.stack[i] = NewInteger(int64(i + 1))
	}
	e.stackInsert(1, NewInteger(99))
	if len(e.stack) != 4 {
		t.Fatalf("len = %d, want 4", len(e.stack))
	}
	if cap(e.stack) != 10 {
		t.Fatalf("cap = %d, want 10 (should not have reallocated)", cap(e.stack))
	}
	want := []int64{1, 99, 2, 3}
	for i, w := range want {
		if e.stack[i].AsInteger() != w {
			t.Errorf("stack[%d] = %d, want %d", i, e.stack[i].AsInteger(), w)
		}
	}
}

// --- Record type tests ---

func TestRecordTypeCreation(t *testing.T) {
	e := New(DefaultRegistry())

	// record {x:number, y:number} => record{x:number,y:number}
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TNumber))
	input := []Value{NewWord("record"), NewMap(fields)}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	if !result[0].IsRecordType() {
		t.Fatalf("result is not a record type: %s", result[0].String())
	}
	if result[0].String() != "record{x:number,y:number}" {
		t.Errorf("got %s, want record{x:number,y:number}", result[0].String())
	}
}

func TestRecordTypeWithDef(t *testing.T) {
	e := New(DefaultRegistry())

	// def Point record {x:number, y:number}
	// Point => record{x:number,y:number}
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TNumber))
	input := []Value{
		NewWord("def"), NewWord("Point"),
		NewWord("record"), NewMap(fields),
		NewWord("end"),
		NewWord("Point"),
	}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	if !result[0].IsRecordType() {
		t.Fatalf("result is not a record type: %s", result[0].String())
	}
	if result[0].String() != "record{x:number,y:number}" {
		t.Errorf("got %s, want record{x:number,y:number}", result[0].String())
	}
}

func TestRecordTypeUnifyWithMap(t *testing.T) {
	e := New(DefaultRegistry())

	tests := []struct {
		name    string
		input   []Value
		want    string
		wantLen int
	}{
		{
			name: "record unifies with matching map",
			input: func() []Value {
				fields := NewOrderedMap()
				fields.Set("x", NewTypeLiteral(TNumber))
				fields.Set("y", NewTypeLiteral(TNumber))
				m := NewOrderedMap()
				m.Set("x", NewInteger(1))
				m.Set("y", NewInteger(2))
				return []Value{NewMap(m), NewRecordType(fields), NewWord("unify")}
			}(),
			want:    "{x:1,y:2} true",
			wantLen: 2,
		},
		{
			name: "record fails with wrong value type",
			input: func() []Value {
				fields := NewOrderedMap()
				fields.Set("x", NewTypeLiteral(TNumber))
				fields.Set("y", NewTypeLiteral(TNumber))
				m := NewOrderedMap()
				m.Set("x", NewString("hello"))
				m.Set("y", NewInteger(2))
				return []Value{NewMap(m), NewRecordType(fields), NewWord("unify")}
			}(),
			want:    "'~unify-fail' false",
			wantLen: 2,
		},
		{
			name: "record fails with missing field",
			input: func() []Value {
				fields := NewOrderedMap()
				fields.Set("x", NewTypeLiteral(TNumber))
				fields.Set("y", NewTypeLiteral(TNumber))
				m := NewOrderedMap()
				m.Set("x", NewInteger(1))
				return []Value{NewMap(m), NewRecordType(fields), NewWord("unify")}
			}(),
			want:    "'~unify-fail' false",
			wantLen: 2,
		},
		{
			name: "record fails with extra field",
			input: func() []Value {
				fields := NewOrderedMap()
				fields.Set("x", NewTypeLiteral(TNumber))
				m := NewOrderedMap()
				m.Set("x", NewInteger(1))
				m.Set("y", NewInteger(2))
				return []Value{NewMap(m), NewRecordType(fields), NewWord("unify")}
			}(),
			want:    "'~unify-fail' false",
			wantLen: 2,
		},
		{
			name: "two record types unify when compatible",
			input: func() []Value {
				f1 := NewOrderedMap()
				f1.Set("x", NewTypeLiteral(TAny))
				f2 := NewOrderedMap()
				f2.Set("x", NewTypeLiteral(TNumber))
				return []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")}
			}(),
			want:    "record{x:number} true",
			wantLen: 2,
		},
		{
			name: "two record types fail with different keys",
			input: func() []Value {
				f1 := NewOrderedMap()
				f1.Set("x", NewTypeLiteral(TNumber))
				f2 := NewOrderedMap()
				f2.Set("y", NewTypeLiteral(TNumber))
				return []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")}
			}(),
			want:    "'~unify-fail' false",
			wantLen: 2,
		},
		{
			name: "record with nested type",
			input: func() []Value {
				inner := NewOrderedMap()
				inner.Set("z", NewTypeLiteral(TString))
				fields := NewOrderedMap()
				fields.Set("x", NewTypeLiteral(TNumber))
				fields.Set("y", NewRecordType(inner))
				m := NewOrderedMap()
				m.Set("x", NewInteger(1))
				innerMap := NewOrderedMap()
				innerMap.Set("z", NewString("hello"))
				m.Set("y", NewMap(innerMap))
				return []Value{NewMap(m), NewRecordType(fields), NewWord("unify")}
			}(),
			want:    "{x:1,y:{z:'hello'}} true",
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Fatalf("got %d values, want %d", len(result), tt.wantLen)
			}
			var parts []string
			for _, v := range result {
				parts = append(parts, v.String())
			}
			got := ""
			for i, p := range parts {
				if i > 0 {
					got += " "
				}
				got += p
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRecordTypeMapLiteralUnify(t *testing.T) {
	e := New(DefaultRegistry())

	// map type literal unifies with record type
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	input := []Value{NewTypeLiteral(TMap), NewRecordType(fields), NewWord("unify")}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2", len(result))
	}
	if !result[0].IsRecordType() {
		t.Errorf("expected record type, got %s", result[0].String())
	}
	if !result[1].AsBoolean() {
		t.Errorf("expected true, got false")
	}
}

// --- Benchmarks ---

func BenchmarkSimpleExpression(b *testing.B) {
	reg := DefaultRegistry()
	input := []Value{NewInteger(1), NewInteger(2), NewWord("add")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

func BenchmarkComplexExpression(b *testing.B) {
	reg := DefaultRegistry()
	input := []Value{
		NewInteger(1), NewInteger(2), NewWord("add"),
		NewInteger(3), NewWord("mul"),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

func BenchmarkRepeatedRun(b *testing.B) {
	reg := DefaultRegistry()
	input := []Value{NewInteger(1), NewInteger(2), NewWord("add")}
	eng := New(reg)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Run(input)
	}
}
