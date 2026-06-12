package lang

import (
	"strings"
	"testing"
)

// Stage-1 bytecode emitter goldens (design/aql-bytecode-plan.0.md
// Stage 1 gate): the recording pass lowers straight-line monomorphic
// code to the expected instruction stream, and everything beyond the
// stage is refused with a precise reason — never lowered wrongly.

func compile(t *testing.T, src string) (string, string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck(%q): %v", src, err)
	}
	if prog == nil {
		return "", reason
	}
	return prog.Disassemble(), ""
}

func TestEmitGoldens(t *testing.T) {
	cases := []struct {
		src    string
		golden string
	}{
		{`add 1 2`, `0000 PUSH_CONST  k1   ; 2 (Integer)
0001 PUSH_CONST  k0   ; 1 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 sigs=1 max-stack=2
`},
		// Swap form `a f b` binds sig[0] from the forward side — the
		// lowering reflects the binding, so the push order differs
		// from `add 1 2` while computing the same sum.
		{`1 add 2`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 sigs=1 max-stack=2
`},
		{`0 add 7 sub 3`, `0000 PUSH_CONST  k1   ; 0 (Integer)
0001 PUSH_CONST  k0   ; 7 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; sub (Number, Number)
; consts=3 sigs=2 max-stack=2
`},
		// A paren result feeding the next call: the prior result stays
		// on the simulated stack; only the literal is pushed.
		{`(1 add 2) mul 3`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; mul (Number, Number)
; consts=3 sigs=2 max-stack=2
`},
		// Top-level strings are stripped to carriers by check mode;
		// RecordStrip preserves the originals for interning.
		{`'a' add 'b'`, `0000 PUSH_CONST  k1   ; 'a' (ProperString)
0001 PUSH_CONST  k0   ; 'b' (ProperString)
0002 CALL_NATIVE s0   ; add (Scalar, Scalar)
; consts=2 sigs=1 max-stack=2
`},
		// Literal-substitution def: x resolves to the interned literal
		// through value provenance — the report's §5.2 inline case.
		{`def x 1 x add 2`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 sigs=1 max-stack=2
`},
	}
	for _, c := range cases {
		got, reason := compile(t, c.src)
		if reason != "" {
			t.Errorf("%q unexpectedly uncompilable: %s", c.src, reason)
			continue
		}
		if got != c.golden {
			t.Errorf("%q lowering changed:\n--- got\n%s--- want\n%s", c.src, got, c.golden)
		}
	}
}

// Mirror-equivalent forms produce IDENTICAL bytecode. The mirror of
// `add 1 2` is `2 1 add` (the kernel's one convention: sig position
// 0 is the FIRST forward arg in forward form and the TOP of stack in
// stack form, so the stack form is written reversed).
func TestEmitMirrorFormsIdentical(t *testing.T) {
	a, _ := compile(t, `add 1 2`)
	b, _ := compile(t, `2 1 add`)
	if a == "" || a != b {
		t.Errorf("mirror forms diverged:\nforward:\n%s\nstack:\n%s", a, b)
	}
}

// The swap form `1 add 2` and ITS mirror `1 2 add` also lower
// identically — but to DIFFERENT code than `add 1 2`: the swap form
// binds sig[0] from the forward side (the 2), the forward form binds
// sig[0] to the first forward arg (the 1). Two equivalence classes,
// one per binding; identical results here only because add is
// commutative. Canonicalising them to one form would change the
// semantics of non-commutative words.
func TestEmitSwapFormClassIdentical(t *testing.T) {
	a, _ := compile(t, `1 add 2`)
	b, _ := compile(t, `1 2 add`)
	if a == "" || a != b {
		t.Errorf("swap-form class diverged:\nswap:\n%s\nstack:\n%s", a, b)
	}
	fwd, _ := compile(t, `add 1 2`)
	if fwd == a {
		t.Errorf("forward and swap forms lowered identically — distinct bindings must not be conflated")
	}
}

// Negative: every beyond-Stage-1 construct is refused with a precise
// reason — never lowered wrongly.
func TestEmitRefusals(t *testing.T) {
	cases := []struct {
		src        string
		wantReason string
	}{
		{`if true [1] [2]`, "code-body word if"},
		{`def f fn [[n:Integer] [Integer] [n add 1]] f 1`, "user fn call f"},
		{`for 3 [i]`, "code-body word for"},
		{`do [1 add 2]`, "code-body word do"},
	}
	for _, c := range cases {
		got, reason := compile(t, c.src)
		if got != "" {
			t.Errorf("%q compiled but must be refused at Stage 1:\n%s", c.src, got)
			continue
		}
		if !strings.Contains(reason, c.wantReason) {
			t.Errorf("%q refused with %q, want reason containing %q", c.src, reason, c.wantReason)
		}
	}
}

// Negative: a polymorphic (partitioned) site is classified and
// refused — later stages emit CALL_NATIVE_POLY here.
func TestEmitRefusesPolySite(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(`def y if (1 gt 0) [1] ['s'] y add 1`)
	if err != nil {
		t.Fatal(err)
	}
	if prog != nil {
		t.Fatalf("polymorphic program compiled at Stage 1:\n%s", prog.Disassemble())
	}
	// The `if` marks first (code-body); the load-bearing assertion is
	// refusal, with either the control-flow or the poly reason.
	if !strings.Contains(reason, "code-body") && !strings.Contains(reason, "polymorphic") {
		t.Errorf("unexpected refusal reason %q", reason)
	}
}
