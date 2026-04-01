package engine

import (
	"math"
	"testing"
)

// --- Math function coverage tests ---
// Tests for basic arithmetic operations (add, sub, mul, div, mod, pow)
// that remain as built-in words.
// Extended math operations (abs, negate, sign, min, max, ceil, floor,
// round, trunc, sqrt, cbrt, exp, log, log2, log10, sin, cos, tan,
// asin, acos, atan, atan2, hypot, math-pi, math-e) are now in the
// "aql:math" native module and tested in internal/nativemod/.

func TestMathPow(t *testing.T) {
	r, _ := DefaultRegistry()
	// Integer pow
	result := runAQL(t, r, []Value{NewInteger(2), NewWord("pow"), NewInteger(3)})
	if result[0].AsInteger() != 8 {
		t.Errorf("2 pow 3 = %v, want 8", result[0])
	}
	// Integer pow with 0 exponent
	result = runAQL(t, r, []Value{NewInteger(5), NewWord("pow"), NewInteger(0)})
	if result[0].AsInteger() != 1 {
		t.Errorf("5 pow 0 = %v, want 1", result[0])
	}
	// Negative exponent should error
	err := runAQLError(t, r, []Value{NewInteger(2), NewWord("pow"), NewInteger(-1)})
	if err == nil {
		t.Error("expected error for negative exponent")
	}
	// Decimal pow
	result = runAQL(t, r, []Value{NewDecimal(2), NewWord("pow"), NewDecimal(0.5)})
	if math.Abs(result[0].AsNumber()-math.Sqrt(2)) > 0.0001 {
		t.Errorf("2 pow 0.5 = %v, want sqrt(2)", result[0])
	}
}

func TestMathDiv(t *testing.T) {
	r, _ := DefaultRegistry()
	// Integer div
	result := runAQL(t, r, []Value{NewInteger(10), NewWord("div"), NewInteger(3)})
	if result[0].AsInteger() != 3 {
		t.Errorf("10 div 3 = %v, want 3", result[0])
	}
	// Decimal div
	result = runAQL(t, r, []Value{NewDecimal(10), NewWord("div"), NewDecimal(4)})
	if result[0].AsNumber() != 2.5 {
		t.Errorf("10.0 div 4.0 = %v, want 2.5", result[0])
	}
	// Decimal div by zero
	err := runAQLError(t, r, []Value{NewDecimal(1), NewWord("div"), NewDecimal(0)})
	if err == nil {
		t.Error("expected error for decimal division by zero")
	}
}

func TestMathMod(t *testing.T) {
	r, _ := DefaultRegistry()
	// Integer mod
	result := runAQL(t, r, []Value{NewInteger(10), NewWord("mod"), NewInteger(3)})
	if result[0].AsInteger() != 1 {
		t.Errorf("10 mod 3 = %v, want 1", result[0])
	}
	// Decimal mod
	result = runAQL(t, r, []Value{NewDecimal(10.5), NewWord("mod"), NewDecimal(3)})
	if math.Abs(result[0].AsNumber()-1.5) > 0.0001 {
		t.Errorf("10.5 mod 3.0 = %v, want 1.5", result[0])
	}
	// Decimal mod by zero
	err := runAQLError(t, r, []Value{NewDecimal(1), NewWord("mod"), NewDecimal(0)})
	if err == nil {
		t.Error("expected error for decimal modulo by zero")
	}
}

func TestMathMulDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(2.5), NewWord("mul"), NewDecimal(4)})
	if result[0].AsNumber() != 10.0 {
		t.Errorf("2.5 mul 4.0 = %v, want 10.0", result[0])
	}
}

func TestMathSubDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(5.5), NewWord("sub"), NewDecimal(2.5)})
	if result[0].AsNumber() != 3.0 {
		t.Errorf("5.5 sub 2.5 = %v, want 3.0", result[0])
	}
}

func TestMathAddDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1.5), NewWord("add"), NewDecimal(2.5)})
	if result[0].AsNumber() != 4.0 {
		t.Errorf("1.5 add 2.5 = %v, want 4.0", result[0])
	}
}

// --- Boolean operation coverage tests ---

func TestBoolXor(t *testing.T) {
	r, _ := DefaultRegistry()
	tests := []struct {
		a, b bool
		want bool
	}{
		{true, true, false},
		{true, false, true},
		{false, true, true},
		{false, false, false},
	}
	for _, tt := range tests {
		result := runAQL(t, r, []Value{NewBoolean(tt.a), NewWord("xor"), NewBoolean(tt.b)})
		if result[0].AsBoolean() != tt.want {
			t.Errorf("%v xor %v = %v, want %v", tt.a, tt.b, result[0].AsBoolean(), tt.want)
		}
	}
}

func TestBoolNand(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("nand"), NewBoolean(true)})
	if result[0].AsBoolean() != false {
		t.Errorf("true nand true = %v, want false", result[0])
	}
	result = runAQL(t, r, []Value{NewBoolean(true), NewWord("nand"), NewBoolean(false)})
	if result[0].AsBoolean() != true {
		t.Errorf("true nand false = %v, want true", result[0])
	}
}

func TestBoolImplies(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("implies"), NewBoolean(false)})
	if result[0].AsBoolean() != false {
		t.Errorf("true implies false = %v, want false", result[0])
	}
	result = runAQL(t, r, []Value{NewBoolean(false), NewWord("implies"), NewBoolean(false)})
	if result[0].AsBoolean() != true {
		t.Errorf("false implies false = %v, want true", result[0])
	}
}

func TestBoolOrNonBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("or"), NewInteger(2)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].IsDisjunct() {
		t.Errorf("expected disjunct, got %s", result[0])
	}
}

// --- Mixed integer/decimal operations ---

func TestMathAddMixed(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("add"), NewDecimal(2.5)})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("1 add 2.5 = %v, want 3.5", result[0])
	}
	result = runAQL(t, r, []Value{NewDecimal(1.5), NewWord("add"), NewInteger(2)})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("1.5 add 2 = %v, want 3.5", result[0])
	}
}

// --- String add (concatenation) ---

func TestStringAdd(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewString("hello"), NewWord("add"), NewString(" world")})
	if result[0].AsString() != "hello world" {
		t.Errorf("expected 'hello world', got %s", result[0])
	}
}
