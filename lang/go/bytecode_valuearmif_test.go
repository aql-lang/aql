package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmitValueArmIf: `if cond v1 v2` with VALUE arms (not `[body]` code lists)
// now lowers the then arm symmetrically with the else arm — a select (push v1 /
// push v2 around JMP_IF_FALSE) — instead of refusing "then-branch not captured".
// This covers the direct form, a dynamic condition, and the usurp-if shape
// (`usurp if` dispatches `if` with value arms). The negative half pins that a
// COMPUTED then value (an event eagerly on the stack) still refuses the
// value-then path rather than miscompiling.
func TestEmitValueArmIf(t *testing.T) {
	cases := []struct{ src, want string }{
		{`if true 99 88`, "[99]"},
		{`if false 99 88`, "[88]"},
		{`def x 0  if (x eq 0) 99 88`, "[99]"},         // dynamic (event) condition, value arms
		{`def ifu (usurp if)  true ifu 88 99`, "[99]"}, // usurp-if reverses to value arms
		{`def ifu (usurp if)  false ifu 88 99`, "[88]"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (value-arm if), but refused", c.src)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native (value-arm select), not island:\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, errI := b.Run(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// NEGATIVE: `if cond (a) (b)` — BOTH arms are computed events — stacks two
	// eager values below the cond, which the single-eager computed-branch
	// lowering cannot seat, so the recorder refuses it (and falls back
	// faithfully). A SINGLE computed arm (then OR else) does compile; see
	// TestEmitComputedThenIf.
	const bothComputed = `def x 0  if (x eq 0) (add 1 2) (sub 10 1)`
	m, _ := New()
	mp, reason, _, _ := m.CompileCheck(bothComputed)
	if mp != nil {
		t.Errorf("%q: both-computed arms must NOT compile (got a program); reason=%q", bothComputed, reason)
	}
}
