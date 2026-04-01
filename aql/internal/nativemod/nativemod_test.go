package nativemod

import (
	"math"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// runAQL is a test helper that creates an engine and runs the given values.
func runAQL(t *testing.T, r *engine.Registry, input []engine.Value) []engine.Value {
	t.Helper()
	e := engine.New(r)
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

// mathRegistry returns a registry with the aql:math module loaded.
func mathRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterMath(r)
	return r
}

// --- Resolve tests ---

func TestResolveKnownModule(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := Resolve("math", r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify a math word is now available
	if r.Lookup("sin") == nil {
		t.Error("expected sin to be registered after Resolve(\"math\")")
	}
}

func TestResolveUnknownModule(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := Resolve("nonexistent", r); err == nil {
		t.Error("expected error for unknown module")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	found := false
	for _, n := range names {
		if n == "math" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'math' in Names()")
	}
}

// --- Math: abs ---

func TestMathAbsInteger(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(-5), engine.NewWord("abs")})
	if result[0].AsInteger() != 5 {
		t.Errorf("abs(-5) = %v, want 5", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewInteger(3), engine.NewWord("abs")})
	if result[0].AsInteger() != 3 {
		t.Errorf("abs(3) = %v, want 3", result[0])
	}
}

func TestMathAbsDecimal(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(-3.5), engine.NewWord("abs")})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("abs(-3.5) = %v, want 3.5", result[0])
	}
}

// --- Math: negate ---

func TestMathNegateInteger(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(5), engine.NewWord("negate")})
	if result[0].AsInteger() != -5 {
		t.Errorf("negate(5) = %v, want -5", result[0])
	}
}

func TestMathNegateDecimal(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(3.5), engine.NewWord("negate")})
	if result[0].AsNumber() != -3.5 {
		t.Errorf("negate(3.5) = %v, want -3.5", result[0])
	}
}

// --- Math: sign ---

func TestMathSign(t *testing.T) {
	r := mathRegistry(t)
	tests := []struct {
		input engine.Value
		want  int64
	}{
		{engine.NewInteger(5), 1},
		{engine.NewInteger(-3), -1},
		{engine.NewInteger(0), 0},
		{engine.NewDecimal(2.5), 1},
		{engine.NewDecimal(-1.5), -1},
		{engine.NewDecimal(0.0), 0},
	}
	for _, tt := range tests {
		result := runAQL(t, r, []engine.Value{tt.input, engine.NewWord("sign")})
		if result[0].AsInteger() != tt.want {
			t.Errorf("sign(%v) = %v, want %d", tt.input, result[0], tt.want)
		}
	}
}

// --- Math: min/max ---

func TestMathMin(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(3), engine.NewWord("min"), engine.NewInteger(7)})
	if result[0].AsInteger() != 3 {
		t.Errorf("3 min 7 = %v, want 3", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(3.5), engine.NewWord("min"), engine.NewDecimal(1.5)})
	if result[0].AsNumber() != 1.5 {
		t.Errorf("3.5 min 1.5 = %v, want 1.5", result[0])
	}
}

func TestMathMax(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(3), engine.NewWord("max"), engine.NewInteger(7)})
	if result[0].AsInteger() != 7 {
		t.Errorf("3 max 7 = %v, want 7", result[0])
	}
	result = runAQL(t, r, []engine.Value{engine.NewDecimal(3.5), engine.NewWord("max"), engine.NewDecimal(1.5)})
	if result[0].AsNumber() != 3.5 {
		t.Errorf("3.5 max 1.5 = %v, want 3.5", result[0])
	}
}

// --- Math: rounding ---

func TestMathCeil(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1.2), engine.NewWord("ceil")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("ceil(1.2) = %v, want 2.0", result[0])
	}
}

func TestMathFloor(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1.8), engine.NewWord("floor")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("floor(1.8) = %v, want 1.0", result[0])
	}
}

func TestMathRound(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1.5), engine.NewWord("round")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("round(1.5) = %v, want 2.0", result[0])
	}
}

func TestMathTrunc(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1.9), engine.NewWord("trunc")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("trunc(1.9) = %v, want 1.0", result[0])
	}
}

// --- Math: roots, exp/log ---

func TestMathSqrt(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(4), engine.NewWord("sqrt")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("sqrt(4) = %v, want 2.0", result[0])
	}
}

func TestMathCbrt(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(8), engine.NewWord("cbrt")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("cbrt(8) = %v, want 2.0", result[0])
	}
}

func TestMathExp(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("exp")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("exp(0) = %v, want 1.0", result[0])
	}
}

func TestMathLog(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1), engine.NewWord("log")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("log(1) = %v, want 0.0", result[0])
	}
}

func TestMathLog2(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(8), engine.NewWord("log2")})
	if result[0].AsNumber() != 3.0 {
		t.Errorf("log2(8) = %v, want 3.0", result[0])
	}
}

func TestMathLog10(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(100), engine.NewWord("log10")})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("log10(100) = %v, want 2.0", result[0])
	}
}

// --- Math: trigonometry ---

func TestMathSin(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("sin")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("sin(0) = %v, want 0.0", result[0])
	}
}

func TestMathSinInteger(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewInteger(0), engine.NewWord("sin")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathCos(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("cos")})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("cos(0) = %v, want 1.0", result[0])
	}
}

func TestMathTan(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("tan")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("tan(0) = %v, want 0.0", result[0])
	}
}

func TestMathAsin(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1), engine.NewWord("asin")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathAcos(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(1), engine.NewWord("acos")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("acos(1) = %v, want 0.0", result[0])
	}
}

func TestMathAtan(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("atan")})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("atan(0) = %v, want 0.0", result[0])
	}
}

func TestMathAtan2(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(0), engine.NewWord("atan2"), engine.NewDecimal(1)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMathHypot(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewDecimal(3), engine.NewWord("hypot"), engine.NewDecimal(4)})
	if result[0].AsNumber() != 5.0 {
		t.Errorf("hypot(3,4) = %v, want 5.0", result[0])
	}
}

// --- Math: constants ---

func TestMathConstants(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{engine.NewWord("math-pi")})
	if math.Abs(result[0].AsNumber()-math.Pi) > 0.0001 {
		t.Errorf("math-pi = %v, want %v", result[0], math.Pi)
	}
	result = runAQL(t, r, []engine.Value{engine.NewWord("math-e")})
	if math.Abs(result[0].AsNumber()-math.E) > 0.0001 {
		t.Errorf("math-e = %v, want %v", result[0], math.E)
	}
}

// --- Import integration test ---

func TestNativeModImportViaResolver(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.NativeModResolver = Resolve

	// Before import, sin should not exist
	if r.Lookup("sin") != nil {
		t.Fatal("sin should not be registered before import")
	}

	// Simulate "aql:math" import
	if err := Resolve("math", r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r.MarkNativeModLoaded("math")

	// After import, sin should exist
	if r.Lookup("sin") == nil {
		t.Error("sin should be registered after import")
	}

	// Loading again should be a no-op (tracked by the registry)
	if !r.IsNativeModLoaded("math") {
		t.Error("math should be marked as loaded")
	}
}
