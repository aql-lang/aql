package lang

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// End-to-end tests for the detached-stamp primitive (eng.StampFnValue) over
// REAL fn bodies — the compile path eng's own tests cannot drive (eng has no
// def/fn words). Positive paths pair with negatives per lang/go/CLAUDE.md.

// stampHarness runs src on a fresh AQL, arms runtime stamping when armed is
// true, and returns the AQL plus the fn value bound to name.
func stampHarness(t *testing.T, src, name string, armed bool) (*AQL, Value) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(src); err != nil {
		t.Fatalf("setup run: %v", err)
	}
	if armed {
		a.registry.EnableRuntimeStamping()
	}
	v, ok := a.registry.Defs.Top(name)
	if !ok {
		t.Fatalf("binding %q not found after setup", name)
	}
	return a, v
}

// invokeFnValue dispatches a fn VALUE through the callback seam exactly as
// the codec / service words do: MatchFnSig then InvokeCallback.
func invokeFnValue(t *testing.T, a *AQL, fn Value, args ...Value) []Value {
	t.Helper()
	fd, ok := fn.Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("not a fn value: %v", fn)
	}
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		t.Fatalf("MatchFnSig found no signature for %d args", len(args))
	}
	out, err := eng.InvokeCallback(a.registry, sig, args, fd.Captured)
	if err != nil {
		t.Fatalf("InvokeCallback: %v", err)
	}
	return out
}

// A runtime-constructed fn with a compilable body stamps, and the VM result
// is identical to the interpreter's over the same value. The negative twin:
// without arming (a plain interpreter run), the same value declines.
func TestStampFnValueRealBodyVMMatchesInterpreter(t *testing.T) {
	const src = `def h (fn [[x:Integer] [Integer] [x add 1]])`

	a, plain := stampHarness(t, src, "h", false)
	if _, ok := eng.StampFnValue(a.registry, plain); ok {
		t.Fatalf("unarmed registry must not stamp")
	}
	interp := invokeFnValue(t, a, plain, eng.NewInteger(41))

	a2, v2 := stampHarness(t, src, "h", true)
	stamped, ok := eng.StampFnValue(a2.registry, v2)
	if !ok {
		t.Fatalf("armed registry must stamp the compilable body")
	}
	fd := stamped.Data.(eng.FnDefInfo)
	ref := fd.Signatures[0].CompiledRef()
	if ref == nil || ref.Prog == nil {
		t.Fatalf("stamped value must carry a finalized CompiledFnRef")
	}
	vm := invokeFnValue(t, a2, stamped, eng.NewInteger(41))

	if len(vm) != len(interp) || len(vm) != 1 {
		t.Fatalf("result shape: vm=%v interp=%v", vm, interp)
	}
	vn, _ := vm[0].AsConcreteInteger()
	in, _ := interp[0].AsConcreteInteger()
	if vn != in || vn != 42 {
		t.Fatalf("vm=%d interp=%d, want both 42", vn, in)
	}

	// The ORIGINAL value is untouched by the clone-and-stamp.
	origFd := v2.Data.(eng.FnDefInfo)
	if origFd.Signatures[0].CompiledRef() != nil {
		t.Fatalf("StampFnValue must not mutate the input value's shared impl")
	}

	// A DEP-FREE body (a literal — no module-level name reads) stamps with an
	// empty snapshot and stays vacuously fresh across invokes.
	a3, v3 := stampHarness(t, `def k (fn [[x:Integer] [Integer] [42]])`, "k", true)
	depFree, ok := eng.StampFnValue(a3.registry, v3)
	if !ok {
		t.Fatalf("dep-free literal body must stamp")
	}
	for i := 0; i < 2; i++ {
		out := invokeFnValue(t, a3, depFree, eng.NewInteger(1))
		if n, _ := out[len(out)-1].AsConcreteInteger(); n != 42 {
			t.Fatalf("dep-free stamped fn: got %d, want 42", n)
		}
	}
}

// A stamped body reading a module-level dep: after the dep is rebound the
// depSnap goes stale and the invoke falls back to the interpreter, which
// resolves the LIVE binding — never the frozen one.
func TestStampFnValueDepRebindFallsBackLive(t *testing.T) {
	a, v := stampHarness(t, `
def bump 10
def h (fn [[x:Integer] [Integer] [x add bump]])
`, "h", true)
	stamped, ok := eng.StampFnValue(a.registry, v)
	if !ok {
		t.Fatalf("stamp declined for the dep-reading body")
	}
	out := invokeFnValue(t, a, stamped, eng.NewInteger(1))
	if n, _ := out[0].AsConcreteInteger(); n != 11 {
		t.Fatalf("pre-rebind: got %d, want 11", n)
	}

	// Rebind the dep. The stamped unit froze bump=10; the interpreter (and
	// therefore the stale-dep fallback) must see 20.
	if _, err := a.Run(`def bump 20`); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	out = invokeFnValue(t, a, stamped, eng.NewInteger(1))
	if n, _ := out[0].AsConcreteInteger(); n != 21 {
		t.Fatalf("post-rebind: got %d, want the LIVE 21 (interpreter fallback)", n)
	}
}

// A body the compiler refuses — a mid-expression fn-value application
// ("fn-value application bounded by a paren", the verified Stage-3 refusal) —
// declines the stamp and keeps interpreting unchanged.
func TestStampFnValueRefusingBodyInterpretsUnchanged(t *testing.T) {
	a, v := stampHarness(t, `
def m {f: ([y:Integer] => [y add 1])}
def h (fn [[x:Integer] [Integer] [ add 1 ((m get "f") x) ]])
`, "h", true)
	stamped, ok := eng.StampFnValue(a.registry, v)
	if ok {
		t.Fatalf("refusing body must decline the stamp")
	}
	// The returned value is the input, and it still runs on the interpreter:
	// add 1 ((m.f) 7) = 1 + (7+1) = 9.
	out := invokeFnValue(t, a, stamped, eng.NewInteger(7))
	if n, _ := out[len(out)-1].AsConcreteInteger(); n != 9 {
		t.Fatalf("declined value must interpret unchanged: got %d, want 9", n)
	}
}

// The detached compile is fully isolated: the live registry's def depths,
// check-state identity, and diagnostics are untouched by a stamp performed
// mid-session.
func TestStampFnValueParentStateUntouched(t *testing.T) {
	a, v := stampHarness(t, `
def bump 10
def h (fn [[x:Integer] [Integer] [x add bump]])
`, "h", true)
	r := a.registry
	checkBefore := r.Check
	emitBefore := r.Check.Emit
	diagsBefore := len(r.Check.Diagnostics)
	depthBase, depthH := r.Defs.Depth("base"), r.Defs.Depth("h")
	genBase := r.Defs.Gen("base")

	if _, ok := eng.StampFnValue(r, v); !ok {
		t.Fatalf("stamp declined")
	}

	if r.Check != checkBefore {
		t.Fatalf("stamp replaced the parent's CheckState")
	}
	if r.Check.Emit != emitBefore {
		t.Fatalf("stamp swapped the parent's Emit recorder")
	}
	if len(r.Check.Diagnostics) != diagsBefore {
		t.Fatalf("stamp leaked %d diagnostics into the parent", len(r.Check.Diagnostics)-diagsBefore)
	}
	if r.Defs.Depth("base") != depthBase || r.Defs.Depth("h") != depthH {
		t.Fatalf("stamp changed parent def depths")
	}
	if r.Defs.Gen("base") != genBase {
		t.Fatalf("stamp bumped a parent binding generation")
	}
	if r.Check.Mode {
		t.Fatalf("stamp left the parent in check mode")
	}
}

// RunCompiled / RunCompiledStrict arm the policy; a plain Run never does —
// the mode contract that keeps -no-compile a pure interpreter path.
func TestCompiledEntriesArmRuntimeStamping(t *testing.T) {
	a, _ := New()
	if _, err := a.Run(`1 add 1`); err != nil {
		t.Fatal(err)
	}
	if a.registry.RuntimeStampingEnabled() {
		t.Fatalf("plain Run must not arm runtime stamping")
	}

	b, _ := New()
	if _, _, err := b.RunCompiled(`1 add 1`); err != nil {
		t.Fatal(err)
	}
	if !b.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiled must arm runtime stamping")
	}

	c, _ := New()
	if _, err := c.RunCompiledStrict(`1 add 1`); err != nil {
		t.Fatal(err)
	}
	if !c.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiledStrict must arm runtime stamping")
	}

	// The fallback path of RunCompiled (uncompilable program) keeps the flag:
	// stored callbacks still earn the VM under compiled-mode requests. The
	// mid-expression fn-value apply refuses whole-program compilation
	// ("fn-value application bounded by a paren").
	d, _ := New()
	out, compiled, err := d.RunCompiled(`def m {f: ([y:Integer] => [y add 1])} add 1 ((m get "f") 5)`)
	if err != nil {
		t.Fatal(err)
	}
	if compiled {
		t.Fatalf("expected the interpreter fallback for the refusing program")
	}
	if len(out) != 1 {
		t.Fatalf("fallback result: %v", out)
	}
	if !d.registry.RuntimeStampingEnabled() {
		t.Fatalf("the compiled-mode fallback must keep runtime stamping armed")
	}
}

// Module sub-registries inherit the armed flag (RunModuleBody), so module
// bodies — where the real apps construct their callbacks — can stamp; and
// they do NOT inherit it when the parent is unarmed.
func TestModuleRegistryInheritsRuntimeStamping(t *testing.T) {
	src := `module [ def helper (fn [[x:Integer] [Integer] [x add 1]]) export "M" {helper: helper/r} ]`

	a, _ := New()
	a.registry.EnableRuntimeStamping()
	// Rebind the export's fn to a top-level name so the test reads it from
	// the def table without unwrapping the ModuleExport payload Go-side.
	if _, err := a.Run(`import ` + src + ` def hh M.helper/r`); err != nil {
		t.Fatalf("armed import: %v", err)
	}
	helper, ok := a.registry.Defs.Top("hh")
	if !ok {
		t.Fatalf("hh not bound after armed import")
	}
	fd, isFn := helper.Data.(eng.FnDefInfo)
	if !isFn || fd.Registry == nil {
		t.Fatalf("M.helper is not a module fn with a sub-registry")
	}
	if !fd.Registry.RuntimeStampingEnabled() {
		t.Fatalf("module sub-registry must inherit the armed flag")
	}

	b, _ := New()
	if _, err := b.Run(`import ` + src + ` def hh M.helper/r`); err != nil {
		t.Fatalf("unarmed import: %v", err)
	}
	h2, ok := b.registry.Defs.Top("hh")
	if !ok {
		t.Fatalf("hh not bound after unarmed import")
	}
	fd2, isFn2 := h2.Data.(eng.FnDefInfo)
	if !isFn2 {
		t.Fatalf("M.helper (unarmed) is not a fn value")
	}
	if fd2.Registry != nil && fd2.Registry.RuntimeStampingEnabled() {
		t.Fatalf("unarmed parent must not arm the module sub-registry")
	}
}
