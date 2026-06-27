package lang

import (
	"fmt"
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
	// want is the program RESIDUAL (a slice), so a single value prints wrapped.
	sound := []struct{ name, src, want string }{
		{"each captured comparator apply",
			`def f fn [[comp:Function][List][ [1 2] each [ var [[x] (x x comp) ] ] ]] cmp/r f`, "[[0 0]]"},
		{"fold captured comparator apply",
			`def f fn [[comp:Function][Integer][ 0 fold [ var [[acc x] (x x comp) ] ] [1 2] ]] cmp/r f`, "[0]"},
		{"scan captured comparator apply",
			`def f fn [[comp:Function][List][ scan [ var [[acc x] (acc x comp) ] ] [3 1 2] ]] cmp/r f`, "[[3 -1 1]]"},
		{"each comparator apply over param values",
			`def f fn [[comp:Function][List][ [3 1] each [ var [[x] (x 2 comp) ] ] ]] cmp/r f`, "[[1 -1]]"},
	}
	for _, c := range sound {
		t.Run("sound/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src) // fallback allowed
			b, _ := New()
			want, werr := b.Run(c.src)
			if (err == nil) != (werr == nil) {
				t.Fatalf("error mismatch: compiled=%v interp=%v", err, werr)
			}
			if err == nil {
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
				}
				if fmt.Sprint(want) != c.want {
					t.Errorf("interp got %v, want %s", want, c.want)
				}
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
