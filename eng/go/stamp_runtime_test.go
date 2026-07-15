package eng

import (
	"strings"
	"testing"
)

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

// allowAllChecker is a WordChecker that denies nothing — installing ANY
// checker marks the registry policy-gated, which is what the stamp gate
// keys on (compiled dispatch consults no word rules, so even a permissive
// policy must keep dispatch on the interpreter where the gate runs).
type allowAllChecker struct{}

func (allowAllChecker) CheckWord(string) error { return nil }

// Word-policy gate: a policy-gated registry must never stamp a detached
// unit — a compiled body invoked via InvokeCallback would bypass the
// per-dispatch word gate (the same security refusal CompileCheck applies
// to whole programs). The refusal is a recorded attempt so -compile-report
// attributes it; an identical registry without the checker never records
// the policy reason (the negative twin).
func TestStampDetachedFnWordPolicyGate(t *testing.T) {
	const policyReason = "policy-gated registry (compiled dispatch does not consult word rules)"
	fd := aqlBodyFd(NewInteger(1))

	gated := stampReg(t)
	gated.EnableRuntimeStamping()
	if err := gated.Capabilities.Set(CapPolicy, allowAllChecker{}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	if _, ok := StampDetachedFn(gated, fd, SrcPos{}); ok {
		t.Fatalf("policy-gated registry: stamp must decline")
	}
	events := gated.StampEvents()
	if len(events) != 1 || events[0].Stamped || events[0].Reason != policyReason {
		t.Fatalf("want one refusal event with the policy reason, got %+v", events)
	}

	open := stampReg(t)
	open.EnableRuntimeStamping()
	_, _ = StampDetachedFn(open, fd, SrcPos{})
	for _, ev := range open.StampEvents() {
		if ev.Reason == policyReason {
			t.Fatalf("policy-free registry recorded the policy refusal: %+v", ev)
		}
	}
}

// Shape gates: captured fns, multi-own-sig fns, empty bodies, sentinel bodies,
// and non-fn values all decline, returning the input unchanged.
func TestStampDetachedFnShapeGates(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()

	// Capturing bodies COMPILE post the capture-slot landing (plan Phase
	// 6.3 — fd.Captured rides the closure compile; the ref carries the
	// values; see lang/go/stamp_capture_test.go for the end-to-end pin).
	// This eng-level fixture still declines on the NARROWER gate: its
	// capture value is runtime-minted with no compile identity, so the
	// closure compile refuses it — recorded, so -compile-report attributes.
	captured := aqlBodyFd(NewInteger(1))
	captured.Captured = []CapturedBinding{{Name: "kv", Value: NewInteger(9)}}
	if _, ok := StampDetachedFn(r, captured, SrcPos{}); ok {
		t.Fatalf("identity-less capture must decline")
	}
	events := r.StampEvents()
	if len(events) == 0 || !strings.Contains(events[len(events)-1].Reason, "closure captures a runtime-minted value") {
		t.Fatalf("want the runtime-minted-capture reason recorded, got %+v", events)
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

// The stamp-attribution collector (stamp_report.go): armed attempts record
// (stamped and refusal-with-reason), shape noise does not, forks and
// InheritRuntimeStamping share one log, and unarmed registries report nil.
func TestStampReportCollector(t *testing.T) {
	r := stampReg(t)
	if r.StampEvents() != nil {
		t.Fatalf("unarmed registry must report nil events")
	}
	r.EnableRuntimeStamping()

	// A successful stamp records Stamped=true.
	v := Value{Parent: TFunction, Data: aqlBodyFd(NewInteger(1))}
	if _, ok := StampFnValue(r, v); !ok {
		t.Fatalf("stamp failed")
	}
	// A capturing fn records its refusal reason.
	captured := aqlBodyFd(NewInteger(1))
	captured.Name = "grabby"
	captured.Captured = []CapturedBinding{{Name: "kv", Value: NewInteger(9)}}
	if _, ok := StampDetachedFn(r, captured, SrcPos{Row: 3, Col: 7}); ok {
		t.Fatalf("capturing fn must decline")
	}
	// Shape noise (multi-own-sig) records NOTHING.
	multi := FnDefInfo{Signatures: []Signature{
		{Impl: AQL([]Value{NewInteger(1)})},
		{Impl: AQL([]Value{NewInteger(2)})},
	}}
	if _, ok := StampDetachedFn(r, multi, SrcPos{}); ok {
		t.Fatalf("multi-sig must decline")
	}

	events := r.StampEvents()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (stamped + capture refusal): %v", len(events), events)
	}
	if !events[0].Stamped || events[0].Reason != "" {
		t.Fatalf("first event must be the successful stamp: %+v", events[0])
	}
	if events[1].Stamped || events[1].Name != "grabby" ||
		events[1].Pos.Row != 3 || events[1].Reason == "" {
		t.Fatalf("second event must be grabby's capture refusal: %+v", events[1])
	}

	// Forks and module-style inheritance feed the SAME log.
	fork := r.ForkConcurrent()
	if _, ok := StampFnValue(fork, Value{Parent: TFunction, Data: aqlBodyFd(NewInteger(2))}); !ok {
		t.Fatalf("fork stamp failed")
	}
	child, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	child.InitRootContext()
	child.InheritRuntimeStamping(r)
	if _, ok := StampFnValue(child, Value{Parent: TFunction, Data: aqlBodyFd(NewInteger(3))}); !ok {
		t.Fatalf("inherited-child stamp failed")
	}
	if got := len(r.StampEvents()); got != 4 {
		t.Fatalf("shared log must hold 4 events, got %d", got)
	}

	// Inheriting from an UNARMED parent is a no-op.
	plain := stampReg(t)
	orphan := stampReg(t)
	orphan.InheritRuntimeStamping(plain)
	if orphan.RuntimeStampingEnabled() {
		t.Fatalf("inheriting from an unarmed parent must not arm")
	}
	// nil receivers stay inert.
	var nilR *Registry
	nilR.InheritRuntimeStamping(r)
	if nilR.StampEvents() != nil {
		t.Fatalf("nil registry must report nil events")
	}
	nilR.recordStampEvent(StampEvent{})
	// An UNARMED (nil-log) registry's record is inert too — the nil-receiver
	// guard on stampLog.record.
	plain.recordStampEvent(StampEvent{})
	if plain.StampEvents() != nil {
		t.Fatalf("unarmed record must stay inert")
	}
}

// DisableRuntimeStamping / ResetStampLog are the compiled-request scope helpers
// (RunCompiled restores the flag and drops the rolled-back check-pass stamps).
// A disarm undoes ONLY the flag — the attribution log survives so StampReport
// still reads after the run; a reset clears the log but leaves stamping armed.
// Both stay inert on nil / unarmed registries.
func TestDisableAndResetStampLog(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()
	r.recordStampEvent(StampEvent{Name: "a", Stamped: true})
	r.recordStampEvent(StampEvent{Name: "b", Stamped: true})
	if got := len(r.StampEvents()); got != 2 {
		t.Fatalf("armed log must hold 2 events, got %d", got)
	}
	// ResetStampLog clears the events but keeps stamping armed.
	r.ResetStampLog()
	if got := len(r.StampEvents()); got != 0 {
		t.Fatalf("reset must clear the log, got %d", got)
	}
	if !r.RuntimeStampingEnabled() {
		t.Fatalf("reset must not disarm stamping")
	}
	// DisableRuntimeStamping disarms WITHOUT wiping the log.
	r.recordStampEvent(StampEvent{Name: "c", Stamped: true})
	r.DisableRuntimeStamping()
	if r.RuntimeStampingEnabled() {
		t.Fatalf("DisableRuntimeStamping must disarm")
	}
	if got := len(r.StampEvents()); got != 1 {
		t.Fatalf("disarm must leave the log intact, got %d", got)
	}

	// Unarmed registry: ResetStampLog hits the nil-log guard (stampLog.reset),
	// inert.
	plain := stampReg(t)
	plain.ResetStampLog()
	if plain.StampEvents() != nil {
		t.Fatalf("reset on an unarmed registry must stay inert")
	}
	// nil receivers stay inert (no panic).
	var nilR *Registry
	nilR.DisableRuntimeStamping()
	nilR.ResetStampLog()
	if nilR.RuntimeStampingEnabled() {
		t.Fatalf("nil registry reports armed")
	}
}

// storedBodySpecFor + compileStoredParamBody degenerate arms (the
// stored-param-body machinery behind Signature.StoredBodies): an undeclared
// position yields no spec; a nil/inactive state, a non-list body, and an
// empty body all decline the compile and leave the operand untouched.
func TestStoredParamBodyDeclines(t *testing.T) {
	sig := &Signature{StoredBodies: []StoredBodySpec{{Pos: 2, Params: []FnParam{{Name: "r", Type: TMap}}}}}
	if storedBodySpecFor(sig, 2) == nil {
		t.Error("declared position must yield its spec")
	}
	if storedBodySpecFor(sig, 1) != nil {
		t.Error("an undeclared position must yield nil")
	}

	params := []FnParam{{Name: "r", Type: TMap}}
	var nilES *EmitState
	if _, ok := nilES.compileStoredParamBody(NewList([]Value{NewInteger(1)}), params); ok {
		t.Error("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredParamBody(NewList([]Value{NewInteger(1)}), params); ok {
		t.Error("registry-less EmitState must decline")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es := &EmitState{reg: r}
	if _, ok := es.compileStoredParamBody(NewInteger(1), params); ok {
		t.Error("a non-list body must decline")
	}
	if _, ok := es.compileStoredParamBody(NewList(nil), params); ok {
		t.Error("an empty body must decline")
	}
	if _, ok := es.compileStoredParamBody(NewList([]Value{NewWord("break")}), params); ok {
		t.Error("a flow-sentinel body must decline")
	}
	// A nil param Type defaults to Any (the input carrier's generalisation);
	// the compile itself declines here (no armed pass), which is the point —
	// every decline leaves the operand untouched.
	if _, ok := es.compileStoredParamBody(NewList([]Value{NewInteger(1)}), []FnParam{{Name: "x"}}); ok {
		t.Error("an unarmed pass must decline")
	}
}

// interpMemberInert's all-inert MAP arm went corpus-invisible when check-prop
// bodies moved to stored-param units (the map-bearing prop-spec bodies now
// take the StoredBodies edge before the inert-bake gates) — pin it directly.
func TestInterpMemberInertMapArms(t *testing.T) {
	inert := NewOrderedMap()
	inert.Set("a", NewInteger(1))
	if !interpMemberInert(NewMap(inert)) {
		t.Error("a map of inert members IS interp-inert")
	}
	active := NewOrderedMap()
	active.Set("a", NewCarrier(TInteger))
	if interpMemberInert(NewMap(active)) {
		t.Error("a map bearing a non-inert member (a carrier) is NOT interp-inert")
	}
	if !interpMemberInert(NewParenExpr([]Value{NewInteger(1)})) {
		t.Error("a paren-expr of inert tokens IS interp-inert")
	}
	if interpMemberInert(NewParenExpr([]Value{NewCarrier(TInteger)})) {
		t.Error("a paren-expr bearing a non-inert token is NOT interp-inert")
	}
}
