package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/engine"
	"github.com/metsitaba/voxgig-exp/lang/native"
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
	r, _ := engine.DefaultRegistry(native.Register)
	// Integer pow
	result := runAQL(t, r, []engine.Value{engine.NewInteger(2), engine.NewWord("pow"), engine.NewInteger(3)})
	_as0, _ := result[0].AsInteger()
	if _as0 != 8 {
		t.Errorf("2 pow 3 = %v, want 8", result[0])
	}
	// Integer pow with 0 exponent
	result = runAQL(t, r, []engine.Value{engine.NewInteger(5), engine.NewWord("pow"), engine.NewInteger(0)})
	_as1, _ := result[0].AsInteger()
	if _as1 != 1 {
		t.Errorf("5 pow 0 = %v, want 1", result[0])
	}
	// Negative exponent should error
	err := runAQLError(t, r, []engine.Value{engine.NewInteger(2), engine.NewWord("pow"), engine.NewInteger(-1)})
	if err == nil {
		t.Error("expected error for negative exponent")
	}
	// Decimal pow
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(2), engine.NewWord("pow"), engine.NewDecimal(0.5)})
	_as2, _ := result[0].AsNumber()
	if math.Abs(_as2-math.Sqrt(2)) > 0.0001 {
		t.Errorf("2 pow 0.5 = %v, want sqrt(2)", result[0])
	}
}

func TestMathDiv(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	// Integer div
	result := runAQL(t, r, []engine.Value{engine.NewInteger(10), engine.NewWord("div"), engine.NewInteger(3)})
	_as3, _ := result[0].AsInteger()
	if _as3 != 3 {
		t.Errorf("10 div 3 = %v, want 3", result[0])
	}
	// Decimal div
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(10), engine.NewWord("div"), engine.NewDecimal(4)})
	_as4, _ := result[0].AsNumber()
	if _as4 != 2.5 {
		t.Errorf("10.0 div 4.0 = %v, want 2.5", result[0])
	}
	// Decimal div by zero
	err := runAQLError(t, r, []engine.Value{engine.NewDecimal(1), engine.NewWord("div"), engine.NewDecimal(0)})
	if err == nil {
		t.Error("expected error for decimal division by zero")
	}
}

func TestMathMod(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	// Integer mod
	result := runAQL(t, r, []engine.Value{engine.NewInteger(10), engine.NewWord("mod"), engine.NewInteger(3)})
	_as5, _ := result[0].AsInteger()
	if _as5 != 1 {
		t.Errorf("10 mod 3 = %v, want 1", result[0])
	}
	// Decimal mod
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(10.5), engine.NewWord("mod"), engine.NewDecimal(3)})
	_as6, _ := result[0].AsNumber()
	if math.Abs(_as6-1.5) > 0.0001 {
		t.Errorf("10.5 mod 3.0 = %v, want 1.5", result[0])
	}
	// Decimal mod by zero
	err := runAQLError(t, r, []engine.Value{engine.NewDecimal(1), engine.NewWord("mod"), engine.NewDecimal(0)})
	if err == nil {
		t.Error("expected error for decimal modulo by zero")
	}
}

func TestMathMulDecimal(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(2.5), engine.NewWord("mul"), engine.NewDecimal(4)})
	_as7, _ := result[0].AsNumber()
	if _as7 != 10.0 {
		t.Errorf("2.5 mul 4.0 = %v, want 10.0", result[0])
	}
}

func TestMathSubDecimal(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(5.5), engine.NewWord("sub"), engine.NewDecimal(2.5)})
	_as8, _ := result[0].AsNumber()
	if _as8 != 3.0 {
		t.Errorf("5.5 sub 2.5 = %v, want 3.0", result[0])
	}
}

func TestMathAddDecimal(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1.5), engine.NewWord("add"), engine.NewDecimal(2.5)})
	_as9, _ := result[0].AsNumber()
	if _as9 != 4.0 {
		t.Errorf("1.5 add 2.5 = %v, want 4.0", result[0])
	}
}

// --- Boolean operation coverage tests ---

func TestBoolXor(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
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
		result := runAQL(t, r, []engine.Value{engine.NewBoolean(tt.a), engine.NewWord("xor"), engine.NewBoolean(tt.b)})
		_as10, _ := result[0].AsBoolean()
		if _as10 != tt.want {
			_as11, _ := result[0].AsBoolean()
			t.Errorf("%v xor %v = %v, want %v", tt.a, tt.b, _as11, tt.want)
		}
	}
}

func TestBoolNand(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewBoolean(true), engine.NewWord("nand"), engine.NewBoolean(true)})
	_as12, _ := result[0].AsBoolean()
	if _as12 != false {
		t.Errorf("true nand true = %v, want false", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewBoolean(true), engine.NewWord("nand"), engine.NewBoolean(false)})
	_as13, _ := result[0].AsBoolean()
	if _as13 != true {
		t.Errorf("true nand false = %v, want true", result[0])
	}
}

func TestBoolImplies(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewBoolean(true), engine.NewWord("implies"), engine.NewBoolean(false)})
	_as14, _ := result[0].AsBoolean()
	if _as14 != false {
		t.Errorf("true implies false = %v, want false", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewBoolean(false), engine.NewWord("implies"), engine.NewBoolean(false)})
	_as15, _ := result[0].AsBoolean()
	if _as15 != true {
		t.Errorf("false implies false = %v, want true", result[0])
	}
}

func TestBoolTorNonBoolean(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(1), engine.NewWord("tor"), engine.NewInteger(2)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].IsDisjunct() {
		t.Errorf("expected disjunct, got %s", result[0])
	}
}

// --- Mixed integer/decimal operations ---

func TestMathAddMixed(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewDecimal(2.5)})
	_as16, _ := result[0].AsNumber()
	if _as16 != 3.5 {
		t.Errorf("1 add 2.5 = %v, want 3.5", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(1.5), engine.NewWord("add"), engine.NewInteger(2)})
	_as17, _ := result[0].AsNumber()
	if _as17 != 3.5 {
		t.Errorf("1.5 add 2 = %v, want 3.5", result[0])
	}
}

// --- String add (concatenation) ---

func TestStringAdd(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewString("hello"), engine.NewWord("add"), engine.NewString(" world")})
	_as18, _ := result[0].AsString()
	if _as18 != "hello world" {
		t.Errorf("expected 'hello world', got %s", result[0])
	}
}
