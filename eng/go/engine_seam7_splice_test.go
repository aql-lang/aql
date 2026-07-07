package eng

// Wave-7 coverage, part 5: splice / hint / rearrange helpers reached with
// hand-built engine state. See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// fnShapeCtxEngine returns an engine positioned in a fn-shape typed-binding
// context (isFnShapeTypedBindingContext() == true).
func fnShapeCtxEngine(t *testing.T) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 1})
	e := NewTop(r)
	e.tape = NewTape([]Value{NewMap(m), fwd, NewInteger(0)}, stackHeadroom)
	e.pointer = 3
	if !e.isFnShapeTypedBindingContext() {
		t.Fatal("setup: expected fn-shape typed-binding context")
	}
	return e
}

// --- maybeAddFnShapeHint ---------------------------------------------------

func TestS7MaybeAddFnShapeHintNil(t *testing.T) {
	e := &Engine{}
	if got := e.maybeAddFnShapeHint(nil); got != nil {
		t.Errorf("nil err → nil, got %v", got)
	}
}

func TestS7MaybeAddFnShapeHintNonSigError(t *testing.T) {
	e := &Engine{}
	orig := &AqlError{Code: "other_error", Detail: "x"}
	if got := e.maybeAddFnShapeHint(orig); got != error(orig) {
		t.Error("a non-signature error must pass through unchanged")
	}
}

func TestS7MaybeAddFnShapeHintSetsHint(t *testing.T) {
	e := fnShapeCtxEngine(t)
	// Empty existing hint → hint is set.
	err := &AqlError{Code: "signature_error", Src: "double", Hint: ""}
	out := e.maybeAddFnShapeHint(err).(*AqlError)
	if !strings.Contains(out.Hint, "double/q") {
		t.Errorf("hint = %q, want the /q suggestion", out.Hint)
	}
}

func TestS7MaybeAddFnShapeHintAppendsHint(t *testing.T) {
	e := fnShapeCtxEngine(t)
	// Non-empty existing hint → appended.
	err := &AqlError{Code: "signature_error", Src: "double", Hint: "prior"}
	out := e.maybeAddFnShapeHint(err).(*AqlError)
	if !strings.Contains(out.Hint, "prior") || !strings.Contains(out.Hint, "double/q") {
		t.Errorf("hint = %q, want prior + /q suggestion", out.Hint)
	}
}

// --- rearrangeForForward ---------------------------------------------------

func TestS7RearrangeForForwardZeroTotal(t *testing.T) {
	e := engWithTape(t, []Value{NewWord("x")}, 0)
	e.rearrangeForForward(0, 0) // total == 0 → early return, no panic
}

// --- spliceMatchResults fallback branch ------------------------------------

func TestS7SpliceMatchResultsFallback(t *testing.T) {
	// len(sortedIndices) != n and n != 0 → contiguous fallback splice.
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2), NewInteger(3), NewWord("w")}, 3)
	err := e.spliceMatchResults(&MatchResult{}, nil, 2, []Value{NewInteger(99)})
	if err != nil {
		t.Fatalf("spliceMatchResults: %v", err)
	}
	// argStart = pointer(3)-n(2) = 1; [1..3] replaced by [99].
	if e.pointer != 1 {
		t.Errorf("pointer = %d, want 1", e.pointer)
	}
}

func TestS7SpliceMatchResultsFallbackClampsArgStart(t *testing.T) {
	// pointer < n → argStart clamps to 0.
	e := engWithTape(t, []Value{NewInteger(1), NewWord("w")}, 1)
	if err := e.spliceMatchResults(&MatchResult{}, nil, 5, []Value{NewInteger(7)}); err != nil {
		t.Fatalf("spliceMatchResults: %v", err)
	}
	if e.pointer != 0 {
		t.Errorf("pointer = %d, want 0 (clamped)", e.pointer)
	}
}
