package lang

import (
	"fmt"
	"strings"
	"testing"
)

// OpCallNativePoly's FAITHFUL no-match raise (plan 3c): a poly recorded with a
// PolyNoMatchSpec — the check pass proved, at the failed-dispatch tape state
// it recovered from, that the interpreter's sigError diagnostic is
// rebuildable from the runtime operand window — raises the byte-identical
// signature_error in the VM instead of deferring the whole run to the
// interpreter. The three rows cover the written-tuple derivation modes the
// spec must reproduce: a single unclaimed forward token (`(x add 1)` renders
// one argument), the full stack prefix (`1 x add` renders both), and the
// stack fallback after a word breaks the forward walk (`(x add x)` renders
// one).
func TestPolyNoMatchRaisesByteIdentical(t *testing.T) {
	rows := []string{
		`def m (flex {k:[1 2]})
def x (m get "k")
(x add 1)`,
		`def m (flex {k:[1 2]})
def x (m get "k")
1 x add`,
		`def m (flex {k:[1 2]})
def x (m get "k")
(x add x)`,
	}
	for _, src := range rows {
		t.Run(fmt.Sprintf("%.20s", strings.ReplaceAll(src, "\n", " ")), func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, ran, errC := a.RunCompiled(src)
			if !ran {
				t.Fatal("the row must run COMPILED — the poly no-match raise owns it, not a fallback")
			}
			if errC == nil {
				t.Fatal("the compiled poly no-match must raise")
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, errI := b.Run(src)
			if errI == nil {
				t.Fatal("the interpreter must raise")
			}
			if errC.Error() != errI.Error() {
				t.Errorf("poly no-match raise is not byte-identical:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
			}
		})
	}
}

// TestPolyNoMatchAfterEffectRaisesCanonical pins the bug the raise closes: a
// side effect BEFORE a poly no-match used to trip the C1 effect fence — the
// fallback was blocked ("output was already emitted") and the user saw an
// internal_error telling them to report a compiler bug instead of the
// canonical signature_error. With the spec-gated raise the compiled run keeps
// its effects (printed exactly once) and raises the interpreter's error.
func TestPolyNoMatchAfterEffectRaisesCanonical(t *testing.T) {
	const src = `print "pre-effect"
def m (flex {k:[1 2]})
def x (m get "k")
(x add 1)`
	var outC, outI strings.Builder
	a := mustNew(t)
	a.SetOutput(&outC)
	_, ran, errC := a.RunCompiled(src)
	if !ran {
		t.Fatal("the effectful row must run COMPILED — deferring here is the fence-blocked bug")
	}
	b := mustNew(t)
	b.SetOutput(&outI)
	_, errI := b.Run(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("effectful poly no-match parity:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
	}
	if got := outC.String(); got != outI.String() || strings.Count(got, "pre-effect") != 1 {
		t.Errorf("the effect must run exactly once: compiled output %q, interp output %q", outC.String(), outI.String())
	}
}

// TestPolyNoMatchDeeperStackKeepsDefer — the written-tuple bound (the
// local-add lesson applied to polys): with a third concrete value below the
// 2-operand window, the interpreter's stack-prefix derivation renders THREE
// arguments where the window holds two, so the spec's window mapping declines
// at record time and the sound defer stays — parity via the interpreter
// fallback.
func TestPolyNoMatchDeeperStackKeepsDefer(t *testing.T) {
	const src = `def m (flex {k:[1 2]})
def x (m get "k")
9 1 x add`
	a := mustNew(t)
	_, ran, errC := a.RunCompiled(src)
	if ran {
		t.Fatal("the deeper-stack shape must defer (the written tuple is wider than the window)")
	}
	b := mustNew(t)
	_, errI := b.Run(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("fallback parity: compiled-path err %v != interp err %v", errC, errI)
	}
}
