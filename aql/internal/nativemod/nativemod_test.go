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

// mathRegistry returns a registry with the aql:math module loaded via
// the standard ModuleDesc/installExports path (simulated by building
// the module and installing the "math" export as a def).
func mathRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
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
		// Use installDef via Register with the def word simulation.
		// We can just push the map onto DefStacks directly.
		r.DefStacks[name] = append(r.DefStacks[name], engine.NewMap(exportMap))
	}
	return r
}

// --- Resolve tests ---

func TestResolveKnownModule(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := Resolve("math", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := desc.Exports["math"]; !ok {
		t.Error("expected 'math' export in module descriptor")
	}
	// Check that the export map has sin
	mathExport := desc.Exports["math"]
	if _, ok := mathExport.Get("sin"); !ok {
		t.Error("expected 'sin' in math export map")
	}
}

func TestResolveUnknownModule(t *testing.T) {
	r, err := engine.DefaultRegistry()
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
		if n == "math" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'math' in Names()")
	}
}

// --- Math export map structure ---

func TestMathExportContainsAllWords(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildMathModule(r)
	if err != nil {
		t.Fatal(err)
	}
	mathExport := desc.Exports["math"]

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
	result := runAQL(t, r, []engine.Value{
		engine.NewInteger(-5),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("abs"),
		engine.NewWord(")"),
	})
	if result[0].AsInteger() != 5 {
		t.Errorf("math.abs(-5) = %v, want 5", result[0])
	}
}

func TestMathDotSin(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(0),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("sin"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 0.0 {
		t.Errorf("math.sin(0) = %v, want 0.0", result[0])
	}
}

func TestMathDotCos(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(0),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("cos"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("math.cos(0) = %v, want 1.0", result[0])
	}
}

func TestMathDotSqrt(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(4),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("sqrt"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("math.sqrt(4) = %v, want 2.0", result[0])
	}
}

func TestMathDotMin(t *testing.T) {
	r := mathRegistry(t)
	// 3 math.min 7 — but since FnDef takes both args from stack:
	// We need: 3 7 (math get min)
	result := runAQL(t, r, []engine.Value{
		engine.NewInteger(3), engine.NewInteger(7),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("min"),
		engine.NewWord(")"),
	})
	if result[0].AsInteger() != 3 {
		t.Errorf("math.min(3,7) = %v, want 3", result[0])
	}
}

func TestMathDotMax(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewInteger(3), engine.NewInteger(7),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("max"),
		engine.NewWord(")"),
	})
	if result[0].AsInteger() != 7 {
		t.Errorf("math.max(3,7) = %v, want 7", result[0])
	}
}

func TestMathDotPi(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("pi"),
		engine.NewWord(")"),
	})
	if math.Abs(result[0].AsNumber()-math.Pi) > 0.0001 {
		t.Errorf("math.pi = %v, want %v", result[0], math.Pi)
	}
}

func TestMathDotE(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("e"),
		engine.NewWord(")"),
	})
	if math.Abs(result[0].AsNumber()-math.E) > 0.0001 {
		t.Errorf("math.e = %v, want %v", result[0], math.E)
	}
}

func TestMathDotNegate(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewInteger(5),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("negate"),
		engine.NewWord(")"),
	})
	if result[0].AsInteger() != -5 {
		t.Errorf("math.negate(5) = %v, want -5", result[0])
	}
}

func TestMathDotCeil(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(1.2),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("ceil"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("math.ceil(1.2) = %v, want 2.0", result[0])
	}
}

func TestMathDotFloor(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(1.8),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("floor"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 1.0 {
		t.Errorf("math.floor(1.8) = %v, want 1.0", result[0])
	}
}

func TestMathDotRound(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewDecimal(1.5),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("round"),
		engine.NewWord(")"),
	})
	if result[0].AsNumber() != 2.0 {
		t.Errorf("math.round(1.5) = %v, want 2.0", result[0])
	}
}

func TestMathDotSign(t *testing.T) {
	r := mathRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewInteger(-7),
		engine.NewWord("("),
		engine.NewWord("math"), engine.NewWord("get"), engine.NewWord("sign"),
		engine.NewWord(")"),
	})
	if result[0].AsInteger() != -1 {
		t.Errorf("math.sign(-7) = %v, want -1", result[0])
	}
}
