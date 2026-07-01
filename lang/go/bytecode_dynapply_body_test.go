package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestClosureBodyUnappliedFnValueSound pins the fix for an off-corpus MISCOMPILE the
// langspec differential is blind to: a captured/param comparator applied as
// `(a b comp)` inside a higher-order CLOSURE body leaves the residual [a, b, comp]
// with the fn VALUE on top. The closure's top-taking handler (each/fold/scan) was
// trimming to that top and mapping every element to the UNAPPLIED comp —
// `[1 2] each [(x x comp)]` interpreted to [0 0] but COMPILED to [fn, fn]. The fix
// refuses such a closure body (sound interpreter fallback) — mirroring the fn-body
// unapplied-fn-value refusal and resolveDynamicApply's main-residual refusals — so
// compile == interpret holds. A SOLE inert fn-reference body still compiles (it maps
// every element to the reference, which is a concrete const, not an unapplied apply).
func TestClosureBodyUnappliedFnValueSound(t *testing.T) {
	// compile == interpret MUST hold (fallback allowed) for the comparator-apply
	// shapes — the exact off-corpus miscompile and its siblings across each/fold/scan.
	// The captured-comparator apply now LOWERS natively (OpCallDynTrailTop), so it
	// must compile WITHOUT a fallback island AND RunCompiledStrict == Run — the
	// off-corpus gate for the trailing-fn-value lowering's exact arg-ordering
	// (`(x 2 comp)` binds the comparator's first param to the TOP arg, like the
	// interpreter's paren). want is the program RESIDUAL (a slice), single wrapped.
	strict := []struct{ name, src, want string }{
		{"each captured comparator apply",
			`def f fn [[comp:Function][List][ [1 2] each [ var [[x] (x x comp) ] ] ]] cmp/r f`, "[[0 0]]"},
		{"fold captured comparator apply",
			`def f fn [[comp:Function][Integer][ 0 fold [ var [[acc x] (x x comp) ] ] [1 2] ]] cmp/r f`, "[0]"},
		{"scan captured comparator apply",
			`def f fn [[comp:Function][List][ scan [ var [[acc x] (acc x comp) ] ] [3 1 2] ]] cmp/r f`, "[[3 -1 1]]"},
		{"each comparator apply, asymmetric args (arg-order gate)",
			`def f fn [[comp:Function][List][ [3 1] each [ var [[x] (x 2 comp) ] ] ]] cmp/r f`, "[[1 -1]]"},
		{"fn-body comparator apply",
			`def f fn [[comp:Function a:Integer b:Integer][Integer][ (a b comp) ]] 5 3 cmp/r f`, "[-1]"},
		{"3-arg trailing apply",
			`def g fn [[f:Function][Integer][ def a 1 def b 2 def c 3 (a b c f) ]] def add3 fn [[x:Integer y:Integer z:Integer][Integer][(x add (y add z))]] add3/r g`, "[6]"},
		{"lambda (compiled-closure) comparator apply",
			`def f fn [[comp:Function][List][ [3 1] each [ var [[x] (x 2 comp) ] ] ]] ([b:Any a:Any] => [(a cmp b)]) f`, "[[1 -1]]"},
		{"trailing apply with a DYNAMIC arg (comp still the fn, not the leading dynamic)",
			`def f fn [[comp:Function][List][ def arr (flex [5 3]) [0 1] each [ var [[x] ((arr get x) 4 comp) ] ] ]] cmp/r f`, "[[1 -1]]"},
		// The comparator apply BOUND TO A DEF-LOCAL (`def c (a b comp)`) then consumed
		// by `if (c gt 0)` — an INTERMEDIATE apply, not the body's trailing residual.
		// Recorded as a RecordDynApply EVENT so it seats like any computed result; this
		// is the def-bound-dynamic-apply leaf that gates 8/11 comparison sorts.
		{"def-bound comparator apply feeding an if (2 dynamic args)",
			`def f fn [[comp:Function][List][ def arr (flex [3 1 2]) [0 1] each [ var [[i] def c ((arr get i) (arr get (i add 1)) comp) if (c gt 0) [9] [0] ] ] ]] cmp/r f`, "[[9 0]]"},
		{"def-bound comparator apply, result used twice",
			`def f fn [[comp:Function][Integer][ def c (5 3 comp) (c add c) ]] cmp/r f`, "[2]"},
		// A branch ARM whose result is an ENCLOSING-scope value-def (`[g]` / `[c]`) — the
		// value lives on the PARENT sim, unreachable from the arm's own fragment sim, so
		// planValueDefLocals must PROMOTE it (a fragResult that is not fragInternal) and
		// lowerFragment re-resolves the arm's captured opEvent out to the local. Clears
		// the comb sort + the Stage-3 "branch reads enclosing computation" cluster.
		{"branch arm result is an enclosing computed value-def",
			`def f fn [[n:Integer][Integer][ def g (n add 5) def gg (if (g lt 1) [1] [g]) gg ]] 3 f`, "[8]"},
		{"branch arm result is an enclosing dynApply value-def",
			`def f fn [[comp:Function][Integer][ def c (5 3 comp) if (c gt 0) [c] [0] ]] cmp/r f`, "[1]"},
	}
	for _, c := range strict {
		t.Run("strict/"+c.name, func(t *testing.T) {
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

	// A SOLE inert fn-reference body (mapping every element to the reference) is a
	// concrete const, NOT an unapplied apply — it must NOT be over-refused into a
	// wrong result; compile == interpret (fallback allowed).
	t.Run("sound/sole inert fn-ref body", func(t *testing.T) {
		src := `[1 2] each [cmp/r]`
		a, _ := New()
		got, _, err := a.RunCompiled(src)
		b, _ := New()
		want, werr := b.Run(src)
		if (err == nil) != (werr == nil) {
			t.Fatalf("error mismatch: compiled=%v interp=%v", err, werr)
		}
		if err == nil && fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("compiled %v != interpreter %v", got, want)
		}
	})
}
