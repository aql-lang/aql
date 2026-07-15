package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The two live miscompiles the Stage-J Run flip's full-tree sweep surfaced
// (2026-07-15) — each pinned compiled-vs-interpreted directly so the pins
// hold regardless of what Run routes to.

// A multi-overload user fn with an fn-PREDICATE-typed param slot must route
// through the user-poly runtime re-match: check-mode sigTypeMatches accepts
// predicate types LENIENTLY (RunPredicate short-circuits true), so a static
// arm commit baked `classify -3` to the Pos arm and raised signature_error
// where the interpreter's runtime predicate run falls through to the Any arm.
func TestPredicateOverloadDispatchCompiledParity(t *testing.T) {
	src := `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def classify fn [
  [x:Pos] [String] ["positive"]
  [x:Any] [String] ["other"]
]
classify 5
classify -3
classify "hi"`
	a := mustNew(t)
	gotC, compiled, errC := a.RunCompiled(src)
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	if fmt.Sprint(gotC) != "[positive other other]" {
		t.Errorf("compiled = %v, want [positive other other]", gotC)
	}
	b := mustNew(t)
	gotI, errI := b.RunInterp(src)
	if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("parity: compiled=%v interp=%v (err=%v)", gotC, gotI, errI)
	}

	// The hazardous sites ride CALL_USER_POLY; the String call has ONE
	// reachable arm (no hazard) and keeps the static CALL_USER commit.
	dis := compileDisasm(t, src)
	if strings.Count(dis, "CALL_USER_POLY") != 2 {
		t.Errorf("want the two Integer calls on CALL_USER_POLY:\n%s", dis)
	}
	if !strings.Contains(dis, "CALL_USER ") {
		t.Errorf("the single-reachable-arm String call must stay static:\n%s", dis)
	}

	// Negative: a SINGLE-overload predicate fn keeps the static unit — its
	// CALL_USER param guard re-validates at entry and raises exactly the
	// interpreter's no-match error.
	one := `def Pos fn [[n:Integer] [Boolean] [n gt 0]] def only fn [[x:Pos] [String] ["p"]] only -3`
	c := mustNew(t)
	_, _, errOC := c.RunCompiled(one)
	d := mustNew(t)
	_, errOI := d.RunInterp(one)
	if codeOf(errOC) != codeOf(errOI) || codeOf(errOC) == "" {
		t.Errorf("single-overload predicate error parity: compiled=[%s]%v interp=[%s]%v",
			codeOf(errOC), errOC, codeOf(errOI), errOI)
	}
	if oneDis := compileDisasm(t, one); strings.Contains(oneDis, "CALL_USER_POLY") {
		t.Errorf("single-overload predicate fn must not go poly:\n%s", oneDis)
	}

	// When the poly bake DECLINES (a zero-return overload set fails the
	// poly gate's committed-returns requirement), the hazard refuses the
	// program — slow, not wrong — and the interpreter keeps parity.
	declining := `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def shout fn [
  [x:Pos] [] ["p" print]
  [x:Any] [] ["o" print]
]
shout -3`
	e := mustNew(t)
	prog, reason, _, cerr := e.CompileCheck(declining)
	if cerr != nil || prog != nil ||
		!strings.Contains(reason, "fn-predicate-typed overload dispatch at `shout`") {
		t.Errorf("declining poly bake must refuse with the hazard reason: prog=%v reason=%q err=%v",
			prog != nil, reason, cerr)
	}
	f := mustNew(t)
	if _, ierr := f.RunInterp(declining); ierr != nil {
		t.Errorf("the refused program must interpret cleanly: %v", ierr)
	}
}

// Two Xml element literals are two INSTANCES (`eq` is container identity):
// the const pool must never canon-merge identity-bearing payloads — the
// merge pushed ONE instance twice and `(<a/>) eq (<a/>)` answered true.
func TestXmlLiteralIdentityCompiledParity(t *testing.T) {
	a := mustNew(t)
	gotC, compiled, errC := a.RunCompiled(`(<a/>) eq (<a/>)`)
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	if fmt.Sprint(gotC) != "[false]" {
		t.Errorf("compiled identity eq = %v, want [false]", gotC)
	}
	// Positive twin: ONE instance read twice through its binding IS eq to
	// itself — and the def-of-Xml-literal write-back (the stripped-literal
	// rescue in lowerDynBind) keeps the program compiling.
	b := mustNew(t)
	gotS, compiledS, errS := b.RunCompiled(`def x <a/> x eq x`)
	if !compiledS || errS != nil {
		t.Fatalf("self-eq run: compiled=%v err=%v", compiledS, errS)
	}
	if fmt.Sprint(gotS) != "[true]" {
		t.Errorf("self identity eq = %v, want [true]", gotS)
	}
	// Structural deq stays value-based across distinct instances.
	c := mustNew(t)
	gotD, _, errD := c.RunCompiled(`(<a x="1"><b/></a>) deq (<a x="1"><b/></a>)`)
	if errD != nil || fmt.Sprint(gotD) != "[true]" {
		t.Errorf("structural deq = %v (err=%v), want [true]", gotD, errD)
	}
}
