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
; consts=2 sigs=1 fns=0 max-stack=2 locals=0
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
; consts=2 sigs=1 fns=0 max-stack=2 locals=0
`},
		{`0 add 7 sub 3`, `0000 PUSH_CONST  k1   ; 0 (Integer)
0001 PUSH_CONST  k0   ; 7 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; sub (Number, Number)
; consts=3 sigs=2 fns=0 max-stack=2 locals=0
`},
		// A paren result feeding the next call: the prior result stays
		// on the simulated stack; only the literal is pushed.
		{`(1 add 2) mul 3`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; mul (Number, Number)
; consts=3 sigs=2 fns=0 max-stack=2 locals=0
`},
		// Top-level strings are stripped to carriers by check mode;
		// RecordStrip preserves the originals for interning.
		{`'a' add 'b'`, `0000 PUSH_CONST  k1   ; 'a' (ProperString)
0001 PUSH_CONST  k0   ; 'b' (ProperString)
0002 CALL_NATIVE s0   ; add (Scalar, Scalar)
; consts=2 sigs=1 fns=0 max-stack=2 locals=0
`},
		// `if` lowers to JMP_IF_FALSE / JMP: condition code first, a
		// branch per fragment, the join value on the stack for the
		// downstream call. Const-only branches are single pushes.
		{`if (1 gt 0) [10] [20]`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 0 (Integer)
0002 CALL_NATIVE s0   ; gt (Any, Any)
0003 JMP_IF_FALSE -> 0006
0004 PUSH_CONST  k2   ; 10 (Integer)
0005 JMP         -> 0007
0006 PUSH_CONST  k3   ; 20 (Integer)
; consts=4 sigs=1 fns=0 max-stack=2 locals=0
`},
		// The branch result feeds the downstream mul; a stripped
		// Boolean literal condition compiles as PUSH_CONST + jump
		// (CoerceBoolean truthiness at run time).
		{`def y if (2 gt 1) [1 add 2] [9] y mul 2`, `0000 PUSH_CONST  k1   ; 2 (Integer)
0001 PUSH_CONST  k0   ; 1 (Integer)
0002 CALL_NATIVE s0   ; gt (Any, Any)
0003 JMP_IF_FALSE -> 0008
0004 PUSH_CONST  k0   ; 1 (Integer)
0005 PUSH_CONST  k1   ; 2 (Integer)
0006 CALL_NATIVE s1   ; add (Number, Number)
0007 JMP         -> 0009
0008 PUSH_CONST  k2   ; 9 (Integer)
0009 PUSH_CONST  k1   ; 2 (Integer)
0010 CALL_NATIVE s2   ; mul (Number, Number)
; consts=3 sigs=3 fns=0 max-stack=2 locals=0
`},
		// Literal-substitution def: x resolves to the interned literal
		// through value provenance — the report's §5.2 inline case.
		{`def x 1 x add 2`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 sigs=1 fns=0 max-stack=2 locals=0
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

// Counted loops lower to FOR_SETUP / FOR_NEXT with the iterator as a
// VM local and the body's trailing JMP as the program's only
// back-edge; one value per iteration accumulates on the stack,
// matching the interpreter (`for 3 [i]` → 0 1 2).
func TestEmitForLoopGolden(t *testing.T) {
	got, reason := compile(t, `for 3 [i add 10]`)
	if reason != "" {
		t.Fatalf("for-loop unexpectedly uncompilable: %s", reason)
	}
	// The count form lowers as the range [0, 3, 1]: FOR_SETUP pops
	// start (top), end, step — the parseRange triple.
	want := `0000 PUSH_CONST  k3   ; 1 (Integer)
0001 PUSH_CONST  k2   ; 3 (Integer)
0002 PUSH_CONST  k1   ; 0 (Integer)
0003 FOR_SETUP   l0
0004 FOR_NEXT    -> 0009
0005 PUSH_LOCAL  l0
0006 PUSH_CONST  k0   ; 10 (Integer)
0007 CALL_NATIVE s0   ; add (Number, Number)
0008 JMP         -> 0004
; consts=4 sigs=1 fns=0 max-stack=3 locals=1
`
	if got != want {
		t.Errorf("for lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}
}

// Stage-2 completion shapes: range/negative-step loops, list-form
// conditions, the 2-arg if, and break/continue all compile and any
// downstream consumption of the variadic results refuses.
func TestEmitStage2CompletionShapes(t *testing.T) {
	for _, src := range []string{
		`for [2,5] [i]`,
		`for [5,0,-1] [i]`,
		`if [1 lt 2] [10] [20]`,
		`if (1 gt 0) [7]`,
		`for 5 [if (i gt 2) [break] [i]]`,
		`for 5 [if (i eq 2) [continue] [i]]`,
		// A def-bound range is CONCRETE at check time (def evaluates
		// its body) — statically known bounds compile.
		`def r [1,3] for r [i]`,
	} {
		if got, reason := compile(t, src); got == "" {
			t.Errorf("%q unexpectedly uncompilable: %s", src, reason)
		}
	}
	// Negative: the 2-arg if's result is VARIADIC — consuming it must
	// refuse (only the program residual may absorb it).
	if got, _ := compile(t, `(if (1 gt 0) [7]) add 1`); got != "" {
		t.Errorf("2-arg if result consumption compiled but must refuse:\n%s", got)
	}
	// Negative: break outside any loop refuses.
	if got, _ := compile(t, `if (1 gt 0) [break] [1]`); got != "" {
		t.Errorf("break outside a loop compiled but must refuse:\n%s", got)
	}
	// Negative: a range with a computed element refuses — its bounds
	// are carriers at check time, not literals.
	if got, _ := compile(t, `for [1, (1 add 2)] [i]`); got != "" {
		t.Errorf("computed range compiled but must refuse:\n%s", got)
	}
}

// Loop results are VARIADIC at run time (one value per iteration):
// downstream calls must refuse to consume them — only the program
// residual may absorb the accumulation.
func TestEmitRefusesLoopResultConsumption(t *testing.T) {
	got, reason := compile(t, `(for 3 [i]) size`)
	if got != "" {
		t.Fatalf("loop-result consumption compiled but must refuse:\n%s", got)
	}
	if !strings.Contains(reason, "loop") && !strings.Contains(reason, "provenance") {
		t.Errorf("unexpected refusal reason %q", reason)
	}
}

// Negative: every beyond-Stage-1 construct is refused with a precise
// reason — never lowered wrongly.
func TestEmitRefusals(t *testing.T) {
	cases := []struct {
		src        string
		wantReason string
	}{
		// A branch reading an enclosing computation breaks the closed-
		// fragment rule (Stage 3, with locals).
		{`def y (1 add 2) if (1 gt 0) [y mul 2] [0]`, "branch reads enclosing computation"},
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

// Stage 3: user fns compile as their own code units with params as
// frame locals; the body is compiled against GENERALISED carrier
// args (a call's concrete values must not constant-fold into the
// shared unit); tail-position recursive calls lower to
// TAIL_CALL_USER — the language's tail-call guarantee in compiled
// form.
func TestEmitUserFnAndTailCall(t *testing.T) {
	got, reason := compile(t, `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 10 0`)
	if reason != "" {
		t.Fatalf("tail-recursive fn unexpectedly uncompilable: %s", reason)
	}
	if !strings.Contains(got, "TAIL_CALL_USER") {
		t.Errorf("tail call not lowered as TAIL_CALL_USER:\n%s", got)
	}
	if !strings.Contains(got, "CALL_USER") || !strings.Contains(got, "RET") {
		t.Errorf("user-fn frame opcodes missing:\n%s", got)
	}
	// The generalisation guard: the body must reference its params as
	// locals, never the outer call's constants (n sub 1 with n=10
	// must NOT fold to 9).
	if strings.Contains(got, "; 9 (Integer)") {
		t.Errorf("call-site constant folded into the shared fn unit:\n%s", got)
	}

	// Non-tail recursion also compiles (frames stack).
	got2, reason2 := compile(t, `def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]] fact 10`)
	if reason2 != "" {
		t.Fatalf("factorial unexpectedly uncompilable: %s", reason2)
	}
	if strings.Contains(got2, "TAIL_CALL_USER") {
		t.Errorf("non-tail recursive call wrongly marked tail:\n%s", got2)
	}

	// Refusals: closures and unchecked fns are beyond this slice.
	if _, r3 := compile(t, `def mk fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]] def a5 (mk 5) a5 3`); r3 == "" {
		t.Error("closure compiled but must refuse")
	}
}

// Runtime equality + the tail guarantee under a tight ceiling: deep
// self tail-recursion runs in O(1) frames in compiled mode, while
// equally deep NON-tail recursion exhausts loudly with the shared
// taxonomy.
func TestRunCompiledTailGuarantee(t *testing.T) {
	tight := Options{Tape: TapeOptions{InitialSize: 64, MaxGrows: 1, GrowthFactor: 2.7}}
	a, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 100000 0`)
	if err != nil {
		t.Fatalf("deep tail recursion: %v", err)
	}
	if !compiled {
		t.Fatal("tail-recursive program fell back to the interpreter")
	}
	if len(out) != 1 || out[0] != int64(5000050000) {
		t.Fatalf("s2 100000 0 = %v, want 5000050000", out)
	}

	b, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	_, compiled2, err2 := b.RunCompiled(`def f fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (f (n sub 1))]]] f 100000`)
	if !compiled2 {
		t.Fatal("non-tail program fell back to the interpreter")
	}
	if err2 == nil || !strings.Contains(err2.Error(), "tape_exhausted") {
		t.Fatalf("deep non-tail recursion under tight ceiling = %v, want tape_exhausted", err2)
	}
}
