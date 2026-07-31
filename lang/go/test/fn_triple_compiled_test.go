package test

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// fn's 3-arg triple form and afn both construct Function values whose
// operands can be COMPUTED (`fn (2 add 3) [String] ['five']`, `(2 add 3)
// afn [body]`). Under static analysis a computed operand resolves to a
// carrier, and the check-time construction the compiler would intern as
// a constant carries that carrier where the live value belongs (an
// Integer carrier as the input pattern instead of 5) — baking it made
// `d 5` refuse dispatch in the compiled engine while the interpreter,
// re-constructing with the live value, matched. fnTripleHandler and
// afnHandler now mark such constructions uncompilable so the program
// falls back to the interpreter, and a carrier PATTERN on a signature
// is never enforced at match time (patternsOk / MatchSignature /
// MatchFnSig skip it) so the CHECK pass has no false positive either.
// This test pins compiled == interpreted for the operand shapes. The
// cases live here rather than in lang/spec because these constructions
// are deliberately interpreter-only and the spec corpus enforces
// refusalCeiling = 0 (every spec value row must compile).
func TestFnConstructionCompiledParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	cases := []struct {
		src  string
		want string // the (shared) result both engines must produce
	}{
		// Computed triple-form input — the miscompile case: the input
		// pattern must be the LIVE 5, not the check-time carrier.
		{`def d fn (2 add 3) [String] ["five"]  d 5`, `[five]`},
		// Computed afn input — same class, pre-existing before the
		// triple form; fixed by the same guard.
		{`def d ((2 add 3) afn ["got5"])  d 5`, `[got5]`},
		// Fully-literal constructions still compile natively and agree.
		{`def d fn 5 [String] ["five"]  d 5`, `[five]`},
		{`def d fn x:Integer [Integer] [x mul 2]  d 4`, `[8]`},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ai, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			interp, ierr := ai.Run(c.src)
			if ierr != nil {
				t.Fatalf("interp: %v", ierr)
			}
			ac, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			comp, _, cerr := ac.RunCompiled(c.src)
			if cerr != nil {
				t.Fatalf("compiled: %v", cerr)
			}
			is, cs := fmt.Sprint(interp), fmt.Sprint(comp)
			if is != c.want {
				t.Errorf("interpreted = %s, want %s", is, c.want)
			}
			if cs != is {
				t.Errorf("compiled = %s diverges from interpreted = %s (miscompile)", cs, is)
			}
		})
	}
}

// The negative side of the parity contract: a computed input pattern is
// still ENFORCED at runtime (the guard only defers construction to the
// interpreter, it does not loosen the constructed fn), and the check
// pass stays CLEAN on the deferred construction (the carrier pattern is
// not a static false positive).
func TestFnComputedPatternStillEnforced(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := a.Run(`def d fn (2 add 3) [String] ["five"]  d 9`); err == nil {
		t.Fatal("d 9 must not match the computed input pattern 5")
	}
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := ac.Check(`def d fn (2 add 3) [String] ["five"]  d 5`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		t.Errorf("check must be clean on a computed-input construction, got: %v", d)
	}
}
