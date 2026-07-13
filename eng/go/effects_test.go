package eng

import (
	"bytes"
	"strings"
	"testing"
)

// The C1 effect fence (effects.go, design/RUNTIME-INDEPENDENCE-COMPLETION-
// PLAN.0.md): the ledger counts observable effects so the compiled-mode
// fallback arms can prove "nothing escaped yet" before silently re-running a
// program on the interpreter — a re-run after an effect duplicates it (the
// L-DUP class). These tests pin the ledger primitives, the writer fence, and
// the InvokeCallback retry fence; the whole-program arms are pinned on the
// lang side (bytecode_effectfence_test.go).

// A nil ledger counts nothing and never panics — the pre-fence behaviour for
// registries assembled without NewRegistry (zero-value test fixtures).
func TestEffectLedgerNilSafe(t *testing.T) {
	var l *EffectLedger
	l.Note() // must not panic
	if got := l.Count(); got != 0 {
		t.Fatalf("nil ledger Count = %d, want 0", got)
	}
}

// The positive twin: a real ledger counts each Note.
func TestEffectLedgerCounts(t *testing.T) {
	l := &EffectLedger{}
	if got := l.Count(); got != 0 {
		t.Fatalf("fresh ledger Count = %d, want 0", got)
	}
	l.Note()
	l.Note()
	if got := l.Count(); got != 2 {
		t.Fatalf("Count after two notes = %d, want 2", got)
	}
}

// ArmEffectFence wraps both writers: a non-empty write marks the ledger and
// still reaches the underlying writer; an EMPTY write does not count (no
// observable effect); restore reinstates the original writers so later writes
// stop counting.
func TestArmEffectFenceCountsWrites(t *testing.T) {
	r := runUnitReg(t)
	var out, errOut bytes.Buffer
	r.Output, r.ErrOutput = &out, &errOut

	restore := r.ArmEffectFence()
	if _, err := r.Output.Write(nil); err != nil {
		t.Fatalf("empty write: %v", err)
	}
	if got := r.Effects.Count(); got != 0 {
		t.Fatalf("empty write counted: Count = %d, want 0", got)
	}
	if _, err := r.Output.Write([]byte("x")); err != nil {
		t.Fatalf("stdout write: %v", err)
	}
	if _, err := r.ErrOutput.Write([]byte("y")); err != nil {
		t.Fatalf("stderr write: %v", err)
	}
	if got := r.Effects.Count(); got != 2 {
		t.Fatalf("Count after one write per channel = %d, want 2", got)
	}
	if out.String() != "x" || errOut.String() != "y" {
		t.Fatalf("writes did not reach the underlying writers: out=%q err=%q", out.String(), errOut.String())
	}

	restore()
	if r.Output != &out || r.ErrOutput != &errOut {
		t.Fatal("restore did not reinstate the original writers")
	}
	if _, err := r.Output.Write([]byte("z")); err != nil {
		t.Fatalf("post-restore write: %v", err)
	}
	if got := r.Effects.Count(); got != 2 {
		t.Fatalf("post-restore write counted: Count = %d, want 2", got)
	}
}

// The negative twin: nil writers are left nil (arming a writerless registry
// must not manufacture a writer), and restore is still safe.
func TestArmEffectFenceNilWriters(t *testing.T) {
	r := runUnitReg(t)
	r.Output, r.ErrOutput = nil, nil
	restore := r.ArmEffectFence()
	if r.Output != nil || r.ErrOutput != nil {
		t.Fatal("arming a nil writer must leave it nil")
	}
	restore()
	if r.Output != nil || r.ErrOutput != nil {
		t.Fatal("restore over nil writers must leave them nil")
	}
}

// noteEffectSig builds a 0-arg, 0-result native signature whose handler marks
// the registry's effect ledger — the unit-test stand-in for a printing word.
func noteEffectSig() *Signature {
	return &Signature{
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			reg.NoteEffect()
			return nil, nil
		}),
	}
}

// TestInvokeCallbackBailAfterEffectPropagates pins the C1 fence on the
// callback seam: a stamped unit that emits an observable effect (the
// CALL_NATIVE notes the ledger, standing in for a write to the peer) and THEN
// raises internal_error (CALL_DYNAMIC underflow) must PROPAGATE the
// internal_error — the CallAQL retry would re-run the body and duplicate the
// effect. TestInvokeCallbackInternalErrorFallsBack is the positive twin: the
// same bail with NO prior effect still retries on the interpreter.
func TestInvokeCallbackBailAfterEffectPropagates(t *testing.T) {
	p := &Program{
		Sigs: []SigRef{{Word: "zz-note", Sig: noteEffectSig()}},
		Fns: []CompiledFn{{
			Name: "emit-then-boom", NParams: 0, NLocals: 0,
			Code: []Instr{
				{Op: OpCallNative, Arg: 0},
				{Op: OpCallDynamic, Arg: 0},
				{Op: OpRet, Arg: 0},
			},
			Debug: []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	r := runUnitReg(t)
	// The sig carries an AQL body the interpreter COULD run to 42 — the test
	// is that the fence refuses to, because the effect already escaped.
	sig := &Signature{Impl: &AQLImpl{Body: []Value{NewInteger(42)}, Compiled: ref}}
	out, err := InvokeCallback(r, sig, nil, nil)
	if !isInternalErr(err) {
		t.Fatalf("fenced callback bail: err = %v (out=%v), want the propagated internal_error", err, out)
	}
	if got := r.Effects.Count(); got != 1 {
		t.Fatalf("effect count = %d, want exactly 1 (no retry ran the body again)", got)
	}
	if !strings.Contains(err.Error(), "internal") {
		t.Fatalf("propagated error should carry the internal_error taxonomy: %v", err)
	}
}
