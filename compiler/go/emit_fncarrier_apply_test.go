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

// TestRecordDynApplyNameArms pins the §4.3 name-route apply's decline arms
// and its success path over a recorded def-site bind.
func TestRecordDynApplyNameArms(t *testing.T) {
	es := NewEmitState()
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
	}}})
	out := core.NewCarrier(core.TAny)
	arg := core.NewInteger(5)

	if es.RecordDynApplyName("", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("an empty name must decline")
	}
	if es.RecordDynApplyName("h", []core.Value{arg}, core.NewInteger(1), out, core.SrcPos{}) {
		t.Error("a non-fn value must decline")
	}
	qfn := fn
	qfn.Quoted = true
	if es.RecordDynApplyName("h", []core.Value{arg}, qfn, out, core.SrcPos{}) {
		t.Error("a quoted fn must decline")
	}
	multi := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny, core.TAny}, BarrierPos: -1,
	}}})
	if es.RecordDynApplyName("h", []core.Value{arg}, multi, out, core.SrcPos{}) {
		t.Error("a multi-return fn must decline")
	}
	if es.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a name with no recorded def-site bind must decline")
	}

	// A recorded def-site bind with an event operand: the apply records.
	bound := core.NewCarrier(core.TFunction)
	es.frames[0] = append(es.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: 4}})
	seqBefore := len(es.frames[0])
	if !es.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Fatal("a bound name with a resolvable arg must record")
	}
	if len(es.frames[0]) != seqBefore+1 {
		t.Fatalf("expected one appended apply event, frames grew %d", len(es.frames[0])-seqBefore)
	}
	// A def site with NO operand home declines.
	es2 := NewEmitState()
	es2.frames[0] = append(es2.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: -1}})
	if es2.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a def site with no operand home must decline")
	}
	// An fn-valued ARG declines.
	es3 := NewEmitState()
	es3.frames[0] = append(es3.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: 4}})
	if es3.RecordDynApplyName("h", []core.Value{fn}, fn, out, core.SrcPos{}) {
		t.Error("an fn-valued arg must decline")
	}
	// An UNRESOLVABLE arg (a provenance-less carrier) declines.
	if es3.RecordDynApplyName("h", []core.Value{core.NewCarrier(core.TInteger)}, fn, out, core.SrcPos{}) {
		t.Error("an unresolvable arg must decline")
	}
	// A def site recorded with a SRC operand (a capture/local slot) is the
	// other resolution arm.
	es4 := NewEmitState()
	es4.frames[0] = append(es4.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", src: localOperand(0), srcSeq: -1}})
	if !es4.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a src-operand def site must record")
	}
	_ = bound
}
