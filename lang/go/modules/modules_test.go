package modules

import (
	"math"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// runBORU is a test helper that creates an engine and runs the given values.
func runBORU(t *testing.T, r *native.Registry, input []native.Value) []native.Value {
	t.Helper()
	e := native.New(r)
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

// mathRegistry returns a registry with the boru:math module loaded via
// the standard ModuleDesc/installExports path (simulated by building
// the module and installing the "math" export as a def).
func mathRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Build the module descriptor.
	desc, err := BuildMathModule(r)
	if err != nil {
		t.Fatal(err)
	}
	// Install exports as defs — same as the import handler does.
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return r
}

// --- Resolve tests ---

func TestResolveKnownModule(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := Resolve("math-util", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := desc.Exports["MathUtil"]; !ok {
		t.Error("expected 'math' export in module descriptor")
	}
	// Check that the export map has sin
	mathExport := desc.Exports["MathUtil"]
	if _, ok := mathExport.Get("sin"); !ok {
		t.Error("expected 'sin' in math export map")
	}
}

func TestResolveUnknownModule(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve("nonexistent", r); err == nil {
		t.Error("expected error for unknown module")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	found := false
	for _, n := range names {
		if n == "math-util" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'math' in Names()")
	}
}

// --- Math export map structure ---

func TestMathExportContainsAllWords(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildMathModule(r)
	if err != nil {
		t.Fatal(err)
	}
	mathExport := desc.Exports["MathUtil"]

	expected := []string{
		"abs", "negate", "sign", "min", "max",
		"ceil", "floor", "round", "trunc",
		"sqrt", "cbrt", "exp", "log", "log2", "log10",
		"sin", "cos", "tan", "asin", "acos", "atan", "atan2", "hypot",
		"pi", "e",
	}
	for _, name := range expected {
		if _, ok := mathExport.Get(name); !ok {
			t.Errorf("expected %q in math export map", name)
		}
	}
}

// --- Math word tests via dot notation ---
// These test that the FnDef wrappers in the export map work correctly
// when the "math" def is accessed.

func TestMathDotAbs(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewInteger(-5),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("abs"),
		native.NewCloseParen(),
	})
	v, _ := native.AsInteger(result[0])
	if v != 5 {
		t.Errorf("MathUtil.abs(-5) = %v, want 5", result[0])
	}
}

func TestMathDotSin(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(0),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("sin"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 0.0 {
		t.Errorf("MathUtil.sin(0) = %v, want 0.0", result[0])
	}
}

func TestMathDotCos(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(0),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("cos"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 1.0 {
		t.Errorf("MathUtil.cos(0) = %v, want 1.0", result[0])
	}
}

func TestMathDotSqrt(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(4),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("sqrt"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 2.0 {
		t.Errorf("MathUtil.sqrt(4) = %v, want 2.0", result[0])
	}
}

func TestMathDotMin(t *testing.T) {
	r := mathRegistry(t)
	// 3 MathUtil.min 7 — but since FnDef takes both args from stack:
	// We need: 3 7 (math get min)
	result := runBORU(t, r, []native.Value{
		native.NewInteger(3), native.NewInteger(7),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("min"),
		native.NewCloseParen(),
	})
	v, _ := native.AsInteger(result[0])
	if v != 3 {
		t.Errorf("MathUtil.min(3,7) = %v, want 3", result[0])
	}
}

func TestMathDotMax(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewInteger(3), native.NewInteger(7),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("max"),
		native.NewCloseParen(),
	})
	v, _ := native.AsInteger(result[0])
	if v != 7 {
		t.Errorf("MathUtil.max(3,7) = %v, want 7", result[0])
	}
}

func TestMathDotPi(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("pi"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if math.Abs(v-math.Pi) > 0.0001 {
		t.Errorf("MathUtil.pi = %v, want %v", result[0], math.Pi)
	}
}

func TestMathDotE(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("e"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if math.Abs(v-math.E) > 0.0001 {
		t.Errorf("MathUtil.e = %v, want %v", result[0], math.E)
	}
}

func TestMathDotNegate(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewInteger(5),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("negate"),
		native.NewCloseParen(),
	})
	v, _ := native.AsInteger(result[0])
	if v != -5 {
		t.Errorf("MathUtil.negate(5) = %v, want -5", result[0])
	}
}

func TestMathDotCeil(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(1.2),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("ceil"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 2.0 {
		t.Errorf("MathUtil.ceil(1.2) = %v, want 2.0", result[0])
	}
}

func TestMathDotFloor(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(1.8),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("floor"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 1.0 {
		t.Errorf("MathUtil.floor(1.8) = %v, want 1.0", result[0])
	}
}

func TestMathDotRound(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewFloat(1.5),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("round"),
		native.NewCloseParen(),
	})
	v, _ := native.AsNumber(result[0])
	if v != 2.0 {
		t.Errorf("MathUtil.round(1.5) = %v, want 2.0", result[0])
	}
}

func TestMathDotSign(t *testing.T) {
	r := mathRegistry(t)
	result := runBORU(t, r, []native.Value{
		native.NewInteger(-7),
		native.NewOpenParen(),
		native.NewWord("MathUtil"), native.NewWord("dot"), native.NewWord("sign"),
		native.NewCloseParen(),
	})
	v, _ := native.AsInteger(result[0])
	if v != -1 {
		t.Errorf("MathUtil.sign(-7) = %v, want -1", result[0])
	}
}
