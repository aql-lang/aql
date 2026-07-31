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

func boruBodyFd(body ...Value) FnDefInfo {
	return FnDefInfo{Signatures: []Signature{{Impl: BORU(body)}}}
}

// Policy: a registry never armed (a plain interpreter run) must not stamp;
// arming enables the attempt; forks inherit the armed flag.
func TestStampDetachedFnPolicyGate(t *testing.T) {
	r := stampReg(t)
	fd := boruBodyFd(NewInteger(1))
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

// Word-policy contract (the 2026-07-15 LIFT, user-authorized): a
// policy-gated registry STAMPS exactly like a policy-free one — the VM's
// per-dispatch gate (vmContext.gateWord) now enforces the word rules when
// the unit runs, raising the interpreter's identical denial — so the
// pre-lift stamp refusal is retired. The pin: gated and open registries
// produce the SAME stamp outcome for the same fd, and no policy-flavoured
// reason is ever recorded.
func TestStampDetachedFnWordPolicyGate(t *testing.T) {
	fd := boruBodyFd(NewInteger(1))

	gated := stampReg(t)
	gated.EnableRuntimeStamping()
	if err := gated.Capabilities.Set(CapPolicy, allowAllChecker{}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	_, gatedOK := StampDetachedFn(gated, fd, SrcPos{})

	open := stampReg(t)
	open.EnableRuntimeStamping()
	_, openOK := StampDetachedFn(open, fd, SrcPos{})

	if gatedOK != openOK {
		t.Fatalf("policy-gated stamp outcome %v must equal the policy-free outcome %v (the lift)", gatedOK, openOK)
	}
	for _, ev := range gated.StampEvents() {
		if strings.Contains(ev.Reason, "policy") {
			t.Fatalf("no policy-flavoured stamp refusal may remain post-lift: %+v", ev)
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
	// An IDENTITY-LESS capture value (runtime-minted under the mode-gated
	// ID elision) STAMPS too since the §7a landing (2026-07-16): the
	// detached compile mints a fresh identity on a CLONE of the captured
	// slice — the frozen snapshot keys its slot like any other capture,
	// and the input value stays untouched (no shared-value mutation).
	captured := boruBodyFd(NewInteger(1))
	captured.Captured = []CapturedBinding{{Name: "kv", Value: NewInteger(9)}}
	ref, ok := StampDetachedFn(r, captured, SrcPos{})
	if !ok || ref == nil {
		t.Fatalf("identity-less capture must stamp (§7a), got %+v", r.StampEvents())
	}
	if n, err := AsInteger(ref.Captures[0]); len(ref.Captures) != 1 || err != nil || n != 9 {
		t.Fatalf("ref must carry the frozen capture value, got %+v", ref.Captures)
	}
	if captured.Captured[0].Value.ID != "" {
		t.Fatalf("the INPUT capture value must stay identity-less (clone, not mutate), got %q", captured.Captured[0].Value.ID)
	}

	// A multi-own-sig fn stamps PER SIG since the §7b landing: the
	// single-sig entry takes the first stampable sig, and StampDetachedSig
	// reaches each remaining overload — MatchFnSig at the invoke seam picks
	// the sig, whose own Impl ref then runs.
	multi := FnDefInfo{Signatures: []Signature{
		{Impl: BORU([]Value{NewInteger(1)})},
		{Impl: BORU([]Value{NewInteger(2)})},
	}}
	if ref0, ok := StampDetachedFn(r, multi, SrcPos{}); !ok || ref0 == nil {
		t.Fatalf("multi-own-sig first overload must stamp (§7b)")
	}
	if ref1, ok := StampDetachedSig(r, multi, 1, SrcPos{}); !ok || ref1 == nil {
		t.Fatalf("multi-own-sig second overload must stamp (§7b)")
	}
	if _, ok := StampDetachedSig(r, multi, 9, SrcPos{}); ok {
		t.Fatalf("an out-of-range sig index must decline")
	}

	if _, ok := StampDetachedFn(r, boruBodyFd(), SrcPos{}); ok {
		t.Fatalf("empty body must decline")
	}

	sentinel := boruBodyFd(NewWord("break"))
	if _, ok := StampDetachedFn(r, sentinel, SrcPos{}); ok {
		t.Fatalf("flow-sentinel body must decline")
	}

	fallbackOnly := FnDefInfo{Signatures: []Signature{
		{Fallback: true, Impl: BORU([]Value{NewInteger(1)})},
	}}
	if _, ok := StampDetachedFn(r, fallbackOnly, SrcPos{}); ok {
		t.Fatalf("fallback-only fn (no own sig) must decline")
	}
}

// StampFnValue value-level gates: non-fn input, Go-backed fns (no BORU own
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
		{Impl: &BORUImpl{Body: []Value{NewInteger(1)}, Compiled: pre}},
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
// the interpreter (CallBORU over the BORU body), not the frozen unit.
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
	// The BORU body the interpreter runs instead: a single literal 7.
	sig := &Signature{Impl: &BORUImpl{
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
	sig.Impl.(*BORUImpl).Compiled.depSnap = map[string]depSnapEntry{
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
	v := Value{Parent: TFunction, Data: boruBodyFd(NewInteger(1))}
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
	w := Value{Parent: TFunction, Data: boruBodyFd(NewInteger(2))}
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
	v := Value{Parent: TFunction, Data: boruBodyFd(NewInteger(1))}
	if _, ok := StampFnValue(r, v); !ok {
		t.Fatalf("stamp failed")
	}
	// A capturing fn — even with an identity-less capture value — STAMPS
	// since the §7a landing (the detached compile mints an identity on a
	// clone), recording Stamped=true under its name.
	captured := boruBodyFd(NewInteger(1))
	captured.Name = "grabby"
	captured.Captured = []CapturedBinding{{Name: "kv", Value: NewInteger(9)}}
	if _, ok := StampDetachedFn(r, captured, SrcPos{Row: 2, Col: 1}); !ok {
		t.Fatalf("capturing fn must stamp (§7a): %+v", r.StampEvents())
	}
	// A body the unit compile REFUSES (the context-dependent `args`, which
	// reads the interpreter's per-call args stack) records its reason.
	argsy := boruBodyFd(NewWord("args"))
	argsy.Name = "argsy"
	if _, ok := StampDetachedFn(r, argsy, SrcPos{Row: 3, Col: 7}); ok {
		t.Fatalf("args-reading fn must decline")
	}
	// Shape noise (a fallback-only fn) records NOTHING.
	noise := FnDefInfo{Signatures: []Signature{{Fallback: true, Impl: BORU([]Value{NewInteger(1)})}}}
	if _, ok := StampDetachedFn(r, noise, SrcPos{}); ok {
		t.Fatalf("a fallback-only fn must decline")
	}

	events := r.StampEvents()
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (stamped + capture stamped + args refusal): %v", len(events), events)
	}
	if !events[0].Stamped || events[0].Reason != "" {
		t.Fatalf("first event must be the successful stamp: %+v", events[0])
	}
	if !events[1].Stamped || events[1].Name != "grabby" {
		t.Fatalf("second event must be grabby's capture STAMP (§7a): %+v", events[1])
	}
	if events[2].Stamped || events[2].Name != "argsy" ||
		events[2].Pos.Row != 3 || events[2].Reason == "" {
		t.Fatalf("third event must be argsy's refusal: %+v", events[2])
	}

	// Forks and module-style inheritance feed the SAME log.
	fork := r.ForkConcurrent()
	if _, ok := StampFnValue(fork, Value{Parent: TFunction, Data: boruBodyFd(NewInteger(2))}); !ok {
		t.Fatalf("fork stamp failed")
	}
	child, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	child.InitRootContext()
	child.InheritRuntimeStamping(r)
	if _, ok := StampFnValue(child, Value{Parent: TFunction, Data: boruBodyFd(NewInteger(3))}); !ok {
		t.Fatalf("inherited-child stamp failed")
	}
	if got := len(r.StampEvents()); got != 5 {
		t.Fatalf("shared log must hold 5 events, got %d", got)
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

// The §7c JIT re-stamp (REFUSAL-CLOSURE.0): a detached ref whose dep was
// REBOUND after the stamp re-compiles against the live binding at invoke
// time — the fresh twin runs on the VM with the NEW value (parity with the
// interpreter's live resolution) — instead of degrading permanently to
// CallBORU. A second invoke under the same binding REUSES the twin (no
// second compile); the try budget bounds a hot rebinding loop; and a
// declined re-stamp (stamping disarmed) takes the interpreter.
func TestInvokeCallbackJITRestamp(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()
	r.Defs.Push("dep", NewInteger(1))

	fd := boruBodyFd(NewWord("dep"))
	fd.Name = "reader"
	ref, ok := StampDetachedFn(r, fd, SrcPos{Row: 1, Col: 1})
	if !ok {
		t.Fatalf("initial stamp declined: %+v", r.StampEvents())
	}
	sig := &Signature{Impl: &BORUImpl{Body: []Value{NewWord("dep")}, Compiled: ref}}

	// Fresh: the frozen unit returns the stamp-time binding.
	out, err := InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("fresh invoke: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 1 {
		t.Fatalf("fresh unit must return the stamp-time value 1, got %v", out[0])
	}

	// Rebind dep → the old snapshot is stale → the seam re-stamps and the
	// twin returns the LIVE value, exactly as the interpreter resolves it.
	r.Defs.Pop("dep")
	r.Defs.Push("dep", NewInteger(2))
	stampsBefore := len(r.StampEvents())
	out, err = InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("restamped invoke: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 2 {
		t.Fatalf("re-stamped unit must return the live value 2, got %v", out[0])
	}
	if got := len(r.StampEvents()); got != stampsBefore+1 {
		t.Fatalf("the re-stamp must record one stamp event, got %d new", got-stampsBefore)
	}

	// Same binding, second invoke: the twin is fresh — REUSED, no compile.
	stampsBefore = len(r.StampEvents())
	if out, err = InvokeCallback(r, sig, nil, nil); err != nil {
		t.Fatalf("reuse invoke: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 2 {
		t.Fatalf("twin reuse must return 2, got %v", out[0])
	}
	if got := len(r.StampEvents()); got != stampsBefore {
		t.Fatalf("twin reuse must not re-compile, got %d new events", got-stampsBefore)
	}

	// The try budget: each further rebind pays one re-stamp until the budget
	// (restampMaxTries, one already spent) exhausts; after that the seam
	// stays on CallBORU — which STILL resolves the live binding, so values
	// keep matching the interpreter (slow, not wrong).
	for i := 3; i <= 6; i++ {
		r.Defs.Pop("dep")
		r.Defs.Push("dep", NewInteger(int64(i)))
		if out, err = InvokeCallback(r, sig, nil, nil); err != nil {
			t.Fatalf("rebind %d invoke: %v", i, err)
		}
		if n, _ := AsInteger(out[0]); n != int64(i) {
			t.Fatalf("rebind %d: got %v, want the live value (VM twin or CallBORU alike)", i, out[0])
		}
	}
	if tries := ref.restamp.tries; tries != restampMaxTries {
		t.Fatalf("try budget must exhaust at %d, got %d", restampMaxTries, tries)
	}

	// A declined re-stamp (stamping disarmed mid-life) takes the interpreter:
	// reset the budget to isolate the decline arm from the exhausted arm.
	ref.restamp.tries = 0
	ref.restamp.cur = nil
	r.DisableRuntimeStamping()
	r.Defs.Pop("dep")
	r.Defs.Push("dep", NewInteger(9))
	if out, err = InvokeCallback(r, sig, nil, nil); err != nil {
		t.Fatalf("disarmed invoke: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 9 {
		t.Fatalf("disarmed re-stamp must fall to CallBORU's live resolution (9), got %v", out[0])
	}
}
