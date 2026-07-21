package lang

import (
	"fmt"
	"testing"
)

// A KNOWN COMPILABLE-SUBSET GAP, pinned so the eventual fix is visible: a
// Map-typed local consumed by a user-fn call inside an `if` BRANCH loses
// its operand provenance for any LATER user-fn call after the join — the
// recorder's resolveOperand fails and the program refuses with "fn call
// operand of unknown provenance", even when both branches are identical
// calls, and even when only ONE branch consumes the local. The boundary
// is sharp (the compiling controls below): a scalar local survives the
// identical shape, the same map local survives without the `if`, and it
// survives when nothing reuses it after the join — so the gap is
// specifically container operands whose producer seat is lost across a
// branch-body consumption.
//
// Reduced from the alice viewer's render fn (voxgig-aql/alice viewer/,
// dx-report 2026-07-21 "bytecode status"), which had to branch on a
// whole widget list instead of binding its middle pane to compile.
//
// The refusal is SOUND (fallback parity is asserted — slow, not wrong)
// but undesired: when the emitter learns to reseat container operands
// across branch consumption, flip the refusal cases below into the
// compiling table and keep the parity asserts.
func TestIfBranchMapOperandKnownGapRefuses(t *testing.T) {
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	cases := []struct{ name, src string }{
		{"both branches consume, reused after join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab (state get "tabs")
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`},
		{"one branch consumes, reused after join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ 5 ])
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog != nil || reason != "fn call operand of unknown provenance" {
				t.Fatalf("prog=%v reason=%q — the known if-branch container-operand gap no longer refuses; move this case to the compiling controls and pin the new behavior", prog, reason)
			}

			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotC, compiled, err := b.RunCompiled(c.src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if compiled {
				t.Fatal("the refused program must fall back to the interpreter")
			}
			i, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := i.RunInterp(c.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
				t.Errorf("fallback parity: compiled-path %v != interp %v", gotC, gotI)
			}
		})
	}
}

// The compiling controls that pin the gap's boundary: each is one edit
// away from the refusing shape and must COMPILE and yield the
// interpreter's result on the VM. If one of these starts refusing, the
// gap has widened — that is a regression, not a doc update.
func TestIfBranchOperandBoundaryCompiles(t *testing.T) {
	cases := []struct {
		name, src string
		want      string
	}{
		{"no reuse after the join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add 1
]]
render {mode: "x", tabs: {a: 1}}`, "[2]"},
		{"scalar local, same shape", `
def mkm fn [[m:Integer] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "n") otherwise 0)
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add (mkm (tab))
]]
render {mode: "x", n: 3}`, "[2]"},
		{"no if between the uses", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (mkm (tab))
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`, "[2]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog == nil {
				t.Fatalf("control refused (%q) — the if-branch container-operand gap has widened", reason)
			}

			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			got, compiled, err := b.RunCompiled(c.src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("control must run on the VM")
			}
			if fmt.Sprintf("%v", got) != c.want {
				t.Errorf("compiled result %v, want %s", got, c.want)
			}
		})
	}
}
