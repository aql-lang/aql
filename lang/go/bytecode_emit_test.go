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
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
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
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		{`0 add 7 sub 3`, `0000 PUSH_CONST  k1   ; 0 (Integer)
0001 PUSH_CONST  k0   ; 7 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; sub (Number, Number)
; consts=3 types=0 sigs=2 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// A paren result feeding the next call: the prior result stays
		// on the simulated stack; only the literal is pushed.
		{`(1 add 2) mul 3`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; mul (Number, Number)
; consts=3 types=0 sigs=2 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// Top-level strings are stripped to carriers by check mode;
		// RecordStrip preserves the originals for interning.
		{`'a' add 'b'`, `0000 PUSH_CONST  k1   ; 'a' (ProperString)
0001 PUSH_CONST  k0   ; 'b' (ProperString)
0002 CALL_NATIVE s0   ; add (Scalar, Scalar)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
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
; consts=4 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
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
; consts=3 types=0 sigs=3 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// Literal-substitution def: x resolves to the interned literal
		// through value provenance — the report's §5.2 inline case.
		{`def x 1 x add 2`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
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
; consts=4 types=0 sigs=1 fallbacks=0 fns=0 max-stack=3 locals=1
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

	// Refusal: a closure ESCAPING as a value (returned, then called
	// through the binding) is a fn-value call site — Stage 4
	// territory. Locally-called closures compile (capture slots);
	// see TestEmitClosureCaptureSlots.
	if _, r3 := compile(t, `def mk fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]] def a5 (mk 5) a5 3`); r3 == "" {
		t.Error("escaping closure compiled but must refuse")
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

// Stage 3 completion: mutual tail recursion. The forward-reference
// rescue clears the checker FP, both bodies compile as units at the
// top-level call site (each is fully defined by then), and the arm
// tail calls lower as cross-unit TAIL_CALL_USER.
func TestEmitMutualTailRecursion(t *testing.T) {
	got, reason := compile(t, `def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 10`)
	if reason != "" {
		t.Fatalf("mutual tail recursion uncompilable: %s", reason)
	}
	if !strings.Contains(got, "TAIL_CALL_USER f1") || !strings.Contains(got, "TAIL_CALL_USER f0") {
		t.Errorf("mutual tails not lowered as cross-unit TAIL_CALL_USER:\n%s", got)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("expected two fn units:\n%s", got)
	}
}

// Mutual tail recursion runs in O(1) frames under a tight ceiling
// (the language guarantee crosses units); mutual NON-tail recursion
// (a pending add below each call) stacks frames and exhausts loudly.
func TestRunCompiledMutualTailGuarantee(t *testing.T) {
	tight := Options{Tape: TapeOptions{InitialSize: 64, MaxGrows: 1, GrowthFactor: 2.7}}
	a, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 1000000`)
	if err != nil {
		t.Fatalf("deep mutual tail recursion: %v", err)
	}
	if !compiled {
		t.Fatal("mutual tail recursion fell back to the interpreter")
	}
	// convertResults renders Booleans as strings.
	if len(out) != 1 || out[0] != "true" {
		t.Fatalf("isev 1000000 = %v, want true", out)
	}

	b, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	_, compiled2, err2 := b.RunCompiled(`def od fn [[n:Integer] [Integer] [if (n eq 0) [0] [1 add (ev (n sub 1))]]] def ev fn [[n:Integer] [Integer] [if (n eq 0) [0] [1 add (od (n sub 1))]]] ev 100000`)
	if !compiled2 {
		t.Fatal("mutual non-tail program fell back to the interpreter")
	}
	if err2 == nil || !strings.Contains(err2.Error(), "tape_exhausted") {
		t.Fatalf("deep mutual non-tail recursion under tight ceiling = %v, want tape_exhausted", err2)
	}
}

// Stage 3 completion: closures. Captures ride as hidden trailing
// param slots — the construction site supplies the enclosing frame's
// values, a recursive call re-passes the frame's own capture slots
// (construction-time snapshot semantics).
func TestEmitClosureCaptureSlots(t *testing.T) {
	// lang/spec/recursion.tsv §6: a captured body-local stays visible
	// through a chain of recursive tail calls.
	got, reason := compile(t, `def outc fn [[] [Integer] [def x 7 def goc fn [[n:Integer] [Integer] [if (n lte 0) [x] [goc (n sub 1)]]] goc 3]] outc`)
	if reason != "" {
		t.Fatalf("recursive closure uncompilable: %s", reason)
	}
	if !strings.Contains(got, "goc/2") {
		t.Errorf("capture slot not added to the unit's params:\n%s", got)
	}

	// Runtime parity, with a non-commutative op so slot ORDER is
	// pinned, and a param (not a literal) as the captured value.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def oc fn [[m:Integer] [Integer] [def gc fn [[n:Integer] [Integer] [m sub n]] gc 3]] oc 10`)
	if err != nil {
		t.Fatalf("closure over param: %v", err)
	}
	if !compiled {
		t.Fatal("closure fell back to the interpreter")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want, err := b.Run(`def oc fn [[m:Integer] [Integer] [def gc fn [[n:Integer] [Integer] [m sub n]] gc 3]] oc 10`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(want) != 1 || out[0] != want[0] {
		t.Fatalf("compiled %v != interpreted %v", out, want)
	}
}

// The fn-unit key includes the construction site: redefining a
// same-name same-sig fn must bind later call sites to the LATER
// body. (Without the site in the key, both calls ran the FIRST
// definition's unit — caught against the interpreter's 2 3.)
func TestCompiledFnRedefinitionBindsLatest(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def rdf fn [[n:Integer] [Integer] [n add 1]] (rdf 1) def rdf fn [[n:Integer] [Integer] [n add 2]] (rdf 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if !compiled {
		t.Fatal("redefinition program fell back to the interpreter")
	}
	if len(out) != 2 || out[0] != int64(2) || out[1] != int64(3) {
		t.Fatalf("redefined fn calls = %v, want [2 3]", out)
	}
}

// A tail-recursive SPIN never grows the stack or the frame list, so
// neither ceiling trips — the VM's step budget must catch it with
// the interpreter's evaluation_limit taxonomy instead of hanging.
func TestRunCompiledStepBudget(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, compiled, err := a.RunCompiled(`def spinf fn [[n:Integer] [Integer] [spinf n]] spinf 0`)
	if !compiled {
		t.Fatal("tail spin fell back to the interpreter")
	}
	if err == nil || !strings.Contains(err.Error(), "evaluation_limit") {
		t.Fatalf("tail spin = %v, want evaluation_limit", err)
	}
}

// Stage 4: generic fns compile one unit per memoised instantiation
// (the unit key carries the instantiated arg types); generic units
// stay OUT of tail marking, mirroring the interpreter's HasGen
// exclusion from frame elision.
func TestEmitGenericInstantiation(t *testing.T) {
	got, reason := compile(t, `def idg gen [T] fn [[x:T] [T] [x]] (idg 5) (idg "a")`)
	if reason != "" {
		t.Fatalf("generic fn uncompilable: %s", reason)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("two instantiations must compile two units:\n%s", got)
	}

	// Generic recursion compiles but is NOT tail-marked.
	got2, reason2 := compile(t, `def cntg gen [T] fn [[x:T n:Integer] [Integer] [if (n lte 0) [n] [cntg x (n sub 1)]]] cntg "a" 5`)
	if reason2 != "" {
		t.Fatalf("generic recursion uncompilable: %s", reason2)
	}
	if strings.Contains(got2, "TAIL_CALL_USER") {
		t.Errorf("generic unit was tail-marked (HasGen exclusion):\n%s", got2)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def pickg gen [T U] fn [[a:T b:U] [U] [b]] pickg 1 "x"`)
	if err != nil || !compiled {
		t.Fatalf("generic call: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "x" {
		t.Fatalf("pickg 1 \"x\" = %v, want x", out)
	}
}

// Stage 4: type operands lower as PUSH_TYPE — resolved through the
// registry's TypeTable at RUN time so the handler always receives
// the CANONICAL node (never a stale pooled copy), for builtins and
// check-pass-minted user types alike.
func TestEmitTypeOperands(t *testing.T) {
	got, reason := compile(t, `5 is Integer`)
	if reason != "" {
		t.Fatalf("is-with-type-operand uncompilable: %s", reason)
	}
	if !strings.Contains(got, "PUSH_TYPE") || !strings.Contains(got, "; Integer") {
		t.Errorf("type operand not lowered as PUSH_TYPE:\n%s", got)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def Pt refine Integer def p:Pt 5 p is Pt`)
	if err != nil || !compiled {
		t.Fatalf("minted-type operand: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "true" {
		t.Fatalf("p is Pt = %v, want true", out)
	}

	// Structural type bodies (make's operand) ride the const pool —
	// payloads are pointer-backed and the minted node is reached via
	// the Parent pointer, which stays canonical.
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out2, compiled2, err := b.RunCompiled(`def M refine Record [k:String] end make M {k:"x"}`)
	if err != nil || !compiled2 {
		t.Fatalf("make with record body: compiled=%v err=%v", compiled2, err)
	}
	if len(out2) != 1 || out2[0] != "{k:'x'}" {
		t.Fatalf("make M = %v, want {k:'x'}", out2)
	}

	// Negative: a type body whose interior holds a check-mode CARRIER
	// (a generic instantiation over a class body with a stripped
	// default) must refuse — baking the analysis artefact in would
	// render `r:Float` where the interpreter rebuilds `r:1.0` (caught
	// by the differential gate).
	if _, r := compile(t, `def Shape surface {area: (fnsig [[Self] [Float]])} def Circle class {r:1.0} def area fn [[c:Circle] [Float] [1.0]] Circle exposes Shape def Holder gen [(T extends Shape)] refine Record [item:T] end Holder of [Circle]`); r == "" {
		t.Error("carrier-tainted type body compiled but must refuse")
	}
}

// Stage 4: a macro call site lowers to its EXPANSION — the macro
// installs during the check pass (RunInCheckMode construction, like
// fn/fnsig), its use expands on the tape (execMacro), and the
// recording pass sees only the expanded stream (plan R6 #29: raw-form
// operand spans are never lowered pre-expansion).
func TestEmitMacroExpansionGolden(t *testing.T) {
	got, reason := compile(t, `def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])  twice 5`)
	if reason != "" {
		t.Fatalf("macro site uncompilable: %s", reason)
	}
	want := `0000 PUSH_CONST  k0   ; 5 (Integer)
0001 PUSH_CONST  k0   ; 5 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=1 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`
	if got != want {
		t.Errorf("macro expansion lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def unless (macro [[c body] [ quote [ if unquote c [0] unquote body ] ]])  unless false [42]`)
	if err != nil || !compiled {
		t.Fatalf("unless macro: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != int64(42) {
		t.Fatalf("unless false [42] = %v, want 42", out)
	}
}

// Stage 4: a module dot-access call compiles to a bare CALL_NATIVE on
// the INNER native's signature — `MathUtil.sqrt 16.0` is the tokens
// `MathUtil get sqrt 16.0`; the import and the get resolution run
// during the check pass and are ELIDED (the resolved wrapper's
// trivial-delegation dispatch records the real call, through the same
// engine/registry the interpreter's short-circuit uses).
func TestEmitModuleCallLowering(t *testing.T) {
	got, reason := compile(t, `"aql:math-util" import end MathUtil.sqrt 16.0`)
	if reason != "" {
		t.Fatalf("module call uncompilable: %s", reason)
	}
	want := `0000 PUSH_CONST  k0   ; 16.0 (Float)
0001 CALL_NATIVE s0   ; sqrt (Float)
; consts=1 types=0 sigs=1 fallbacks=0 fns=0 max-stack=1 locals=0
`
	if got != want {
		t.Errorf("module call lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`"aql:math-util" import end MathUtil.max 5.0 9.0`)
	if err != nil || !compiled {
		t.Fatalf("MathUtil.max: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "9.0" {
		t.Fatalf("max 5.0 9.0 = %v, want 9.0", out)
	}

	// Negative: a get whose RESULT the checker cannot type (dynamic
	// Any — a runtime field read) refuses and falls back; the program
	// still runs correctly through the interpreter.
	if _, r := compile(t, `def m {a:1} m.a`); r == "" {
		t.Error("dynamic-result get compiled but must refuse (checker types it Any)")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out2, compiled2, err := b.RunCompiled(`def m {a:1} m.a`)
	if err != nil || compiled2 {
		t.Fatalf("dynamic get fallback: compiled=%v err=%v", compiled2, err)
	}
	if len(out2) != 1 || out2[0] != int64(1) {
		t.Fatalf("m.a fallback = %v, want 1", out2)
	}
}

// Stage 4: a multi-overload user fn monomorphizes per call SHAPE —
// the checker resolves which overload each statically-typed call site
// selects, and each becomes its own code unit (the memo key carries
// the arg types). No runtime poly dispatch is needed when the checker
// can pick; the genuinely-polymorphic case (a dynamic arg reaching
// several overloads) is Stage-5 fallback territory.
func TestEmitMultiOverloadMonomorphises(t *testing.T) {
	got, reason := compile(t, `def f fn [[a:Integer][Integer][a add 1] [a:String][String][a add "!"]] (f 5) (f "x")`)
	if reason != "" {
		t.Fatalf("multi-overload fn uncompilable: %s", reason)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("two call shapes must compile two units:\n%s", got)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def f fn [[a:Integer][Integer][a add 1] [a:String][String][a add "!"]] (f 5) (f "x")`)
	if err != nil || !compiled {
		t.Fatalf("multi-overload run: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 2 || out[0] != int64(6) || out[1] != "x!" {
		t.Fatalf("multi-overload = %v, want [6 x!]", out)
	}
}

// Stage 4: the minilang `mini` word lowers to a bare CALL_NATIVE —
// it is a deterministic expansion the recording pass treats like any
// other native. (A trailing dynamic accessor on the result — `.n` —
// is Stage-5 fallback territory, asserted as a refusal here.)
func TestEmitMinilangCompiles(t *testing.T) {
	got, reason := compile(t, `"aql:minilang" import end "a1b2c3" mini re "\\d"`)
	if reason != "" {
		t.Fatalf("mini expansion uncompilable: %s", reason)
	}
	if !strings.Contains(got, "CALL_NATIVE") || strings.Contains(got, "CALL_USER") {
		t.Errorf("mini did not lower to a native call:\n%s", got)
	}
	// The dynamic-result accessor refuses (Stage-5 fallback boundary).
	if _, r := compile(t, `"aql:minilang" import end ("a1b2c3" mini re "\\d").n`); r == "" {
		t.Error("dynamic accessor on mini result compiled but must refuse")
	}
}

// Stage 4: an fn-value pulled from a map field and called (`m.f 5`)
// is F4 — the checker types the get result as dynamic Any, so the
// recorder refuses and the program falls back to the interpreter with
// the correct result (the plan's sanctioned F4 fallback).
func TestEmitFnValueCallFallsBack(t *testing.T) {
	if _, r := compile(t, `def m {f: (fn [[a:Integer][Integer][a add 1]])}  m.f 5`); r == "" {
		t.Error("fn-value-from-map call compiled but must fall back (F4)")
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def m {f: (fn [[a:Integer][Integer][a add 1]])}  m.f 5`)
	if err != nil || compiled {
		t.Fatalf("fn-value fallback: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != int64(6) {
		t.Fatalf("m.f 5 fallback = %v, want 6", out)
	}
}

// Stage 5: a code-body higher-order word compiles as an interpreter
// ISLAND (OpFallback) — a self-contained span re-run through a
// sub-engine, with the compiled code on either side intact. The body
// must reference only VM-resolvable words; a body reading a check-time
// `def` (a carrier at run time) refuses to whole-program fallback.
func TestEmitFallbackIsland(t *testing.T) {
	got, reason := compile(t, `each [mul 2] [1 2 3]`)
	if reason != "" {
		t.Fatalf("each island uncompilable: %s", reason)
	}
	if !strings.Contains(got, "FALLBACK") || !strings.Contains(got, "fallbacks=1") {
		t.Errorf("each did not lower to a FALLBACK island:\n%s", got)
	}

	// Runtime parity across each / fold / scan.
	for _, c := range []struct {
		src  string
		want any
	}{
		{`each [mul 2] [1 2 3]`, "[2 4 6]"},
		{`[1 2 3] each [mul 2]`, "[2 4 6]"},
		{`fold [add] [1 2 3 4] 0`, int64(10)},
		{`scan [add] [1 2 3]`, "[1 3 6]"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := a.RunCompiled(c.src)
		if err != nil || !compiled {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, err)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q compiled = %v, want %v", c.src, out, c.want)
		}
	}

	// Negative: a body reading a value-def refuses (the def is a carrier
	// at VM run time) — whole-program fallback, correct result.
	if _, r := compile(t, `def n 10 each [add n] [1 2 3]`); r == "" {
		t.Error("each with a def-referencing body compiled but must refuse")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := b.RunCompiled(`def n 10 each [add n] [1 2 3]`)
	if err != nil || compiled {
		t.Fatalf("def-body fallback: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "[11 12 13]" {
		t.Fatalf("def-body fallback = %v, want [11 12 13]", out)
	}

	// A module-qualified higher-order word with a code body dispatches
	// the inner native through a sub-registry; the core-dispatch guard
	// keeps any bare-name re-run faithful. It runs correctly either way.
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	mout, _, merr := c.RunCompiled(`"aql:array-util" import end ArrayUtil.group ['a' 'b' 'a'] [1 2 3]`)
	if merr != nil {
		t.Fatalf("module group: %v", merr)
	}
	d, _ := New()
	iout, _ := d.Run(`"aql:array-util" import end ArrayUtil.group ['a' 'b' 'a'] [1 2 3]`)
	if len(mout) != len(iout) || (len(mout) == 1 && mout[0] != iout[0]) {
		t.Fatalf("module group compiled=%v interpreted=%v", mout, iout)
	}
}
