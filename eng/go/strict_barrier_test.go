package eng

// PROTOTYPE coverage for the strict-barrier rule (strictForwardBarrier /
// strandedForwardError): a function word beginning its own dispatch is
// always a forward-collection barrier, and a parked forward that cannot
// commit is reported stranded instead of waiting for the fn word's
// result. Tests are white-box and in-package, mirroring the
// commitBarrierForward seams in engine_seam7_barrier_test.go.

import (
	"strings"
	"testing"
)

// setStrict flips the prototype gate for one test and restores it.
func setStrict(t *testing.T, on bool) {
	t.Helper()
	prev := strictForwardBarrier
	strictForwardBarrier = on
	t.Cleanup(func() { strictForwardBarrier = prev })
}

// --- strandedForwardError direct seams ------------------------------------

func TestStrictBarrierInternalWordExempt(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	if err := e.strandedForwardError("__pa"); err != nil {
		t.Errorf("engine-internal boundary word must be exempt, got %v", err)
	}
}

func TestStrictBarrierNoPendingForward(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1)}, 1)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("no pending forward → nil, got %v", err)
	}
}

func TestStrictBarrierParenScopeStops(t *testing.T) {
	// A forward below an OpenParen is outside the current scope.
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0), NewOpenParen()}, 2)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("open-paren scope barrier → nil, got %v", err)
	}
}

func TestStrictBarrierParkedDefStrands(t *testing.T) {
	// There is NO def exemption: `def x add 1 2` must strand under the
	// strict rule. The fn-definition idiom survives structurally instead
	// — def's keyword signature (`[name/q fn/q sigs]`) matches at plan
	// time and never parks, so it never reaches this error path.
	e := engWithTape(t, []Value{fwdMarker("def", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("add"); err == nil {
		t.Error("a parked def waiting on a value slot must strand")
	}
}

func TestStrictBarrierStrandedError(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	err := e.strandedForwardError("iota")
	if err == nil {
		t.Fatal("expected a stranded-forward error")
	}
	if err.Code != "signature_error" {
		t.Errorf("code = %q, want signature_error", err.Code)
	}
	for _, want := range []string{"size", "iota", "strict rule", "size (iota …)"} {
		if !strings.Contains(err.Detail, want) {
			t.Errorf("detail %q missing %q", err.Detail, want)
		}
	}
}

// --- the stepWord seam ------------------------------------------------------

// strictTape parks an uncommittable forward (its word is unregistered,
// so commitBarrierForward declines) below a dispatching "cadd".
func strictTape() []Value {
	return []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWord("cadd"), NewInteger(2), NewInteger(3),
	}
}

func TestStrictBarrierStepWordOff(t *testing.T) {
	setStrict(t, false)
	e := engWithTape(t, strictTape(), 3)
	if err := e.stepWord(e.tape.At(3)); err != nil {
		t.Errorf("gate off keeps shipped behaviour, got %v", err)
	}
}

func TestStrictBarrierStepWordOn(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, strictTape(), 3)
	err := e.stepWord(e.tape.At(3))
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWord")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}

// --- the stepWordUsurp seam -------------------------------------------------

func TestStrictBarrierStepWordUsurpOn(t *testing.T) {
	setStrict(t, true)
	vals := []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWordUsurp("cadd", false), NewInteger(2), NewInteger(3),
	}
	e := engWithTape(t, vals, 3)
	w, _ := AsWord(e.tape.At(3))
	err := e.stepWordUsurp(e.tape.At(3), w)
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWordUsurp")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}

// --- GAP A: dot-access chains are strict-barrier'd ---------------------------

func TestReachBoundaryLabel(t *testing.T) {
	// receiver word + first literal segment → "Rand.int"
	info := ReachInfo{
		Receiver: []Value{NewWord("Rand")},
		Segments: []ReachSeg{{KeyLit: NewAtom("int")}},
	}
	if got := reachBoundaryLabel(info); got != "Rand.int" {
		t.Errorf("label = %q, want Rand.int", got)
	}
	// receiver-only fallback (computed first segment)
	comp := ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{{Computed: true, KeyExpr: []Value{NewWord("k")}}},
	}
	if got := reachBoundaryLabel(comp); got != "m" {
		t.Errorf("computed-segment label = %q, want m", got)
	}
	// multi-token receiver → generic
	multi := ReachInfo{Receiver: []Value{NewWord("a"), NewWord("b")}}
	if got := reachBoundaryLabel(multi); got != "<dot-access>" {
		t.Errorf("multi-receiver label = %q, want <dot-access>", got)
	}
	// atom receiver (foo/q.bar) resolves via AsAtom
	atomRecv := ReachInfo{
		Receiver: []Value{NewAtom("foo")},
		Segments: []ReachSeg{{KeyLit: NewAtom("bar")}},
	}
	if got := reachBoundaryLabel(atomRecv); got != "foo.bar" {
		t.Errorf("atom-receiver label = %q, want foo.bar", got)
	}
	// non-word non-atom receiver → generic fallback
	odd := ReachInfo{Receiver: []Value{NewInteger(1)}}
	if got := reachBoundaryLabel(odd); got != "<dot-access>" {
		t.Errorf("non-word receiver label = %q, want <dot-access>", got)
	}
}

// registerCollector adds a 1-arg forward-collecting native for the
// dot-access barrier tests (covRegistry extra hook).
func registerCollector(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "collector",
		Signatures: []Signature{{
			Args:    []*Type{TAny},
			Impl:    Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil }),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
}

func TestStrictForwardScanReachBarrier(t *testing.T) {
	setStrict(t, true)
	r := covRegistry(t, registerCollector)
	e := NewTop(r)
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("x")},
		Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
		Eval:     true,
	})
	e.tape = NewTape([]Value{NewWord("collector"), reach}, stackHeadroom)
	e.pointer = 0
	fn := r.Lookup("collector")
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "collector", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	// Under strict, the scan must STOP at the Reach — it stays a Reach on
	// the tape (not expanded to a dot chain).
	if !IsReach(e.tape.At(1)) {
		t.Errorf("strict forward scan must leave the Reach un-expanded, got %v", e.tape.At(1))
	}
}

func TestStrictForwardScanReachBarrierOffExpands(t *testing.T) {
	setStrict(t, false)
	r := covRegistry(t, registerCollector)
	e := NewTop(r)
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("x")},
		Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
		Eval:     true,
	})
	e.tape = NewTape([]Value{NewWord("collector"), reach}, stackHeadroom)
	e.pointer = 0
	fn := r.Lookup("collector")
	_ = e.resolveForwardArgs(fn, WordInfo{Name: "collector", ArgCount: -1})
	// Gate off: the Reach is expanded to its dot chain (no longer a Reach).
	if IsReach(e.tape.At(1)) {
		t.Error("gate off: the Reach must expand during forward collection")
	}
}

func TestStrictDotAccessStrandsEndToEnd(t *testing.T) {
	setStrict(t, true)
	r := covRegistry(t, registerCollector)
	e := NewTop(r)
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("x")},
		Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
		Eval:     true,
	})
	// `collector x.k` — collector parks (forward scan breaks at the reach),
	// then the reach dispatches at statement position and strands collector.
	_, err := e.Run([]Value{NewWord("collector"), reach})
	if err == nil {
		t.Fatal("a dot-access feeding a parked forward must strand under strict")
	}
	if !strings.Contains(err.Error(), "still waiting") || !strings.Contains(err.Error(), "x.k") {
		t.Errorf("strand error = %q, want collector waiting on the x.k barrier", err.Error())
	}
}

// registerGAndDot adds a 1-and-2-arg `gg` (the smaller overload commits
// at a boundary) and a trivial `dot` so a Reach can evaluate.
func registerGAndDot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "gg",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(777)}, nil
			}), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(999)}, nil
			}), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "dot",
		Signatures: []Signature{{
			Args:      []*Type{TAtom, TAny},
			QuoteArgs: map[int]bool{0: true},
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(42)}, nil
			}),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
}

func TestStrictDotAccessCommitsGuardFirst(t *testing.T) {
	setStrict(t, true)
	r := covRegistry(t, registerGAndDot)
	r.Defs.Push("xmap", NewInteger(0)) // receiver binding (dot ignores it)
	e := NewTop(r)
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("xmap")},
		Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
		Eval:     true,
	})
	// `gg 1 xmap.k` — gg collects 1, the Reach is a barrier so gg parks
	// with its 1-arg overload committable; at the Reach the commit fires
	// FIRST (gg → 777), then the dot-access evaluates (→ 42). The commit
	// path is what this exercises.
	out, err := e.Run([]Value{NewWord("gg"), NewInteger(1), reach})
	if err != nil {
		t.Fatalf("commit-at-reach then dot-access must succeed, got %v", err)
	}
	var saw777 bool
	for _, v := range out {
		if n, _ := AsInteger(v); n == 777 {
			saw777 = true
		}
	}
	if !saw777 {
		t.Errorf("gg's 1-arg overload must commit at the reach barrier (777), got %v", out)
	}
}
