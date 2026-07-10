package lang

// Frozen module reads (the unit-bake freeze discipline): a CONCRETE
// module-scope binding read inside a fn/closure UNIT analysis bakes into the
// unit (a const, or splice-fired tokens) that re-runs on every call, while
// the interpreter re-resolves the name per call (the documented module-level
// dynamic semantics, lang/go/CLAUDE.md "Closures and Capture"). A later
// module-scope rebind therefore made the compiled program diverge:
//
//	def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0
//	interpreter: 1 2      compiled (before the fix): 1 1   ← MISCOMPILE
//
// The fix: NoteFrozenRead records such reads (engine.go stepWord), and
// NotifyNameRebound marks the program uncompilable on a later module-scope
// rebind — interpreter fallback, correct result. STORED-REF units (service
// handlers, spawn bodies) are exempt: their rebind safety is the precise
// per-ref poisoning (TestCompiledStoredHandlerFreezeRedefine), which keeps
// the rest of the program compiled.

import (
	"fmt"
	"strings"
	"testing"
)

// TestModuleReadRebindRefusesAndMatches — the miscompile pin: the rebind
// program refuses with the named reason and the fallback matches the
// interpreter's documented dynamic-read result.
func TestModuleReadRebindRefusesAndMatches(t *testing.T) {
	cases := []string{
		// The documented module-dynamic case (scalar read baked as a const).
		`def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0`,
		// The splice twin: the macro payload fires into the unit's tokens at
		// analysis time; a rebind would leave the old tokens frozen.
		`def op (quote [1 add 2])  def f fn [[n:Integer] [Any] [do [n drop word op]]]  f 0  def op (quote [10 mul 4])  f 0`,
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
		if prog != nil {
			t.Errorf("%q: a module rebind after a unit baked the binding must refuse; compiled instead", src)
		} else if reason == "" {
			t.Errorf("%q: refusal must carry a named reason", src)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled {
			t.Errorf("%q: expected the interpreter fallback", src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
}

// TestModuleReadNoRebindStillCompiles — the positive pair: the same fn over a
// NEVER-rebound module binding keeps compiling (the freeze is keyed on actual
// rebinds, not on "reads a module ref").
func TestModuleReadNoRebindStillCompiles(t *testing.T) {
	src := `def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  f 10`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: must compile; reason=%q err=%v", src, reason, cerr)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: want compiled parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}

// TestStaticSpliceBodiesCompile — closure bodies containing `word` splices
// over compile-time-known payloads compile NATIVELY: the check engine fires
// the splice during body analysis (`word` is a non-emitting projection) and
// the post-splice dispatches record into the unit. Unblocked by the
// multi-out/out-of-order residual work; pinned here so the shapes never
// regress to "code-body word do (Stage 2)".
func TestStaticSpliceBodiesCompile(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def xs (quote [1 add 2])  do [word xs]`, "[3]"},
		{`def op (quote [mul 2])  do [5 word op]`, "[10]"},
		{`def twice word [dup add]  do [5 twice]`, "[10]"},
		{`def op (quote [mul 2])  [1 2 3] each [word op]`, "[[2 4 6]]"},
		// a computed payload that CONST-FOLDS (literal index) composes.
		{`def i 0  def ops [quote [1 add 2]]  do [word (ops get i)]`, "[3]"},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: splice body must compile; reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile natively, not island", c.src)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil ||
			fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: want %s compiled with parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				c.src, c.want, compiled, gotC, errC, gotI, errI)
		}
	}
	// NEGATIVE: a RUNTIME-computed payload (non-foldable) keeps refusing with
	// interpreter parity — Phase E (runtime JIT) territory.
	src := `def xs [add 1 2]  do [word xs]`
	b, _ := New()
	prog, _, _, _ := b.CompileCheck(src)
	if prog != nil {
		t.Errorf("%q: a computed (carrier) splice payload must keep refusing", src)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: fallback parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}

// TestComputedEachBodyStaysRefused — a COMPUTED body on a non-CompileDynBody
// higher-order word (each) keeps the refusal with fallback parity: the island
// cannot bake a carrier code body (its tokens carry the island's program), and
// each declares no dyn-body backstop. Pins the island's non-bakeable
// NoEvalArgs decline.
func TestComputedEachBodyStaysRefused(t *testing.T) {
	src := `def op (quote [mul 2])  def f fn [[b:List] [List] [[1 2 3] each b]]  f op`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck(%q): %v", src, cerr)
	}
	if prog != nil || reason == "" {
		t.Errorf("%q: computed each body must refuse; got prog=%v reason=%q", src, prog != nil, reason)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: fallback parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}
