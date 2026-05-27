package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

func randRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallRandExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func runRandAQL(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	e := native.NewTop(r)
	result, err := e.Run(values)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	return result
}

func TestRandModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildRandModule(r)
	if err != nil {
		t.Fatal(err)
	}
	randExport, ok := desc.Exports["rand"]
	if !ok {
		t.Fatal("expected 'rand' export")
	}
	for _, name := range []string{"seed", "int", "bool", "float", "string", "one-of"} {
		if _, ok := randExport.Get(name); !ok {
			t.Errorf("missing export: %q", name)
		}
	}
}

func TestRandIntDeterministicWithSeed(t *testing.T) {
	r := randRegistry(t)
	// Parens delimit each draw so forward-collection cannot reach
	// into the next call's tokens.
	src := `42 rand.seed (0 100 rand.int) (0 100 rand.int) (0 100 rand.int)`
	res := runRandAQL(t, r, src)
	if len(res) != 3 {
		t.Fatalf("expected 3 values on stack, got %d (%v)", len(res), res)
	}
	var got [3]int64
	for i := 0; i < 3; i++ {
		n, err := res[i].AsConcreteInteger()
		if err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
		got[i] = n
		if n < 0 || n > 100 {
			t.Errorf("draw %d out of range: %d", i, n)
		}
	}
	// Same seed in a fresh registry must produce the same sequence.
	r2 := randRegistry(t)
	res2 := runRandAQL(t, r2, src)
	for i := 0; i < 3; i++ {
		n, _ := res2[i].AsConcreteInteger()
		if n != got[i] {
			t.Errorf("draw %d not deterministic: first=%d second=%d", i, got[i], n)
		}
	}
}

func TestRandIntRejectsInvertedBounds(t *testing.T) {
	r := randRegistry(t)
	// Stack form `lo hi rand.int` puts hi on top. To exercise hi<lo,
	// push hi=0 then lo=10.
	values, _ := parser.Parse(`10 0 rand.int`)
	e := native.NewTop(r)
	_, err := e.Run(values)
	if err == nil {
		t.Fatal("expected error for hi<lo")
	}
}

func TestRandBool(t *testing.T) {
	r := randRegistry(t)
	// Just check that 20 draws produce both true and false at seed 1.
	src := `1 rand.seed`
	for i := 0; i < 20; i++ {
		src += "  rand.bool"
	}
	res := runRandAQL(t, r, src)
	if len(res) != 20 {
		t.Fatalf("expected 20 bools, got %d", len(res))
	}
	sawT, sawF := false, false
	for _, v := range res {
		b, err := v.AsConcreteBoolean()
		if err != nil {
			t.Fatalf("not a boolean: %v", err)
		}
		if b {
			sawT = true
		} else {
			sawF = true
		}
	}
	if !sawT || !sawF {
		t.Errorf("expected both true and false in 20 draws (seen true=%v false=%v)", sawT, sawF)
	}
}

func TestRandString(t *testing.T) {
	r := randRegistry(t)
	res := runRandAQL(t, r, `1 rand.seed  "abc" 10 rand.string`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	s, err := res[0].AsConcreteString()
	if err != nil {
		t.Fatalf("not a string: %v", err)
	}
	if len(s) != 10 {
		t.Errorf("len=%d, want 10", len(s))
	}
	for _, ch := range s {
		if ch != 'a' && ch != 'b' && ch != 'c' {
			t.Errorf("char %q not in charset", ch)
		}
	}
}

func TestRandStringEmptyCharsetZeroLen(t *testing.T) {
	r := randRegistry(t)
	res := runRandAQL(t, r, `"" 0 rand.string`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	s, _ := res[0].AsConcreteString()
	if s != "" {
		t.Errorf("got %q, want empty", s)
	}
}

func TestRandOneOfSingleCall(t *testing.T) {
	r := randRegistry(t)
	// A single rand.one-of call dispatches cleanly. The multi-call
	// case (running many one-of's in sequence) hits forward-collection
	// ambiguity between adjacent statements and is intentionally left
	// to higher-level PBT loops in the test framework.
	res := runRandAQL(t, r, `7 rand.seed [10 20 30] rand.one-of`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	n, err := res[0].AsConcreteInteger()
	if err != nil {
		t.Fatalf("not an integer: %v", err)
	}
	if n != 10 && n != 20 && n != 30 {
		t.Errorf("draw %d not in the source list", n)
	}
}

func TestRandFloatInUnitInterval(t *testing.T) {
	r := randRegistry(t)
	src := `1 rand.seed`
	for i := 0; i < 50; i++ {
		src += "  rand.float"
	}
	res := runRandAQL(t, r, src)
	for _, v := range res {
		f, err := v.AsConcreteDecimal()
		if err != nil {
			t.Fatalf("not a decimal: %v", err)
		}
		if f < 0 || f >= 1 {
			t.Errorf("float out of [0,1): %g", f)
		}
	}
}
