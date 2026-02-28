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
		// type mismatch: string with add
		{"string add", []Value{NewString("a"), NewInteger(1), NewWord("add")}},
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
