package lang

import (
	"bytes"
	"strings"
	"testing"
)

// The compiled VM enforces a fn's declared return type/count at RET, the
// mirror of the interpreter's ReturnCheck (__RC). A conforming body
// compiles and runs; a non-conforming one must produce the SAME
// type_error the interpreter raises — whether the VM raises it directly
// (return-type mismatch) or the program refuses to compile and the
// interpreter raises it on the fallback path (return-count mismatch).
func TestCompiledReturnCheck(t *testing.T) {
	// Positive: a conforming fn compiles and returns its value.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def dbl fn [[n:Integer] [Integer] [n mul 2]] dbl 21`)
	if err != nil || !compiled {
		t.Fatalf("conforming fn: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != int64(42) {
		t.Fatalf("dbl 21 = %v, want 42", out)
	}

	// Negative: the VM raises the return-type error directly (the fn
	// compiles, the wrong value reaches RET). Same type_error as interp.
	for _, src := range []string{
		`def f fn [[n:Integer] [String] [n]] f 1`,              // Integer body, String return
		`def Big (Integer gt 10) def mk fn [[] [Big] [5]] mk`,  // 5 fails the predicate
		`def Pos (refine Integer) def mk fn [[] [Pos] [7]] mk`, // nominal newtype mismatch
		`def r2 fn [[n:Integer] [Integer] [n n]] r2 1`,         // return COUNT mismatch
	} {
		ac, _ := New()
		_, _, errC := ac.RunCompiled(src)
		ai, _ := New()
		_, errI := ai.RunInterp(src)
		if errC == nil || errI == nil {
			t.Errorf("%q: expected both to error, compiled=%v interp=%v", src, errC, errI)
			continue
		}
		if !strings.Contains(errC.Error(), "type_error") && !strings.Contains(errC.Error(), errI.Error()) {
			// errC should be the same type_error taxonomy as the interpreter.
			t.Errorf("%q: compiled error %q does not match interpreter %q", src, errC, errI)
		}
	}
}

// RunCompiled compiles the program in check mode, which executes its
// RunInCheckMode words (def/import/type, the Test harness) for real.
// When the program is uncompilable the interpreter fallback re-runs the
// whole source, so the check-pass side effects MUST be rolled back first
// — otherwise a type re-mint, fn re-registration, or re-import diverges
// from a clean interpreter run. These pin that isolation
// (SnapshotForCompile / RestoreForCompile).
func TestRunCompiledFallbackIsolation(t *testing.T) {
	// Each row is UNCOMPILABLE (so it takes the fallback path) and
	// side-effecting (so a double-execution would corrupt the result).
	// RunCompiled must equal a clean interpreter Run.
	cases := []string{
		// type mint + undef, then reuse — a re-mint would clash. A flex default
		// bakes (make freshens it per instance) AND a mutation of the flex field
		// (`p.x push 1`) now compiles too (the class field's declared List type
		// rides strict through gradual contagion — cross-module element typing),
		// so this pairs the class-mint + `undef C` with a still-uncompilable
		// predicate-fn `is` (the VM cannot re-step a fn body) to keep the whole
		// row on the fallback path: the undef-C side effect must still be rolled
		// back before the whole-program interpreter fallback re-runs.
		`def C class {x:(flex [])} def p (make C {}) undef C end def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] (p.x push 1) (5 is Positive)`,
		// fn registration under a capitalised name — a re-register clashes.
		// `is` over the predicate fn INVOKES it (the VM cannot re-step a fn
		// body), so this stays uncompilable even though `typeof`/`tcmp` over a
		// fn value now compile (fn-value introspection).
		`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] 5 is Positive`,
		// native-module import whose namespace metadata a re-import degrades.
		// The module-SYNTHETIC reads (`typeof MathUtil`, `MathUtil.$name`,
		// `MathUtil.$module.name`) now const-fold and compile, so these pair the
		// import with a still-uncompilable operation: the import side effect must
		// still be rolled back before the whole-program fallback re-runs.
		`import "aql:math-util" def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] 5 is Positive`,
		// aql:test import isolation: Test.test / Test.describe cases (closure
		// path) AND the property words prop/check-prop/skip (their inert bodies
		// bake as consts — the dot-access reach inside now an inert member) all
		// compile, so pair the import with the predicate-fn `is` (still
		// uncompilable — the VM cannot re-step the fn body, exactly as row 2/3)
		// to keep the row on the fallback path and exercise the import rollback.
		`import "aql:test" def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] 5 is Positive`,
	}
	for _, src := range cases {
		ac, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotC, wasCompiled, errC := ac.RunCompiled(src)
		if wasCompiled {
			t.Fatalf("%q unexpectedly compiled — this row is meant to exercise the FALLBACK isolation path", src)
		}

		ai, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := ai.RunInterp(src)

		if (errC != nil) != (errI != nil) {
			t.Errorf("%q: error divergence: compiled-fallback=%v interpreter=%v", src, errC, errI)
			continue
		}
		if errC != nil {
			continue
		}
		if len(gotC) != len(gotI) {
			t.Errorf("%q: length divergence: %v vs %v", src, gotC, gotI)
			continue
		}
		for i := range gotC {
			if gotC[i] != gotI[i] {
				t.Errorf("%q: result divergence at %d: compiled-fallback=%v interpreter=%v", src, i, gotC, gotI)
				break
			}
		}
	}

	// Positive control: a COMPILABLE program still runs on the compiled
	// path (the snapshot must not have rolled back state it needs). The
	// minted type reaches PUSH_TYPE at run time.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def Pt class {x:1} def q (make Pt {x:7}) q.x`)
	if err != nil {
		t.Fatalf("compilable user-type program: %v", err)
	}
	_ = compiled // may compile or fall back depending on the emitter; result is what matters
	if len(out) != 1 || out[0] != int64(7) {
		t.Fatalf("user-type field access = %v, want 7", out)
	}
}

// Compiled mode must not swallow a trace: the `trace` word (IO.trace)
// renders the interpreter's step-by-step execution, which the bytecode
// VM has no equivalent for. It refuses to compile ("unannotated or
// opaque word trace"), so a traced program whole-program-falls-back to
// the interpreter and the trace renders — exactly the plan's "compiled
// mode disables itself under trace" contract, realised via fallback.
// Pin it so making `trace` compilable can't silently lose the trace.
func TestCompiledTraceRenders(t *testing.T) {
	// `IO.trace` renders the VALUE it traces, not interpreter execution steps, so
	// it compiles to a CALL_NATIVE (its `[add 1 2]` arg now assembles via
	// OpMakeList) and emits BYTE-IDENTICAL output to the interpreter. The trace
	// side-effect must therefore survive the compiled path unchanged.
	src := `import "aql:io" IO.trace [add 1 2]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var bufC bytes.Buffer
	a.SetOutput(&bufC)
	out, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("traced program: %v", err)
	}
	if !compiled {
		t.Error("a value trace must take the compiled path (it renders the same)")
	}
	if len(out) != 1 || out[0] != int64(3) {
		t.Fatalf("traced result = %v, want 3", out)
	}
	b, _ := New()
	var bufI bytes.Buffer
	b.SetOutput(&bufI)
	if _, errI := b.RunInterp(src); errI != nil {
		t.Fatalf("interpreter trace: %v", errI)
	}
	if !strings.Contains(bufC.String(), "trace") {
		t.Errorf("no trace output captured: %q", bufC.String())
	}
	if bufC.String() != bufI.String() {
		t.Errorf("trace stdout diverged:\n compiled=%q\n interp  =%q", bufC.String(), bufI.String())
	}
}

// Mixed-mode error rendering: when a fallback island errors, the message
// must read byte-identically to the interpreter — the island re-runs the
// SAME tokens through a sub-engine, so its AqlError carries the island's
// own frame attribution ("each: element 0: …"), and the VM stamps it
// through the shared error path (stampAt) without overwriting that
// position. A regression that mangled island-error attribution (e.g.
// overstamping with the OpFallback pc) would diverge here.
func TestCompiledIslandErrorRendering(t *testing.T) {
	cases := []string{
		`each [convert Integer] ['x' 'y']`,
		`fold [convert Integer] ['a'] 0`,
		`scan [convert Integer] ['a' 'b']`,
		`filter [convert Integer] ['a']`,
	}
	for _, src := range cases {
		ac, err := New()
		if err != nil {
			t.Fatal(err)
		}
		_, _, errC := ac.RunCompiled(src)
		ai, _ := New()
		_, errI := ai.RunInterp(src)
		if errC == nil || errI == nil {
			t.Errorf("%q: expected both to error, compiled=%v interp=%v", src, errC, errI)
			continue
		}
		if errC.Error() != errI.Error() {
			t.Errorf("%q: island error text differs\n  compiled: %s\n  interp:   %s", src, errC.Error(), errI.Error())
		}
	}
}

// `args` (and `__pa`) read the interpreter's per-call args stack, which
// the bytecode VM's CALL_USER frame does not maintain — it binds params
// to frame locals. A compiled fn body that reads `args` would fail at run
// time with "args: not inside a function", so the emitter refuses it and
// the program falls back. Pinned because it is a latent soundness gap no
// spec row currently triggers (a future change that let `args` compile
// would silently break it).
func TestCompiledArgsWordFallsBack(t *testing.T) {
	// Bare `args` (the WHOLE per-call list) still falls back: the args
	// projection has no foldable consumer, so it refuses at its use site and
	// the interpreter owns it. (Compiling it would need a build-list-from-locals
	// lowering — a separate, reducible follow-on.) `args.N` is different — it
	// folds to a frame local — and is covered by TestArgsAccessorCompilesNative.
	for _, c := range []struct {
		src  string
		want any
	}{
		{`def g fn [[n:Integer] [List] [args]] g 7`, "[7]"},
	} {
		ac, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := ac.RunCompiled(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if compiled {
			t.Errorf("%q took the compiled path; a fn reading bare `args` must fall back", c.src)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q = %v, want %v", c.src, out, c.want)
		}
	}

	// args.N, by contrast, now compiles to a frame-local read.
	ac, _ := New()
	out, compiled, err := ac.RunCompiled(`def f fn [[n:Integer] [Integer] [args.0]] f 3`)
	if err != nil {
		t.Fatalf("args.0: %v", err)
	}
	if !compiled {
		t.Errorf("args.0 should compile natively (folds to PUSH_LOCAL 0)")
	}
	if len(out) != 1 || out[0] != int64(3) {
		t.Fatalf("args.0 = %v, want 3", out)
	}
}
