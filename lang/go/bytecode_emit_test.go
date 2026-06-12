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
		// `1 add 2` is the k=1 split of the (2,1) assignment — the
		// forward 2 fills sig[0], the stack-prefix 1 fills sig[1]
		// (one uniform rule: forward until the barrier, then
		// backward). Its push order therefore differs from
		// `add 1 2`, the all-forward spelling of the (1,2)
		// assignment; same sum only because add commutes.
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

// One split rule, one bytecode per ASSIGNMENT: every split of the
// same sig-order assignment lowers identically. `1 add 2` (k=1
// split), `add 2 1` (all-forward), and `1 2 add` (all-stack) all
// assign sig[0]=2, sig[1]=1 — identical code. `add 1 2` is the
// (1,2) assignment — a different program (observable on
// non-commutative words: `10 sub 3` = 7, `sub 10 3` = -7), so it
// must NOT be conflated with the other class.
func TestEmitSplitFormsIdentical(t *testing.T) {
	a, _ := compile(t, `1 add 2`)
	b, _ := compile(t, `1 2 add`)
	c, _ := compile(t, `add 2 1`)
	if a == "" || a != b || a != c {
		t.Errorf("splits of one assignment diverged:\nmixed:\n%s\nstack:\n%s\nforward:\n%s", a, b, c)
	}
	fwd, _ := compile(t, `add 1 2`)
	if fwd == a {
		t.Errorf("the (1,2) and (2,1) assignments lowered identically — distinct assignments must not be conflated")
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
