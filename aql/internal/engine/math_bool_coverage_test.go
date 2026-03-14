package engine

import (
	"math"
	"testing"
)

// --- Math function coverage tests ---
// These exercise the registerUnaryNumOp and registerBinaryNumOp paths
// (both integer and decimal overloads).

func TestMathSin(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("sin")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathSinInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(0), NewWord("sin")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathCos(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("cos")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("cos(0) = %v, want 1.0", result[0])
	}
}

func TestMathTan(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("tan")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("tan(0) = %v, want 0.0", result[0])
	}
}

func TestMathAsin(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1), NewWord("asin")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathAcos(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1), NewWord("acos")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("acos(1) = %v, want 0.0", result[0])
	}
}

func TestMathAtan(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("atan")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("atan(0) = %v, want 0.0", result[0])
	}
}

func TestMathAtan2(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("atan2"), NewDecimal(1)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathSqrt(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(4), NewWord("sqrt")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("sqrt(4) = %v, want 2.0", result[0])
	}
}

func TestMathCbrt(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(8), NewWord("cbrt")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("cbrt(8) = %v, want 2.0", result[0])
	}
}

func TestMathExp(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(0), NewWord("exp")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("exp(0) = %v, want 1.0", result[0])
	}
}

func TestMathLog(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1), NewWord("log")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("log(1) = %v, want 0.0", result[0])
	}
}

func TestMathLog2(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(8), NewWord("log2")})
	if result[0].AsNumber() != 3.0 {
		t.Errorf("log2(8) = %v, want 3.0", result[0])
	}
}

func TestMathLog10(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(100), NewWord("log10")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("log10(100) = %v, want 2.0", result[0])
	}
}

func TestMathCeil(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1.2), NewWord("ceil")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("ceil(1.2) = %v, want 2.0", result[0])
	}
}

func TestMathFloor(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1.8), NewWord("floor")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("floor(1.8) = %v, want 1.0", result[0])
	}
}

func TestMathRound(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1.5), NewWord("round")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("round(1.5) = %v, want 2.0", result[0])
	}
}

func TestMathTrunc(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(1.9), NewWord("trunc")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("trunc(1.9) = %v, want 1.0", result[0])
	}
}

func TestMathHypot(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(3), NewWord("hypot"), NewDecimal(4)})
	if result[0].AsNumber() != 5.0 {
		t.Errorf("hypot(3,4) = %v, want 5.0", result[0])
	}
}

func TestMathAbs(t *testing.T) {
	r, _ := DefaultRegistry()
	// Test integer abs
	result := runAQL(t, r, []Value{NewInteger(-5), NewWord("abs")})
	if result[0].AsNumber() != 5 {
		t.Errorf("abs(-5) = %v, want 5", result[0])
	}
	// Test decimal abs
	result = runAQL(t, r, []Value{NewDecimal(-3.5), NewWord("abs")})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("abs(-3.5) = %v, want 3.5", result[0])
	}
}

func TestMathSign(t *testing.T) {
	r, _ := DefaultRegistry()
	tests := []struct {
		input Value
		want  int64
	}{
		{NewInteger(5), 1},
		{NewInteger(-3), -1},
		{NewInteger(0), 0},
		{NewDecimal(2.5), 1},
		{NewDecimal(-1.5), -1},
		{NewDecimal(0.0), 0},
	}
	for _, tt := range tests {
		result := runAQL(t, r, []Value{tt.input, NewWord("sign")})
		if result[0].AsInteger() != tt.want {
			t.Errorf("sign(%v) = %v, want %d", tt.input, result[0], tt.want)
		}
	}
}

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

func TestMathNegateDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(3.5), NewWord("negate")})
	if result[0].AsNumber() != -3.5 {
		t.Errorf("negate(3.5) = %v, want -3.5", result[0])
	}
}

func TestMathMax(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(3), NewWord("max"), NewInteger(7)})
	if result[0].AsInteger() != 7 {
		t.Errorf("3 max 7 = %v, want 7", result[0])
	}
	result = runAQL(t, r, []Value{NewDecimal(3.5), NewWord("max"), NewDecimal(1.5)})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("3.5 max 1.5 = %v, want 3.5", result[0])
	}
}

func TestMathMin(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(3), NewWord("min"), NewInteger(7)})
	if result[0].AsInteger() != 3 {
		t.Errorf("3 min 7 = %v, want 3", result[0])
	}
	result = runAQL(t, r, []Value{NewDecimal(3.5), NewWord("min"), NewDecimal(1.5)})
	if result[0].AsNumber() != 1.5 {
		t.Errorf("3.5 min 1.5 = %v, want 1.5", result[0])
	}
}

func TestMathConstants(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewWord("math-pi")})
	if math.Abs(result[0].AsNumber()-math.Pi) > 0.0001 {
		t.Errorf("math-pi = %v, want %v", result[0], math.Pi)
	}
	result = runAQL(t, r, []Value{NewWord("math-e")})
	if math.Abs(result[0].AsNumber()-math.E) > 0.0001 {
		t.Errorf("math-e = %v, want %v", result[0], math.E)
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
