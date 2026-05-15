package test

import (
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/eng"
	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
)

// Compare-dispatch tests covering the three layers wired through
// eng.CompareValues:
//
//  1. CORE — kernel scalar Comparer behaviors (Number, String,
//     Boolean, Atom). These live on the *Type's Behavior and are
//     consulted via the lattice walk in CompareValues, so Integer
//     and Decimal both resolve to the Number Comparer through their
//     shared ancestor.
//
//  2. CUSTOM NATIVE — domain types (Date, DateTime, TimeOfDay)
//     attach Compare methods to their existing Format Behaviors in
//     native_temporal.go. CompareValues dispatches directly to the
//     type's Behavior — no kernel switch ladder.
//
//  3. USER-DEFINED — the `cmp` word installs an AQL body on a
//     user-defined type's Behavior. The body receives the operands
//     via the `a` and `b` defs and produces an Integer; the kernel
//     normalises to -1/0/1 and propagates body-evaluation errors.

// ─── 1. CORE: kernel scalar comparisons ──────────────────────────────

func TestCompareDispatchCoreIntegerInteger(t *testing.T) {
	got, err := eng.CompareValues(eng.NewInteger(2), eng.NewInteger(5))
	if err != nil || got != -1 {
		t.Errorf("Integer-Integer: got (%d, %v), want (-1, nil)", got, err)
	}
}

func TestCompareDispatchCoreDecimalDecimal(t *testing.T) {
	got, err := eng.CompareValues(eng.NewDecimal(3.14), eng.NewDecimal(2.71))
	if err != nil || got != 1 {
		t.Errorf("Decimal-Decimal: got (%d, %v), want (1, nil)", got, err)
	}
}

// Integer + Decimal walks up to Number for the comparison: the
// lattice walk finds numberCompareBehavior on TNumber. Without the
// LCA logic this would error (no per-pair Behavior on Integer or
// Decimal alone).
func TestCompareDispatchCoreIntegerVsDecimalLCA(t *testing.T) {
	got, err := eng.CompareValues(eng.NewInteger(3), eng.NewDecimal(3.5))
	if err != nil || got != -1 {
		t.Errorf("Integer < Decimal via LCA=Number: got (%d, %v), want (-1, nil)", got, err)
	}
	got, err = eng.CompareValues(eng.NewDecimal(3.0), eng.NewInteger(3))
	if err != nil || got != 0 {
		t.Errorf("Integer == Decimal via LCA=Number: got (%d, %v), want (0, nil)", got, err)
	}
}

func TestCompareDispatchCoreString(t *testing.T) {
	got, err := eng.CompareValues(eng.NewString("apple"), eng.NewString("banana"))
	if err != nil || got != -1 {
		t.Errorf("String lex: got (%d, %v), want (-1, nil)", got, err)
	}
}

func TestCompareDispatchCoreBoolean(t *testing.T) {
	// false < true
	got, err := eng.CompareValues(eng.NewBoolean(false), eng.NewBoolean(true))
	if err != nil || got != -1 {
		t.Errorf("Boolean false<true: got (%d, %v), want (-1, nil)", got, err)
	}
}

func TestCompareDispatchCoreAtom(t *testing.T) {
	got, err := eng.CompareValues(eng.NewAtom("alpha"), eng.NewAtom("beta"))
	if err != nil || got != -1 {
		t.Errorf("Atom lex: got (%d, %v), want (-1, nil)", got, err)
	}
}

// Disjoint scalar branches (Integer vs String) have no shared
// ancestor with a Comparer below the root, so the dispatch errors.
func TestCompareDispatchCoreCrossBranchError(t *testing.T) {
	_, err := eng.CompareValues(eng.NewInteger(1), eng.NewString("a"))
	if err == nil {
		t.Errorf("Integer vs String: expected error, got nil")
	}
}

// Lists / maps have no Comparer in their lattice; CompareValues
// surfaces a clean "cannot compare" error rather than a panic.
func TestCompareDispatchCoreListError(t *testing.T) {
	a := eng.NewList([]eng.Value{eng.NewInteger(1)})
	b := eng.NewList([]eng.Value{eng.NewInteger(2)})
	_, err := eng.CompareValues(a, b)
	if err == nil {
		t.Errorf("List vs List: expected error, got nil")
	}
}

// ─── 2. CUSTOM NATIVE: Date / DateTime / TimeOfDay ───────────────────

func TestCompareDispatchNativeDate(t *testing.T) {
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))

	if got, err := eng.CompareValues(d1, d2); err != nil || got != -1 {
		t.Errorf("Date d1 < d2: got (%d, %v), want (-1, nil)", got, err)
	}
	if got, err := eng.CompareValues(d2, d1); err != nil || got != 1 {
		t.Errorf("Date d2 > d1: got (%d, %v), want (1, nil)", got, err)
	}
	if got, err := eng.CompareValues(d1, d1); err != nil || got != 0 {
		t.Errorf("Date d1 == d1: got (%d, %v), want (0, nil)", got, err)
	}
}

func TestCompareDispatchNativeDateTime(t *testing.T) {
	dt1 := engine.NewDateTime(time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC))
	dt2 := engine.NewDateTime(time.Date(2024, 1, 1, 17, 30, 0, 0, time.UTC))

	if got, err := eng.CompareValues(dt1, dt2); err != nil || got != -1 {
		t.Errorf("DateTime morning<evening: got (%d, %v), want (-1, nil)", got, err)
	}
}

// Date vs DateTime: both descend from Scalar/Time but neither is an
// ancestor of the other. LCA = Scalar/Time which has no Comparer,
// so this errors — preserving "different temporal granularities are
// not directly orderable".
func TestCompareDispatchNativeDateVsDateTimeError(t *testing.T) {
	d := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	dt := engine.NewDateTime(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC))
	_, err := eng.CompareValues(d, dt)
	if err == nil {
		t.Errorf("Date vs DateTime: expected error (no Comparer on shared ancestor)")
	}
}

// Verify the `lt` word picks up the native Comparer via
// CompareValues (end-to-end through the engine, not just the
// Go-level CompareValues call).
func TestCompareDispatchNativeDateLtWord(t *testing.T) {
	r := freshCmpRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))

	out, err := engine.NewTop(r).Run([]eng.Value{d1, d2, eng.NewWord("lt")})
	if err != nil {
		t.Fatalf("lt date date: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected single result, got %d", len(out))
	}
	got, err := eng.AsBoolean(out[0])
	if err != nil || got != true {
		t.Errorf("Jan < June: got %v (err %v), want true", got, err)
	}
}

// ─── 3. USER-DEFINED: cmp word on user types ─────────────────────────

// User types created via `type Foo object {…}` mint a fresh *Type
// whose Behavior we can target with `cmp`. Two values produced via
// `make Foo {…}` then compare through the installed body.
func TestCompareDispatchUserObjectType(t *testing.T) {
	r := freshCmpRegistry(t)
	setup := strings.Join([]string{
		// Define a Person object type with a numeric `age` field.
		"type Person object {age:Integer}",
		// Install a comparator that compares by age. `a` and `b` are
		// pushed as defs by the Comparer wrapper; the trailing
		// Integer on the stack is the result.
		"cmp Person/q [(a 'age' get) (b 'age' get) sub]",
	}, " ")
	if _, err := engine.NewTop(r).Run(parseSrc(t, setup)); err != nil {
		t.Fatalf("setup: %v", err)
	}

	alice := mustEval(t, r, "make Person {age:30}")
	bob := mustEval(t, r, "make Person {age:25}")

	got, err := eng.CompareValues(alice, bob)
	if err != nil || got != 1 {
		t.Errorf("alice(30) vs bob(25): got (%d, %v), want (1, nil)", got, err)
	}
	got, err = eng.CompareValues(bob, alice)
	if err != nil || got != -1 {
		t.Errorf("bob(25) vs alice(30): got (%d, %v), want (-1, nil)", got, err)
	}
}

// End-to-end: the `lt` word sees the user comparator.
func TestCompareDispatchUserObjectTypeLtWord(t *testing.T) {
	r := freshCmpRegistry(t)
	setup := strings.Join([]string{
		"type Person object {age:Integer}",
		"cmp Person/q [(a 'age' get) (b 'age' get) sub]",
		"def alice make Person {age:30}",
		"def bob make Person {age:25}",
	}, " ")
	if _, err := engine.NewTop(r).Run(parseSrc(t, setup)); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out, err := engine.NewTop(r).Run(parseSrc(t, "alice bob lt"))
	if err != nil {
		t.Fatalf("lt: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("lt produced %d results", len(out))
	}
	gotBool, _ := eng.AsBoolean(out[0])
	if gotBool != false {
		t.Errorf("alice(30) lt bob(25): want false (30 > 25), got true")
	}
}

// A second user comparator can override the first via re-`cmp`.
func TestCompareDispatchUserOverride(t *testing.T) {
	r := freshCmpRegistry(t)
	src := strings.Join([]string{
		"type Item object {n:Integer}",
		// Ascending by n.
		"cmp Item/q [(a 'n' get) (b 'n' get) sub]",
	}, " ")
	if _, err := engine.NewTop(r).Run(parseSrc(t, src)); err != nil {
		t.Fatalf("setup: %v", err)
	}

	i1 := mustEval(t, r, "make Item {n:1}")
	i9 := mustEval(t, r, "make Item {n:9}")

	got, _ := eng.CompareValues(i1, i9)
	if got != -1 {
		t.Errorf("ascending Item(1) < Item(9): got %d, want -1", got)
	}

	// Now install the inverse — descending by n.
	if _, err := engine.NewTop(r).Run(parseSrc(t,
		"cmp Item/q [(b 'n' get) (a 'n' get) sub]")); err != nil {
		t.Fatalf("override: %v", err)
	}

	got, _ = eng.CompareValues(i1, i9)
	if got != 1 {
		t.Errorf("descending Item(1) > Item(9): got %d, want 1", got)
	}
}

// `cmp` rejects unknown type names.
func TestCompareDispatchUserRejectsUnknownType(t *testing.T) {
	r := freshCmpRegistry(t)
	_, err := engine.NewTop(r).Run(parseSrc(t, "cmp Nope/q [a b sub]"))
	if err == nil {
		t.Errorf("cmp Nope: expected error for unknown type")
	}
}

// `cmp` refuses to override comparators on kernel-declared builtin
// types — Integer / String / etc. keep their canonical ordering.
func TestCompareDispatchUserRejectsBuiltin(t *testing.T) {
	r := freshCmpRegistry(t)
	_, err := engine.NewTop(r).Run(parseSrc(t, "cmp Integer/q [a b sub]"))
	if err == nil {
		t.Errorf("cmp Integer: expected error for builtin type override")
	}
}

// A user comparator whose body errors should surface that error
// through the Comparer return chain, not panic or silently produce 0.
func TestCompareDispatchUserBodyError(t *testing.T) {
	r := freshCmpRegistry(t)
	src := strings.Join([]string{
		"type Faulty object {x:Integer}",
		// `quux` is not a registered word — running the body errors.
		"cmp Faulty/q [a b quux]",
	}, " ")
	if _, err := engine.NewTop(r).Run(parseSrc(t, src)); err != nil {
		t.Fatalf("setup: %v", err)
	}

	v1 := mustEval(t, r, "make Faulty {x:1}")
	v2 := mustEval(t, r, "make Faulty {x:2}")

	_, err := eng.CompareValues(v1, v2)
	if err == nil {
		t.Errorf("body error: expected error, got nil")
	}
}

// ─── helpers ────────────────────────────────────────────────────────

func freshCmpRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	return r
}

func parseSrc(t *testing.T, src string) []eng.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return vals
}

func mustEval(t *testing.T, r *engine.Registry, src string) eng.Value {
	t.Helper()
	out, err := engine.NewTop(r).Run(parseSrc(t, src))
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	if len(out) == 0 {
		t.Fatalf("eval %q: no result", src)
	}
	return out[len(out)-1]
}
