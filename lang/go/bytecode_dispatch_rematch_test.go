package lang

import (
	"fmt"
	"strings"
	"testing"
)

// OpDispatchRematch (plan Phase 3): a statically-failed dispatch whose window
// held CARRIER operands compiles to a terminal runtime rematch instead of
// refusing the whole program. The six graduated knownRefusals rows (the
// apply.tsv pair + four generics rows) re-match the live values at run time
// and raise the shared rich diagnostic BYTE-IDENTICAL to the interpreter's
// sigError. The corpus differential covers Detail equality; this pin compares
// the FULL rendered error (spans, notes, suggestions).
func TestDispatchRematchRaisesByteIdentical(t *testing.T) {
	rows := []string{
		`def Box gen [T] class {v:T} def f fn [[x:(Box of [Number])] [Integer] [1]] end f (make (Box of [Integer]) {v:1})`,
		`def Box gen [T] class {v:T} def BoxI (Box of [Integer]) def BoxS (Box of [String]) def g fn [[b:BoxI] [Integer] [1]] end g (make BoxS {v:'s'})`,
		`def Box<T> class {value:T} def f fn [[x:Box<Integer>] [Integer] [x dot value]] end f (make Box<String> {value:'s'})`,
		`def Box gen [T] class {value:T} def f fn [[x:(Box of [Integer])] [Integer] [x dot value]] end f (make (Box of [String]) {value:'s'})`,
		`def inc fn [[n:Integer][Integer][n add 1]]  5 inc apply`,
		`def inc fn [[n:Integer][Integer][n add 1]]  5 (ref inc) apply`,
	}
	for _, src := range rows {
		t.Run(fmt.Sprintf("%.40s", src), func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, ran, errC := a.RunCompiled(src)
			if !ran {
				t.Fatal("the row must run COMPILED — the rematch owns the raise, not a fallback")
			}
			if errC == nil {
				t.Fatal("the compiled rematch must raise")
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
				t.Errorf("rematch error is not byte-identical:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
			}
		})
	}
}

// TestDispatchRematchMatchDefers — the soundness arm: when the static
// no-match was WRONG (a fn declared [Integer] returns a Pos-reparented value
// whose runtime tag matches the Pos param), the rematch MATCHES at run time
// and defers to the interpreter (the tail was truncated at the terminal op),
// producing the interpreter's result — slow, not wrong.
func TestDispatchRematchMatchDefers(t *testing.T) {
	const src = `def Pos (refine Integer) def mk fn [[n:Integer][Integer][def y:Pos n y]] def g fn [[p:Pos][Integer][99]] g (mk 5)`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		t.Fatalf("the refined-return shape must compile to a rematch (reason=%q err=%v)", reason, err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	outC, ran, err := b.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if ran {
		t.Error("a runtime MATCH must defer to the interpreter, not keep the truncated compiled run")
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	outI, err := c.Run(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", outC) != fmt.Sprintf("%v", outI) {
		t.Errorf("defer parity: compiled-path %v != interp %v", outC, outI)
	}
}

// TestDispatchRematchWideWindowRendersBounded — the render bound (plan 3a):
// the local-add shape's match examined a WIDER window (3 positions) than the
// tuple its error renders (the single stack value — the forward walk breaks
// at the `true`/`false` WORD tokens). The record gate proves the written
// tuple is the window's leading slice by ID and stamps its length as
// DispatchSpec.NWritten; the compiled rematch re-runs the match over the
// FULL window and renders over window[:NWritten] — byte-identical to the
// interpreter, running COMPILED (formerly a whole-program refusal).
func TestDispatchRematchWideWindowRendersBounded(t *testing.T) {
	const src = `def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		t.Fatalf("the wide-window shape must compile to a render-bounded rematch (reason=%q err=%v)", reason, err)
	}
	if !strings.Contains(prog.Disassemble(), "DISPATCH_REMATCH") {
		t.Fatal("expected a DISPATCH_REMATCH terminal in the lowering")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, ran, errC := b.RunCompiled(src)
	if !ran {
		t.Fatal("the row must run COMPILED — the rematch owns the raise, not a fallback")
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, errI := c.Run(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("render-bounded rematch is not byte-identical:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
	}
}

// TestDispatchRematchVariadicIfOperandStaysRefused — the remaining negative:
// a variadic-if result feeding `each` leaves arm-dependent residual counts
// (then [99] = one value, else [1 2] = two), so no fixed-arity rematch window
// exists and the whole-program refusal stands (fallback parity via the
// interpreter's canonical raise) until the Phase 5 variadic-region lowering
// seats the merged residual.
func TestDispatchRematchVariadicIfOperandStaysRefused(t *testing.T) {
	const src = `def n 0 if (n eq 0) [99] [1 2] each [dup mul]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil || !strings.Contains(reason, "unmatched dispatch recovered at each") {
		t.Fatalf("prog=%v reason=%q — the variadic-if operand shape must stay refused until the variadic-region lowering lands", prog, reason)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, ran, errC := b.RunCompiled(src)
	if ran {
		t.Fatal("refused program must fall back")
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, errI := c.Run(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("fallback parity: compiled-path err %v != interp err %v", errC, errI)
	}
}
