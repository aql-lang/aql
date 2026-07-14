package lang

import (
	"fmt"
	"testing"
)

// The for-lowering's unknown-provenance refusals (eng emit.go RecordLoop): a
// loop body ending on a value the recorder cannot seat — no producing event,
// no frame local, not materialisable as a const (a Module instance bound by a
// module-scope def is the minimal in-repo shape) — refuses soundly in BOTH
// net arms: the multi-value residual branch (net drivers) and the
// single-value branch. Each pairs the reason pin with interpreter-fallback
// parity, per the "slow, not wrong" doctrine.
func TestForBodyUnknownProvenanceRefuses(t *testing.T) {
	cases := []struct{ name, src string }{
		{"multi-value residual", `def m (module [export "X" {a: 1}]) for 2 [9 m]`},
		{"single-value", `def m (module [export "X" {a: 1}]) for 2 [m]`},
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
			if prog != nil || reason != "for: body result of unknown provenance" {
				t.Fatalf("prog=%v reason=%q — want the unknown-provenance refusal", prog, reason)
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
			gotI, err := i.Run(c.src)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
				t.Errorf("fallback parity: compiled-path %v != interp %v", gotC, gotI)
			}
		})
	}
}
