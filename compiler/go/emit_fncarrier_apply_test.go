package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestResolveDynamicApplyReadSubstitutedLeadRefuses pins the Stage 1
// leading-apply guard: a READ-substituted fn-carrier lead (a def-read of a
// name bound to a computed fn — defReads carries its ID) with no
// statically-known closure shape refuses, because the interpreter
// word-dispatches such a read while OpCallDynamic's island runs
// anonymous-VALUE semantics — for a named Go-impl fn value the two diverge
// (`def k (FnUtil.const 7)  (k 99)`: 7 interpreted, 99 from the island).
// The same lead WITHOUT the def-read tag (an event-provenance apply, the
// `((FnUtil.const 7) 99)` spelling) keeps today's lowering — the island
// mirrors the interpreter's value semantics exactly there.
func TestResolveDynamicApplyReadSubstitutedLeadRefuses(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}}
	lead := core.NewCarrier(core.TFunction)
	residual := []core.Value{lead, core.NewCarrier(core.TInteger)}
	_, op, reason := es.resolveDynamicApply(lw, residual)
	if op != OpCallDynamic || reason != "" {
		t.Fatalf("an event-provenance carrier lead must keep the lowering: op=%v reason=%q", op, reason)
	}
	es.defReads = map[string]string{lead.ID: "k"}
	_, op, reason = es.resolveDynamicApply(lw, residual)
	if op != 0 || !strings.Contains(reason, "def-bound computed fn apply") {
		t.Fatalf("a read-substituted lead with unknown closure shape must refuse: op=%v reason=%q", op, reason)
	}
}

// TestRecordDispatchRematchDeclinesReadSubstitutedCarrier pins the rematch
// window guard: a READ-substituted payload-less fn-carrier window value
// poisons the window (the interpreter word-dispatches that read before the
// word ever collects — `def q (if c [fn-arm] [fn-arm])  add 1 (q 5)`
// re-matched add over [1, fn, 5] and raised where the interpreter computes
// 7), so the record declines and the caller's refusal stands.
func TestRecordDispatchRematchDeclinesReadSubstitutedCarrier(t *testing.T) {
	es := NewEmitState()
	carrier := core.NewCarrier(core.TFunction)
	es.defReads = map[string]string{carrier.ID: "q"}
	if es.RecordDispatchRematchValues("add", []core.Value{core.NewInteger(1), carrier}, 0, 1, core.SrcPos{}) {
		t.Error("a read-substituted fn-carrier window value must decline the rematch")
	}
}
