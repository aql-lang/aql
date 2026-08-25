package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestGradualAnyEachFoldScan pins the voxgig dominant leaf: a higher-order
// each/fold/scan over a GRADUAL-Any (Dynamic) collection — statically ambiguous
// between the List and Map overload — used to refuse force-compile ("ambiguous
// overload (List vs Map), no static commit and no poly re-match"). It is sound to
// compile after all: the TOKEN body is shape-generic (both overloads present the
// closure the bare element/value), so the recorder commits the first-reachable
// (List) overload and lowers ONE closure, and the committed handler is runtime-
// robust — it delegates to the sibling collection's iteration by the value's
// concrete type. The SAME compiled closure then drives either shape == the
// interpreter (which dispatches the overload by the runtime type).
//
// The differential corpus has no gradual-collection-over-both-shapes shape, so
// these hand-pinned RunCompiledStrict==Run regressions (incl. one compiled body
// driven by a List AND a Map) are the soundness gate. mkl returns [Any], so its
// result is a Dynamic carrier (the param-Any case is NOT Dynamic and already
// compiled — the refusal needs a fn-result Any).
func TestGradualAnyEachFoldScan(t *testing.T) {
	const pre = `def mkl fn [[x:Any][Any][x]] `

	// MUST compile natively (no island) AND RunCompiledStrict==Run. `want` is the
	// program RESIDUAL (a slice), so a single-value residual prints `[v]`. The body
	// is `[mul 2]` (NOT `[dup add]`, whose `add` over two gradual-Any operands is a
	// separate unrelated leaf that islands on its own).
	strict := []struct{ name, src, want string }{
		// The strongest case: ONE compiled fn body whose each iterates a Dynamic-Any
		// receiver, called with a List and then a Map — proves the committed list
		// overload's handler drives the Map at run time via cross-delegation.
		{"each one-body list+map",
			pre + `def g fn [[x:Any][Any][ def y (mkl x) (y each [mul 2]) ]] [(g [1 2 3]) (g {a:1 b:2})]`,
			"[[[2 4 6] {a:2 b:4}]]"},
		{"each dynamic list", pre + `mkl [1 2 3] each [mul 2]`, "[[2 4 6]]"},
		{"each dynamic map", pre + `mkl {a:1 b:2} each [mul 2]`, "[{a:2 b:4}]"},

		{"fold-init one-body list+map",
			pre + `def g fn [[x:Any][Any][ def y (mkl x) (0 fold [add] y) ]] [(g [1 2 3]) (g {a:1 b:2})]`,
			"[[6 3]]"},
		{"fold-init dynamic list", pre + `0 fold [add] (mkl [1 2 3])`, "[6]"},
		{"fold-init dynamic map", pre + `0 fold [add] (mkl {a:1 b:2})`, "[3]"},

		{"fold-noinit dynamic list", pre + `fold [add] (mkl [1 2 3])`, "[6]"},
		{"fold-noinit dynamic map", pre + `fold [add] (mkl {a:1 b:2})`, "[3]"},

		{"scan dynamic list", pre + `scan [add] (mkl [1 2 3])`, "[[1 3 6]]"},
		{"scan dynamic map", pre + `scan [add] (mkl {a:1 b:2})`, "[{a:1 b:3}]"},
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
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// SOUNDNESS (fallback allowed): compile==interpret must hold for shapes the
	// fix deliberately does NOT widen — a LAMBDA body over a gradual collection
	// (List=element vs Map=KeyVal is genuinely shape-divergent; it matches only the
	// single TFunction overload so the ambiguity gate never fires and it commits
	// the one matched overload), and an EMPTY collection (the seed/empty paths).
	sound := []struct{ name, src string }{
		// NOTE (NUR086's fix, 2026-08-24): "lambda over dynamic map" MOVED to
		// refusesAndFallsBack below. The invariant the comment above states —
		// "a TFunction lambda body matches only the single {TFunction,Map}
		// overload (count 1) — so a lambda never reaches here" — no longer
		// holds: each/for-each/fold/scan gained {TFunction,List}, so a lambda
		// over a GRADUAL collection now matches two TFunction overloads and
		// the ambiguity gate fires. That refusal is CORRECT (a lambda gets the
		// ELEMENT over a list and a KeyVal over a map, so a closure compiled
		// against either shape is wrong for the other), and it is a real, if
		// narrow, compile-coverage cost of the fix.
		{"each over dynamic empty list", pre + `[(mkl [] each [dup add])]`},
		{"each over dynamic empty map", pre + `[(mkl {} each [dup add])]`},
		{"scan over dynamic empty list", pre + `[(scan [add] (mkl []))]`},
	}
	// REFUSES AND FALLS BACK: the compile lane cannot commit, the default lane
	// runs the program correctly on the interpreter with the loud performance
	// warning. Asserted through CompileCheck (the refusal is the point) plus
	// interpreter parity, since RunCompiled surfaces a loud refusal as an error.
	refusesAndFallsBack := []struct{ name, src, reason string }{
		{"lambda over dynamic map", pre + `[(mkl {a:1 b:2} each ([kv:KeyVal] => [kv.v add kv.i]))]`,
			"ambiguous overload (List vs Map)"},
	}
	for _, c := range refusesAndFallsBack {
		t.Run("refuses/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog != nil {
				t.Fatalf("expected a refusal (%s); it compiled — if the shape is now modelled, move it back to sound", c.reason)
			}
			if !strings.Contains(reason, c.reason) {
				t.Errorf("refusal reason %q does not mention %q", reason, c.reason)
			}
			b, _ := New()
			if _, werr := b.RunInterp(c.src); werr != nil {
				t.Errorf("the interpreter must still run it: %v", werr)
			}
		})
	}

	for _, c := range sound {
		t.Run("sound/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src) // fallback allowed
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if (err == nil) != (werr == nil) {
				t.Fatalf("error mismatch: compiled err=%v interp err=%v", err, werr)
			}
			if err == nil && fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}
