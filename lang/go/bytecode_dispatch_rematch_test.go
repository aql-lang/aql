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

// TestDispatchRematchWideWindowStaysRefused — the written-tuple bound: the
// local-add shape's match examined a WIDER window (3 positions) than the
// tuple its error renders (1 stack value), which the runtime rebuild cannot
// yet reproduce faithfully — it must keep the whole-program refusal (and the
// interpreter fallback raises the canonical error).
func TestDispatchRematchWideWindowStaysRefused(t *testing.T) {
	const src = `def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil || !strings.Contains(reason, "unmatched dispatch recovered at add") {
		t.Fatalf("prog=%v reason=%q — the wide-window shape must stay refused until the window-bound lands", prog, reason)
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
