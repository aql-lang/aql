package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_dynframe_test.go pins the whole-frame dynamic-apply replay's VM arms:
// the CALL_DYN_FRAME underflow guard and the escapedFlow translation helper.

func TestCallDynFrameUnderflow(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	if _, _, err := vc.callDynFrame(r, 1, 0, nil, seam7Dbg, 0); err == nil ||
		!strings.Contains(err.Error(), "CALL_DYN_FRAME underflow") {
		t.Errorf("empty-stack replay must underflow loudly, got %v", err)
	}
	if _, _, err := vc.callDynFrame(r, 0, 0, []core.Value{core.NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("a zero-width replay window is a mis-emit — must error")
	}
}

// TestCallDynFrameIslandError pins the replay island's ERROR arm — the one the
// Apply kernel's admission rule must not swallow. `cfail` is a
// trivial-delegation wrapper with no compiled unit, so dynApplyEnter declines
// it whether or not a prefix is present, the window islands, and whatever the
// interpreter raises there has to come back out of callDynFrame.
//
// Both prefix shapes are driven, because the admission rule now branches on the
// prefix: an empty one, and the [9] below that only an all-forward callee could
// be entered over.
func TestCallDynFrameIslandError(t *testing.T) {
	r, _, fail := seam7DelegReg(t)
	vc := seam7VC(r)
	stack := []core.Value{core.NewInteger(9), fail, core.NewInteger(5)}
	for _, frameBase := range []int{1, 0} {
		_, _, err := vc.callDynFrame(vc.r, 2, frameBase, stack, seam7Dbg, 0)
		if err == nil || !strings.Contains(err.Error(), "cfail") {
			t.Errorf("replay island (frameBase %d) must surface the applied fn's error, got %v", frameBase, err)
		}
	}
}

func TestEscapedFlowArms(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	// No signal, nil registries skipped.
	if op := vc.escapedFlow(nil, r); op != 0 {
		t.Errorf("no signal must report 0, got %v", op)
	}
	r.FlowCtrl = core.FlowBreak
	if op := vc.escapedFlow(nil, r); op != compiler.OpFlowBreak {
		t.Errorf("break signal = %v, want OpFlowBreak", op)
	}
	if r.FlowCtrl != core.FlowNone {
		t.Error("escapedFlow must clear the consumed signal")
	}
	r.FlowCtrl = core.FlowContinue
	if op := vc.escapedFlow(r); op != compiler.OpFlowContinue {
		t.Errorf("continue signal = %v, want OpFlowContinue", op)
	}
	if r.FlowCtrl != core.FlowNone {
		t.Error("escapedFlow must clear the consumed signal")
	}
}
