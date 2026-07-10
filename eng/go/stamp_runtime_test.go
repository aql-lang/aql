package eng

import "testing"

// StampDetachedFn / StampFnValue gate tests. The full end-to-end compile of a
// REAL body (a lang `fn` with `add`/`convert` words) lives in lang/go — eng
// has no def/fn words to build one — so these pin the pure shape/policy gates
// and the depsFresh freshness contract, each with its negative twin.

func stampReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func aqlBodyFd(body ...Value) FnDefInfo {
	return FnDefInfo{Signatures: []Signature{{Impl: AQL(body)}}}
}

// Policy: a registry never armed (a plain interpreter run) must not stamp;
// arming enables the attempt; forks inherit the armed flag.
func TestStampDetachedFnPolicyGate(t *testing.T) {
	r := stampReg(t)
	fd := aqlBodyFd(NewInteger(1))
	if _, ok := StampDetachedFn(r, fd, SrcPos{}); ok {
		t.Fatalf("policy off: stamp must decline")
	}
	if r.RuntimeStampingEnabled() {
		t.Fatalf("flag must default off")
	}
	r.EnableRuntimeStamping()
	if !r.RuntimeStampingEnabled() {
		t.Fatalf("EnableRuntimeStamping did not arm")
	}
	if !r.ForkConcurrent().RuntimeStampingEnabled() {
		t.Fatalf("fork must inherit the armed flag")
	}
	// nil receivers are inert, not panics.
	var nilR *Registry
	nilR.EnableRuntimeStamping()
	if nilR.RuntimeStampingEnabled() {
		t.Fatalf("nil registry reports armed")
	}
	if _, ok := StampDetachedFn(nil, fd, SrcPos{}); ok {
		t.Fatalf("nil registry: stamp must decline")
	}
}

// Shape gates: captured fns, multi-own-sig fns, empty bodies, sentinel bodies,
// and non-fn values all decline, returning the input unchanged.
func TestStampDetachedFnShapeGates(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()

	captured := aqlBodyFd(NewInteger(1))
	captured.Captured = []CapturedBinding{{Name: "kv", Value: NewInteger(9)}}
	if _, ok := StampDetachedFn(r, captured, SrcPos{}); ok {
		t.Fatalf("capturing fn must decline (OpPushClosure territory)")
	}

	multi := FnDefInfo{Signatures: []Signature{
		{Impl: AQL([]Value{NewInteger(1)})},
		{Impl: AQL([]Value{NewInteger(2)})},
	}}
	if _, ok := StampDetachedFn(r, multi, SrcPos{}); ok {
		t.Fatalf("multi-own-sig fn must decline (MatchFnSig picks at runtime)")
	}

	if _, ok := StampDetachedFn(r, aqlBodyFd(), SrcPos{}); ok {
		t.Fatalf("empty body must decline")
	}

	sentinel := aqlBodyFd(NewWord("break"))
	if _, ok := StampDetachedFn(r, sentinel, SrcPos{}); ok {
		t.Fatalf("flow-sentinel body must decline")
	}

	fallbackOnly := FnDefInfo{Signatures: []Signature{
		{Fallback: true, Impl: AQL([]Value{NewInteger(1)})},
	}}
	if _, ok := StampDetachedFn(r, fallbackOnly, SrcPos{}); ok {
		t.Fatalf("fallback-only fn (no own sig) must decline")
	}
}

// StampFnValue value-level gates: non-fn input, Go-backed fns (no AQL own
// sig), and already-stamped values decline and return the INPUT unchanged.
func TestStampFnValueGates(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()

	notFn := NewInteger(5)
	if got, ok := StampFnValue(r, notFn); ok || got.ID != notFn.ID {
		t.Fatalf("non-fn value must decline unchanged")
	}

	goFn := Value{Parent: TFunction, Data: FnDefInfo{Signatures: []Signature{
		{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, nil
		})},
	}}}
	if _, ok := StampFnValue(r, goFn); ok {
		t.Fatalf("Go-backed fn must decline (dispatches natively already)")
	}

	pre := &CompiledFnRef{Unit: 0, Prog: &Program{Fns: []CompiledFn{{}}}}
	stampedAlready := Value{Parent: TFunction, Data: FnDefInfo{Signatures: []Signature{
		{Impl: &AQLImpl{Body: []Value{NewInteger(1)}, Compiled: pre}},
	}}}
	if _, ok := StampFnValue(r, stampedAlready); ok {
		t.Fatalf("already-stamped fn must decline (first stamp wins)")
	}
}

// depsFresh: the invoke-time freshness contract for runtime-stamped refs.
func TestDepsFresh(t *testing.T) {
	r := stampReg(t)
	bound := NewInteger(7)
	r.Defs.Push("helper", bound)

	var nilRef *CompiledFnRef
	if !nilRef.depsFresh(r) {
		t.Fatalf("nil ref is vacuously fresh")
	}
	compileTime := &CompiledFnRef{}
	if !compileTime.depsFresh(r) {
		t.Fatalf("nil depSnap (compile-time ref) is vacuously fresh")
	}

	fresh := &CompiledFnRef{depSnap: map[string]depSnapEntry{
		"helper": {Depth: r.Defs.Depth("helper"), Gen: r.Defs.Gen("helper")},
	}}
	if !fresh.depsFresh(r) {
		t.Fatalf("matching depth+gen must be fresh")
	}
	if fresh.depsFresh(nil) {
		t.Fatalf("nil registry cannot validate — must report stale")
	}

	// A rebind pushes a shadowing level: generation (and depth) change → stale.
	r.Defs.Push("helper", NewInteger(8))
	if fresh.depsFresh(r) {
		t.Fatalf("rebound dep must be stale")
	}
	// Popping back does NOT restore freshness: the generation is monotone, so
	// any window in which the binding differed marks the ref stale for good.
	// Conservative direction only — the fallback is the interpreter.
	r.Defs.Pop("helper")
	if fresh.depsFresh(r) {
		t.Fatalf("push+pop bumps the generation — must stay stale")
	}
	// The load-bearing case a depth+ID probe would MISS: undef then redef of
	// an ID-less runtime value lands back at the same depth with a different
	// binding. The generation catches it.
	r2 := stampReg(t)
	v1 := NewInteger(7)
	r2.Defs.Push("helper", v1)
	snap2 := &CompiledFnRef{depSnap: map[string]depSnapEntry{
		"helper": {Depth: r2.Defs.Depth("helper"), Gen: r2.Defs.Gen("helper")},
	}}
	r2.Defs.Pop("helper")
	r2.Defs.Push("helper", NewInteger(9)) // same depth, different value
	if snap2.depsFresh(r2) {
		t.Fatalf("same-depth undef+redef must be stale (generation check)")
	}
	// Fork continuity: an untouched dep stays fresh on a ForkConcurrent clone
	// (Clone carries the gen timeline); a fork-local rebind marks it stale on
	// the fork only.
	r3 := stampReg(t)
	r3.Defs.Push("helper", NewInteger(7))
	snap3 := &CompiledFnRef{depSnap: map[string]depSnapEntry{
		"helper": {Depth: r3.Defs.Depth("helper"), Gen: r3.Defs.Gen("helper")},
	}}
	fork := r3.ForkConcurrent()
	if !snap3.depsFresh(fork) {
		t.Fatalf("untouched dep must stay fresh across a fork")
	}
	fork.Defs.Push("helper", NewInteger(8))
	if snap3.depsFresh(fork) {
		t.Fatalf("fork-local shadow must be stale on the fork")
	}
	if !snap3.depsFresh(r3) {
		t.Fatalf("fork-local shadow must not affect the parent")
	}
	// An empty (non-nil) snapshot — a dep-free runtime stamp — is always fresh.
	depFree := &CompiledFnRef{depSnap: map[string]depSnapEntry{}}
	if !depFree.depsFresh(r) {
		t.Fatalf("dep-free runtime ref must be fresh")
	}
}

// InvokeCallback consults depsFresh: a stale runtime-stamped ref must take
// the interpreter (CallAQL over the AQL body), not the frozen unit.
func TestInvokeCallbackStaleDepFallsBack(t *testing.T) {
	r := stampReg(t)
	r.Defs.Push("dep", NewInteger(1))

	// A unit that would return 42 if the VM path ran.
	p := &Program{
		Consts: []Value{NewInteger(42)},
		Fns: []CompiledFn{{
			Name:  "const42",
			Code:  []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpRet, Arg: 0}},
			Debug: []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	// The AQL body the interpreter runs instead: a single literal 7.
	sig := &Signature{Impl: &AQLImpl{
		Body: []Value{NewInteger(7)},
		Compiled: &CompiledFnRef{Prog: p, Unit: 0, depSnap: map[string]depSnapEntry{
			"dep": {Depth: 99, Gen: -1}, // never matches → stale
		}},
	}}
	out, err := InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("stale ref must run the interpreter body (7), got %v", out[0])
	}

	// The positive twin: a FRESH snapshot takes the VM unit (42).
	sig.Impl.(*AQLImpl).Compiled.depSnap = map[string]depSnapEntry{
		"dep": {Depth: r.Defs.Depth("dep"), Gen: r.Defs.Gen("dep")},
	}
	out, err = InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback (fresh): %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("fresh ref must run the VM unit (42), got %v", out[0])
	}
}

// StampFnValueInPlace: the PRE-PUBLICATION twin — first stamp mutates the
// shared impl (both aliases of the value see it), a second attempt declines
// on already-stamped, and a non-fn declines outright.
func TestStampFnValueInPlace(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()
	v := Value{Parent: TFunction, Data: aqlBodyFd(NewInteger(1))}
	if !StampFnValueInPlace(r, v) {
		t.Fatalf("first in-place stamp must succeed")
	}
	fd := v.Data.(FnDefInfo)
	if ref := fd.Signatures[0].CompiledRef(); ref == nil || ref.Prog == nil {
		t.Fatalf("in-place stamp must mutate the shared impl")
	}
	if StampFnValueInPlace(r, v) {
		t.Fatalf("an already-stamped value must decline (first stamp wins)")
	}
	if StampFnValueInPlace(r, NewInteger(3)) {
		t.Fatalf("a non-fn value must decline")
	}
	// Policy off: untouched.
	r2 := stampReg(t)
	w := Value{Parent: TFunction, Data: aqlBodyFd(NewInteger(2))}
	if StampFnValueInPlace(r2, w) {
		t.Fatalf("an unarmed registry must not stamp in place")
	}
}
