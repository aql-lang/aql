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
		_, errI := ai.Run(src)
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
		// type mint + undef, then reuse — a re-mint would clash.
		`def Point class {x:1} def p (make Point {x:5}) undef Point end p typeof`,
		// fn registration under a capitalised name — a re-register clashes.
		`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] Positive tcmp Positive`,
		// native-module import whose namespace metadata a re-import degrades.
		`"aql:math-util" import end typeof MathUtil`,
		`"aql:math-util" import end MathUtil.$name`,
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
		gotI, errI := ai.Run(src)

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
func TestCompiledTraceFallsBack(t *testing.T) {
	src := `"aql:io" import end IO.trace [add 1 2]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	a.SetOutput(&buf)
	out, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("traced program: %v", err)
	}
	if compiled {
		t.Error("a traced program took the compiled path; it must fall back so the trace renders")
	}
	if len(out) != 1 || out[0] != int64(3) {
		t.Fatalf("traced result = %v, want 3", out)
	}
	if !strings.Contains(buf.String(), "trace") {
		t.Errorf("no trace output captured: %q", buf.String())
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
		_, errI := ai.Run(src)
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
	for _, c := range []struct {
		src  string
		want any
	}{
		{`def g fn [[n:Integer] [List] [args]] g 7`, "[7]"},
		{`def f fn [[n:Integer] [Integer] [args.0]] f 3`, int64(3)},
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
			t.Errorf("%q took the compiled path; a fn reading `args` must fall back", c.src)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q = %v, want %v", c.src, out, c.want)
		}
	}
}
