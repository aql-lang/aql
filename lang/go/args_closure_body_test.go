package lang

// `args` inside a CLOSURE body (a higher-order word's do/each$body unit)
// must not compile: the closure analysis's args frame holds the
// CallableSpec inputs, while at run time the body executes through
// InvokeBody in the ENCLOSING call context — the interpreter's `args`
// reads the enclosing fn's per-call list. Projecting the closure frame
// const-baked the wrong (often empty) list: `do [args]` inside a fn
// compiled to PUSH_CONST [] against the interpreter's [7], and the
// island path reproduced the divergence as error(args_error). The fix
// declines the projection inside closure units (specialWordResults →
// EmitState.inClosureUnit) and screens args/__pa out of island bodies
// (bodyFreeForFallback), so the program refuses and the interpreter
// fallback returns the correct value. Fn-UNIT args projection (args.N
// folding to PUSH_LOCAL N) is untouched.

import (
	"fmt"
	"strings"
	"testing"
)

// argsBothEngines runs src via RunCompiled and interpreted (reusing the
// shared runBothEngines helper), failing the test on either engine
// erroring, and returns the rendered results plus whether the compiled
// path actually compiled.
func argsBothEngines(t *testing.T, src string) (interp, compiled string, wasCompiled bool) {
	t.Helper()
	gotC, was, errC, gotI, errI := runBothEngines(t, src)
	if errC != nil {
		t.Fatalf("RunCompiled(%q): %v", src, errC)
	}
	if errI != nil {
		t.Fatalf("Run(%q): %v", src, errI)
	}
	return fmt.Sprint(gotI), fmt.Sprint(gotC), was
}

// TestArgsInClosureBodyRefusesAndMatches — the negative pins: every
// closure-body args shape refuses to compile (named reason) and the
// interpreter fallback result matches a plain interpreter run.
func TestArgsInClosureBodyRefusesAndMatches(t *testing.T) {
	cases := []string{
		// do body reading the enclosing fn's args (the original miscompile:
		// compiled [] / islanded error(args_error) vs interpreted [7]).
		`def g fn [[n:Integer] [Any] [do [args]]]  g 7`,
		// each closure body reading args.N of the enclosing fn (the fold
		// corollary: the closure's input must not satisfy the projection).
		`def g fn [[n:Integer] [List] [[10 20] each [drop args.0]]]  g 7`,
	}
	for _, src := range cases {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", src, cerr)
		}
		if prog != nil || reason == "" {
			t.Errorf("%q: args inside a closure body must REFUSE to compile; got prog=%v reason=%q",
				src, prog != nil, reason)
		}
		interp, compiled, was := argsBothEngines(t, src)
		if was {
			t.Errorf("%q: RunCompiled reported compiled=true; expected interpreter fallback", src)
		}
		if interp != compiled {
			t.Errorf("%q: engine divergence: interpreted=%q fallback=%q", src, interp, compiled)
		}
	}
}

// TestArgsAtTopLevelDoParity — top-level `do [args]`: args errors
// ("not inside a function") and `do` traps it into an Error value in
// BOTH engines. The program refuses to compile (context-dependent word)
// and the fallback matches.
func TestArgsAtTopLevelDoParity(t *testing.T) {
	src := `do [args]`
	interp, compiled, was := argsBothEngines(t, src)
	if was {
		t.Errorf("%q: RunCompiled reported compiled=true; expected interpreter fallback", src)
	}
	if interp != compiled {
		t.Errorf("%q: engine divergence: interpreted=%q fallback=%q", src, interp, compiled)
	}
	if !strings.Contains(interp, "args") {
		t.Errorf("%q: expected the trapped args_error value, got %q", src, interp)
	}
}

// TestArgsFnUnitFoldStillCompiles — the positive pair: args.N inside a
// plain FN body keeps folding to the frame local (PUSH_LOCAL N) and the
// program compiles. Guards the fn-unit projection against over-broad
// closure gating.
func TestArgsFnUnitFoldStillCompiles(t *testing.T) {
	src := `def f fn [[a:Integer b:Integer] [Integer] [args.1]]  f 1 2`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected the fn-unit args.N fold to compile; reason=%q err=%v", src, reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "PUSH_LOCAL") {
		t.Errorf("%q: expected args.1 to fold to a PUSH_LOCAL frame read; got:\n%s", src, prog.Disassemble())
	}
	interp, compiled, was := argsBothEngines(t, src)
	if !was {
		t.Errorf("%q: expected the compiled path to run", src)
	}
	if interp != compiled || interp != "[2]" {
		t.Errorf("%q: want [2] on both engines; interpreted=%q compiled=%q", src, interp, compiled)
	}
}
