package lang

// NUR123 (measured 2026-09-05). A bare read of a frame binding holding a fn
// is a WORD dispatch on the interpreter — stepWord routes a bound FnDefInfo
// through Registry.Lookup under the binding name: a 0-arg fn fires, an n-arg
// fn collects from the tokens after it and the frame's stack below it, a
// no-match raises `cannot call `g`` — while the check model bound a CARRIER
// the emitter lowered as a slot push: `def f fn [[g:Function][Any][g]]  f
// ([] => [42])` answered `fn` for the interpreter's 42, and the identity fn
// over a 0-arg lambda the same, both on the DEFAULT lane with exit 0. The
// engine now notes each such read (NoteWordRead), the unit's finish arms the
// whole-frame replay over the residual with the binding names, and the VM
// re-steps a fn-valued read as the WORD through the interpreter's own
// dispatch (CompiledFn.DynFrameWords). A fn-typed read the replay cannot
// seat refuses; a gradual one keeps the slot push it always had.

import (
	"fmt"
	"strings"
	"testing"
)

// requireParityHead is requireParity on the error's first line: the
// interpreter's real frame carries the body's DefCleanup marker, which its
// no-match NOTES list as a stray argument (`… and __dc (a __DC)`), while the
// island region re-stepped by the replay holds only the residual — the
// code and message agree, the notes name a marker only one side has.
func requireParityHead(t *testing.T, src string, gotC []any, errC error, gotI []any, errI error) {
	t.Helper()
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || firstErrLine(errC) != firstErrLine(errI) {
		t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}

const wrF = `def f fn [[g:Function][Any]`

// TestWordReadDispatchParity pins every measured shape that now agrees on
// both lanes, value and error text alike (positions are NUR118's).
func TestWordReadDispatchParity(t *testing.T) {
	rows := []struct{ src, want string }{
		{wrF + `[g]]  f ([] => [42])`, "42 — was fn"},
		{wrF + `[g]]  f (z:Integer => [z])`, "cannot call `g` — was fn (Integer)"},
		{wrF + `[g]]  f add/v`, "cannot call `g` — was fn add(…)"},
		{wrF + `[(g)]]  f ([] => [42])`, "42 — the paren's word dispatches"},
		{wrF + `[(g)]]  f (z:Integer => [z])`, "cannot call `g`"},
		{`def f fn [[g:Function][Any][g] [g:Function][Any][g]]  f ([] => [42])`, "42 — two overloads"},
		{wrF + `[def y 1  g]]  f ([] => [42])`, "42 — a def before the read is not after it"},
		{`def f fn [[g:Function][Integer][g]]  f ([] => [42])`, "42 — the dispatch's result meets the declared type"},
		{wrF + `[g]]  f ([] => [42]) print`, "42"},
		{`def id fn [[x:Any][Any][x]]  id ([] => [42])`, "42 — the gradual identity over a fn"},
		{`def id fn [[x:Any][Any][x]]  id 5`, "5 — and over plain data, no island"},
		{`def mk fn [[] [Function] [([] => [42])]]  def f fn [[g:Function][Any][g]]  f (mk)`, "42 — a returned closure"},
		{wrF + `[g]]  def g0 ([] => [42])  f g0/v`, "42 — a named 0-arg lambda"},
		{wrF + `[g]]  f (quote ([] => [42]))`, "42 — a quoted fn still dispatches as the binding"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:Integer => [mul 3 z]) 5`, "15 — the word collects the frame's stack"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:String => [z]) 5`, "cannot call `g`"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f (z:String => [z]) 5`, "cannot call `g` — was the count error over [fn 5] (NUR122's park)"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f ([] => [42]) 5`, "f: expected 1, got 2 — [42 5], the 0-arg fn fires (was [fn 5])"},
		{wrF + `[g 3]]  f (z:Integer => [mul 3 z])`, "9"},
		{wrF + `[g 3]]  f ([] => [42])`, "f: expected 1, got 2 — [42 3]"},
		{wrF + `[g 3]]  f (z:String => [z])`, "cannot call `g`"},
		{wrF + `[do [g]]]  f ([] => [42])`, "42 — the do body islands"},
		// a compiled CLOSURE reaching the binding: the bridge dispatches it by
		// name under the lambda's OWN declared params (lamParamContract), so a
		// typed lambda no-matches exactly as the interpreter's frame binding
		{`def mk fn [[k:Integer][Function][([] => [k])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "[3] — was [fn]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g 2]]  [(mk 3)] each [f]`, "[6]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g "s"]]  [(mk 3)] each [f]`, "cannot call `g` — was [0] (the closure ran over the String)"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "cannot call `g`"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 2]]  [(mk 3)] each [f]`, "cannot call `g` — was [3]"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 "b"]]  [(mk 3)] each [f]`, "[3]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function x:Integer][Any][x g]]  [(mk 3)] each [f 5]`, "[15] — the closure bridged, collecting the frame's x"},
		// value deliveries stay values: a Function-expecting forward, `/v`
		{`def h fn [[k:Function][Any][typeof k/v]]  def f fn [[g:Function][Any][h g]]  f ([] => [42])`, "Function — delivered, not dispatched"},
		{wrF + `[g/v]]  f ([] => [42]) typeof`, "Function"},
		{`[1 2] each ([g:Function] => [g])`, "[fn (Function) fn (Function)] — an unmatched lambda stays data"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParityHead(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestWordReadDispatchRefuses pins the reads the replay cannot seat: a
// container member, a branch residual, a stack-collected argument, a read
// mixed with a `/v` read of the same binding — each a sound fallback that
// answers the interpreter's value.
func TestWordReadDispatchRefuses(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{wrF + `[[g]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[{a: g}]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[if true [g] [0]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Function][Any][k/v]]  def f fn [[g:Function][Any][g h]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[g drop  g/v]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g/v drop  g]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g typeof]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the read has no faithful lowering here", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}
