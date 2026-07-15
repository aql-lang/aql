package lang_test

import (
	"fmt"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestPublicDefineTypeAPI exercises the embedding-API convergence: a host
// defines types three ways — from AQL source (DefineTypeFromSource), from
// a Go membership func (DefineMemberType), and as a union (DefineEnum) —
// then both AQL source AND a host-registered signature use those types,
// all on one lang.AQL instance. This is the external counterpart of `def`:
// types defined from Go are first-class and interchangeable with
// source-defined ones.
func TestPublicDefineTypeAPI(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}

	// (1) From AQL source — the host writes the body exactly as a script
	// would, and gets the *Type back.
	point, err := a.DefineTypeFromSource("Point", "refine Record [x:Integer y:Integer]")
	if err != nil {
		t.Fatalf("DefineTypeFromSource: %v", err)
	}
	if point == nil {
		t.Fatal("Point handle is nil")
	}

	// (2) From a Go predicate — the one shape source can't express here.
	even, err := a.DefineMemberType("Even", lang.TInteger, func(v lang.Value) bool {
		n, e := v.AsConcreteInteger()
		return e == nil && n%2 == 0
	})
	if err != nil {
		t.Fatalf("DefineMemberType: %v", err)
	}

	// (3) As a union.
	if _, err := a.DefineEnum("NumOrStr",
		lang.NewTypeLiteral(lang.TInteger), lang.NewTypeLiteral(lang.TString)); err != nil {
		t.Fatalf("DefineEnum: %v", err)
	}

	// All three are bound and referenceable from AQL source on this
	// instance — just like `def`-installed types.
	out, err := a.RunInterp(`[(4 is Even) (5 is Even) (3 is NumOrStr) ('x' is NumOrStr) (true is NumOrStr)]`)
	if err != nil {
		t.Fatalf("Run membership: %v", err)
	}
	if got := fmt.Sprint(out); got != "[[true false true true false]]" {
		t.Errorf("membership in source = %s, want [[true false true true false]]", got)
	}

	// The source-defined Point is usable from source too.
	if _, err := a.RunInterp(`make Point {x:1 y:2}`); err != nil {
		t.Errorf("make Point: %v", err)
	}

	// A HOST-registered signature can use a Go-defined type handle. `only-even`
	// accepts an Even and returns it; the type constraint is enforced.
	a.Register("only-even", lang.Signature{
		Args: []*lang.Type{even},
		Impl: native.Go(func(args []lang.Value, _ map[string]lang.Value, _ []lang.Value, _ *native.Registry) ([]lang.Value, error) {
			return []lang.Value{args[0]}, nil
		}), BarrierPos: -1,
	})
	if out, err := a.RunInterp(`only-even 8`); err != nil {
		t.Errorf("only-even 8: %v", err)
	} else if got := fmt.Sprint(out); got != "[8]" {
		t.Errorf("only-even 8 = %s, want [8]", got)
	}
	// Negative: an odd number does not satisfy the Even parameter, so the
	// call does not dispatch — the type is a real constraint.
	if _, err := a.RunInterp(`only-even 7`); err == nil {
		t.Error("only-even 7 should fail: 7 is not Even")
	}
}

// TestPublicDefineTypeRejects confirms the public API surfaces the same
// validation as `def`: a lowercase type name is rejected, and a bad
// source body returns the parse/run error rather than a half-built type.
func TestPublicDefineTypeRejects(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.DefineMemberType("lowercase", lang.TInteger, func(lang.Value) bool { return true }); err == nil {
		t.Error("DefineMemberType accepted a lowercase type name")
	}
	if _, err := a.DefineTypeFromSource("Broken", "refine ["); err == nil {
		t.Error("DefineTypeFromSource accepted a malformed body")
	}
}
