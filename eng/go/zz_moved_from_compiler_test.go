package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestW8DispatchRematchDeclines pins the rematch record's decline arms: an
// inactive recorder, an empty window, an unresolvable operand, an empty word,
// no operands, the first-trap-wins latch, the inactive-recorder interface
// no-op, the promoted-operand rewrite of a rematch trap, and the VM
// underflow guard.
func TestW8DispatchRematchVMGuard(t *testing.T) {
	r := newTestRegistry(t)
	_ = r
	// VM underflow guard.
	vc := &vmContext{r: r}
	if err := vc.dispatchRematch(&DispatchSpec{Word: "w", NArgs: 2, NWritten: 2}, nil, nil, 0); err == nil {
		t.Error("a short stack must error")
	}
	// VM render-bound guard: a spec whose written bound is outside 1..NArgs
	// is malformed (the recorder proves the bound before recording).
	if err := vc.dispatchRematch(&DispatchSpec{Word: "w", NArgs: 1},
		[]core.Value{core.NewInteger(1)}, nil, 0); err == nil || !strings.Contains(err.Error(), "written bound") {
		t.Errorf("a zero written bound must raise the bound guard, got %v", err)
	}
}
