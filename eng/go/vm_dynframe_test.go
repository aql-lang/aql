package eng

import (
	"strings"
	"testing"
)

// vm_dynframe_test.go pins the whole-frame dynamic-apply replay's VM arms:
// the CALL_DYN_FRAME underflow guard and the escapedFlow translation helper.

func TestCallDynFrameUnderflow(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	if _, err := vc.callDynFrame(r, 1, 0, nil, seam7Dbg, 0); err == nil ||
		!strings.Contains(err.Error(), "CALL_DYN_FRAME underflow") {
		t.Errorf("empty-stack replay must underflow loudly, got %v", err)
	}
	if _, err := vc.callDynFrame(r, 0, 0, []Value{NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("a zero-width replay window is a mis-emit — must error")
	}
}

func TestEscapedFlowArms(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	// No signal, nil registries skipped.
	if op := vc.escapedFlow(nil, r); op != 0 {
		t.Errorf("no signal must report 0, got %v", op)
	}
	r.FlowCtrl = FlowBreak
	if op := vc.escapedFlow(nil, r); op != OpFlowBreak {
		t.Errorf("break signal = %v, want OpFlowBreak", op)
	}
	if r.FlowCtrl != FlowNone {
		t.Error("escapedFlow must clear the consumed signal")
	}
	r.FlowCtrl = FlowContinue
	if op := vc.escapedFlow(r); op != OpFlowContinue {
		t.Errorf("continue signal = %v, want OpFlowContinue", op)
	}
	if r.FlowCtrl != FlowNone {
		t.Error("escapedFlow must clear the consumed signal")
	}
}
