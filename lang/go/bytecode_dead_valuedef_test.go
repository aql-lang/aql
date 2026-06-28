package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestDeadValueDefDrop pins the fix for a DEAD value-def — `def _ (expr)` bound to a
// name referenced ZERO times — whose result is a USER-fn call or an `if` merge. The
// interpreter binds the result to that name OFF the residual stack, so the value never
// appears in the fn body's residual; the compiler used to LEAVE a user-call / branch
// result on the sim, leaving the body with an extra value ("body leaves extra values
// (Stage 3)"). It now DROPS the dead result (the call's side effects still run). This is
// the recursive sorts' `def _l (… quick-go)` / `def _ (if (n gt 1) [… quick-go] [arr])`
// pattern (quick, merge, heap-ish, slow, stooge, bitonic, sort). compile == interpret;
// compiles natively, RunCompiledStrict == Run. want is the program residual.
func TestDeadValueDefDrop(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// dead USER-call value-def: go mutates the array (side effect), its returned
		// array is bound to a never-read name.
		{"dead user-call value-def (ignored return)",
			`import module [ def go fn [[a:Array] [Array] [ a set 0 99 end a ]] def srt fn [[xs:List] [List] [ def arr (make Array xs) def _ (arr go) (convert List arr) ]] export "M" {srt: srt/r} ] end ([1 2 3] M.srt)`,
			"[[99 2 3]]"},
		// dead BRANCH value-def: the if's merge is bound to a never-read name; the then
		// arm mutates via a (recursive-style) helper.
		{"dead branch value-def (if merge ignored)",
			`import module [ def go fn [[a:Array] [Array] [ a set 1 88 end a ]] def srt fn [[xs:List] [List] [ def arr (make Array xs) def n (xs size) def _ (if (n gt 1) [ arr go ] [ arr ]) (convert List arr) ]] export "M" {srt: srt/r} ] end ([1 2 3] M.srt)`,
			"[[1 88 3]]"},
		// the same at TOP LEVEL (a dead branch value-def over a native call) must also compile.
		{"dead branch value-def, native then-arm",
			`def m (make Array [5 6]) def n 2 def _ (if (n gt 1) [ m set 0 7 end m ] [ m ]) (convert List m)`,
			"[[7 6]]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, refused: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.Run(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
