package eng

import (
	"strings"
	"testing"
)

// pendingList / pendingMap build parser-shaped pending containers
// (Eval=true, unquoted) — the form a source-literal list/map token has
// when it parks as a frame residual.
func pendingList(elems ...Value) Value {
	v := NewList(elems)
	v.Eval = true
	return v
}

func pendingMap(k string, val Value) Value {
	m := NewOrderedMap()
	m.Set(k, val)
	v := NewMap(m)
	v.Eval = true
	return v
}

// TestStepDefCleanupResidualArms drives the marker's residual scan
// directly (the in-package seam): the EvalResidual gate, the
// single-literal off-state, and both container error arms.
func TestStepDefCleanupResidualArms(t *testing.T) {
	r := runReg(t)

	failList := pendingList(NewWord("cfail"), NewInteger(1))
	failMap := pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}}))
	okMap := pendingMap("a", NewInteger(3))

	mk := func(evalResidual bool, residual ...Value) (*Engine, Value, int) {
		e := New(r)
		toks := append([]Value{NewOpenParen()}, residual...)
		marker := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: evalResidual})
		toks = append(toks, marker)
		e.tape = NewTape(toks, stackHeadroom)
		return e, marker, len(toks) - 1
	}

	// Off-state: a pending container is left untouched (the transparency).
	e, marker, idx := mk(false, okMap)
	if err := e.stepDefCleanup(marker, idx); err != nil {
		t.Fatalf("off-state cleanup: %v", err)
	}
	if !e.tape.At(1).Eval {
		t.Fatal("EvalResidual=false must leave the residual pending")
	}

	// On-state: the pending map evaluates in place.
	e, marker, idx = mk(true, okMap)
	if err := e.stepDefCleanup(marker, idx); err != nil {
		t.Fatalf("on-state cleanup: %v", err)
	}
	if e.tape.At(1).Eval {
		t.Fatal("EvalResidual=true must evaluate the residual")
	}

	// List error arm: the container's evaluation failure propagates.
	e, marker, idx = mk(true, failList)
	if err := e.stepDefCleanup(marker, idx); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("list error arm = %v, want boom", err)
	}

	// Map error arm.
	e, marker, idx = mk(true, failMap)
	if err := e.stepDefCleanup(marker, idx); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("map error arm = %v, want boom", err)
	}
}

// TestResidualEvalSweepAndTeardownArms drives the two SECOND-ORDER marker
// processors directly (the in-package seam): stepCloseParen's surviving-
// marker sweep and the TCO eager teardown must propagate a residual-eval
// failure exactly as the main loop does. In census programs the marker
// reaching either site has already been stepped (its containers are no
// longer pending), so these arms need the hand-built first-time-step
// tape.
func TestResidualEvalSweepAndTeardownArms(t *testing.T) {
	r := runReg(t)
	failMap := pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}}))

	// Sweep arm: a never-stepped EvalResidual marker inside a closing
	// group with a pending failing container below it.
	e := New(r)
	e.tape = NewTape([]Value{
		NewOpenParen(),
		failMap,
		NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true}),
		NewCloseParen(),
	}, stackHeadroom)
	e.pointer = 3
	if err := e.stepCloseParen(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("sweep arm = %v, want boom", err)
	}

	// TCO teardown arm: the eager replay of the same marker.
	e2 := New(r)
	e2.tape = NewTape([]Value{
		NewOpenParen(),
		pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}})),
		NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true}),
	}, stackHeadroom)
	if err := e2.teardownFrameState(frameTailScan{TailStart: 2, RCIdx: -1}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("teardown arm = %v, want boom", err)
	}
}
