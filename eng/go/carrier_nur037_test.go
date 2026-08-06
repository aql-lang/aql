package eng

// NUR037: a fn declared inside another fn's body and named as a
// higher-order body word (`for-each [step] xs`) resolves under the
// interpreter and was undefined under the compiled path — the island
// span, the CALL_NATIVE const-bake, and the closure probe all baked the
// NAME against the check-time registry, which the VM's runtime registry
// never binds. bodyRefsFnLocalFn detects the shape at the
// recordDispatchOutcome seam and refuses the whole program ("slow, not
// wrong"). These tests pin the predicate's scope rule — fn-local
// FUNCTION bindings only — and the refusal wiring, positive and
// negative per the repo's test discipline.

import (
	"strings"
	"testing"
)

// fnLocalBind pushes a fn-body baseline and then binds name INSIDE it,
// so Depth(name) > baseline[name] — the ComputeCaptures fn-local rule.
func fnLocalBind(r *Registry, name string, v Value) {
	r.PushFnBaseline(r.Defs.Snapshot())
	r.Defs.Push(name, v)
}

// bodySig builds a for-each-shaped sig: body at position 0 (NoEvalArgs),
// data at position 1.
func bodySig(effect CompileEffect, callable *CallableSpec) *Signature {
	return &Signature{
		Args:          []*Type{TList, TList},
		NoEvalArgs:    map[int]bool{0: true},
		CompileEffect: effect,
		Callable:      callable,
	}
}

func bodyArgs(bodyWords ...string) []Value {
	toks := make([]Value, len(bodyWords))
	for i, w := range bodyWords {
		toks[i] = NewWord(w)
	}
	return []Value{NewList(toks), NewList([]Value{NewInteger(1)})}
}

func TestBodyRefsFnLocalFnPositive(t *testing.T) {
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("step"))
	if !hit || name != "step" {
		t.Errorf("fn-local fn body word: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnCallableGate(t *testing.T) {
	// The Callable disjunct of the family gate: a closure-eligible word
	// with no CompileFallbackBody effect still matches.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	sig := bodySig(0, &CallableSpec{BodyPos: 0, BodyOut: 1})
	if name, hit := BodyRefsFnLocalFn(r, sig, bodyArgs("step")); !hit || name != "step" {
		t.Errorf("Callable-gated sig: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnRepeatedNameEarlyOut(t *testing.T) {
	// A body naming the fn twice exercises the walker's first-hit
	// early-out; the result is the same single name.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("step", "step"))
	if !hit || name != "step" {
		t.Errorf("repeated body word: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnModuleScopeBaselineNil(t *testing.T) {
	// Module scope (no enclosing fn): nothing is fn-local, callbacks
	// keep compiling.
	r := newTestRegistry(t)
	r.Defs.Push("step", NewFunction(FnDefInfo{Name: "step"}))
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("step")); hit {
		t.Errorf("module-scope binding with nil baseline matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnModuleScopeBindingUnderBaseline(t *testing.T) {
	// A module-scope fn referenced from INSIDE a fn body: bound before
	// the baseline snapshot, so Depth ≤ baseline — not fn-local.
	r := newTestRegistry(t)
	r.Defs.Push("step", NewFunction(FnDefInfo{Name: "step"}))
	r.PushFnBaseline(r.Defs.Snapshot())
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("step")); hit {
		t.Errorf("module-scope binding under the baseline matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnValueDefNotMatched(t *testing.T) {
	// A fn-local VALUE def (the mini-redis KEYS accumulator shape) is
	// the closure path's lexical-capture territory — must NOT match.
	r := newTestRegistry(t)
	fnLocalBind(r, "acc", NewInteger(7))
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("acc")); hit {
		t.Errorf("fn-local VALUE def matched %q; only Function bindings may match", name)
	}
}

func TestBodyRefsFnLocalFnUnboundWordSkipped(t *testing.T) {
	// A forward ref / registered-native name has no Defs binding: skipped.
	r := newTestRegistry(t)
	r.PushFnBaseline(r.Defs.Snapshot())
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), bodyArgs("nosuch")); hit {
		t.Errorf("unbound body word matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnNilSigAndNoBodies(t *testing.T) {
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	if _, hit := BodyRefsFnLocalFn(r, nil, bodyArgs("step")); hit {
		t.Error("nil sig must decline")
	}
	plain := &Signature{Args: []*Type{TList}, CompileEffect: CompileFallbackBody}
	if _, hit := BodyRefsFnLocalFn(r, plain, bodyArgs("step")); hit {
		t.Error("a sig with no NoEvalArgs positions must decline")
	}
}

func TestBodyRefsFnLocalFnStructuredWordExcluded(t *testing.T) {
	// The family gate: a structured-lowering word (if / for — NoEvalArgs
	// but neither CompileFallbackBody nor Callable) records its bodies as
	// inline events (fn-local dispatch lowers to CALL_USER by unit ref),
	// so it must keep compiling.
	r := newTestRegistry(t)
	fnLocalBind(r, "g", NewFunction(FnDefInfo{Name: "g"}))
	sig := bodySig(0, nil)
	if name, hit := BodyRefsFnLocalFn(r, sig, bodyArgs("g")); hit {
		t.Errorf("structured-lowering sig matched %q; the family gate must exclude it", name)
	}
}

func TestBodyRefsFnLocalFnNonBodyArgNotWalked(t *testing.T) {
	// The fn-local fn appears only inside the DATA arg (position 1, no
	// NoEvalArgs): data is evaluated, not name-baked — must not match.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	args := []Value{
		NewList([]Value{NewWord("add")}),  // body: registered-word only
		NewList([]Value{NewWord("step")}), // data arg mentions the fn
	}
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), args); hit {
		t.Errorf("non-NoEvalArgs data arg was walked (matched %q); must not", name)
	}
}

func TestBodyRefsFnLocalFnLambdaBodyOpaque(t *testing.T) {
	// A LAMBDA body operand is an FnDefInfo payload — WalkBodyWords
	// treats it as opaque (its own capture analysis handled it), so the
	// predicate must not fire on the lambda value itself.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	args := []Value{
		NewFunction(FnDefInfo{Name: ""}),
		NewList([]Value{NewInteger(1)}),
	}
	if name, hit := BodyRefsFnLocalFn(r, bodySig(CompileFallbackBody, nil), args); hit {
		t.Errorf("lambda body operand matched %q; FnDefInfo payloads are opaque", name)
	}
}

// --- recordDispatchOutcome wiring -------------------------------------------

func TestRecordDispatchOutcomeFnLocalFnRefusal(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	sig := bodySig(CompileFallbackBody, nil)
	recordDispatchOutcome(r, "for-each", sig, bodyArgs("step"), nil, SrcPos{}, r)
	if es.Compilable {
		t.Fatal("fn-local fn body word must latch the program uncompilable")
	}
	if !strings.Contains(es.Reason, "fn-local fn `step`") ||
		!strings.Contains(es.Reason, "for-each") {
		t.Errorf("refusal reason = %q; want the fn-local-fn reason naming step and for-each", es.Reason)
	}
}

func TestRecordDispatchOutcomeFnLocalFnAlreadyProducedSkips(t *testing.T) {
	// A dispatch a structured ReturnsFn hook already recorded (case's
	// branch-chain desugar) lowered its clause bodies as inline events —
	// not a leak path; the guard must not refuse it.
	r := newTestRegistry(t)
	es := armEmit(r)
	fnLocalBind(r, "step", NewFunction(FnDefInfo{Name: "step"}))
	out := NewInteger(7)
	es.producedBy[out.ID] = producer{}
	sig := bodySig(CompileFallbackBody, nil)
	recordDispatchOutcome(r, "case", sig, bodyArgs("step"), []Value{out}, SrcPos{}, r)
	if strings.Contains(es.Reason, "fn-local fn") {
		t.Errorf("already-produced dispatch hit the fn-local-fn refusal: %q", es.Reason)
	}
}

func TestRecordDispatchOutcomeModuleScopeNotFnLocalRefused(t *testing.T) {
	// The module-scope twin of the refusal test: whatever else the
	// recording chain decides, the fn-local-fn reason must not fire.
	r := newTestRegistry(t)
	es := armEmit(r)
	r.Defs.Push("step", NewFunction(FnDefInfo{Name: "step"}))
	sig := bodySig(CompileFallbackBody, nil)
	recordDispatchOutcome(r, "for-each", sig, bodyArgs("step"), nil, SrcPos{}, r)
	if strings.Contains(es.Reason, "fn-local fn") {
		t.Errorf("module-scope callback hit the fn-local-fn refusal: %q", es.Reason)
	}
}
