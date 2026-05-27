package eng_test

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// freshRegistry builds an eng-only Registry plus a small set of probe
// natives. The /r suffix is a parser+kernel feature, so these tests
// stay in eng and only need the kernel surface — no `ref` word, no
// `apply` word, neither of which lives here. They test stepWord's
// ForceRef branch, eng.ResolveRef directly, and the dispatch of
// unquoted Function values via execFnDefLiteral.
func freshRegistry(t *testing.T) *eng.Registry {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "add",

		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[1])
				b, _ := eng.AsInteger(args[0])
				return []eng.Value{eng.NewInteger(a + b)}, nil
			},
			Returns: []*eng.Type{eng.TInteger}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mul",

		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[1])
				b, _ := eng.AsInteger(args[0])
				return []eng.Value{eng.NewInteger(a * b)}, nil
			},
			Returns: []*eng.Type{eng.TInteger}, BarrierPos: -1,
		}},
	})
	r.Defs.Push("answer", eng.NewInteger(42))
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	return r
}

func runSrc(t *testing.T, r *eng.Registry, src string) []eng.Value {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	out, err := eng.NewTop(r).Run(prog)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	return out
}

func runSrcErr(t *testing.T, r *eng.Registry, src string) error {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	_, err = eng.NewTop(r).Run(prog)
	return err
}

// --- the asymmetry the /r suffix exists to address --------------------

// TestBareWordInvokesFnBinding pins existing behavior: a bare word
// for an fn binding fires dispatch.
func TestBareWordInvokesFnBinding(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "2 add 3")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(out), out)
	}
	got, _ := eng.AsInteger(out[0])
	if got != 5 {
		t.Errorf("bare `add` did not invoke: got %d, want 5", got)
	}
}

// TestRefSuffixReturnsFunctionValue: /r resolves to an UNQUOTED
// Function value carrying the FnDefInfo. Unquoted is the new
// default — call site, not data.
func TestRefSuffixReturnsFunctionValue(t *testing.T) {
	r := freshRegistry(t)
	// `add/r` standalone: no following args, no preceding stack args,
	// so the unquoted Function value's sig doesn't match anything —
	// it sits as data at the end of Run.
	out := runSrc(t, r, "add/r")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1", len(out))
	}
	v := out[0]
	if !v.Parent.Equal(eng.TFunction) {
		t.Errorf("top.Parent=%s, want Function", v.Parent.String())
	}
	if v.Quoted {
		t.Errorf("function value is Quoted — /r should produce unquoted in the new dispatch model")
	}
	fnDef, ok := v.Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("payload=%T, want FnDefInfo", v.Data)
	}
	if fnDef.Name != "add" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "add")
	}
}

// TestRefSuffixDispatchesWithForwardArgs is the headline new
// behavior: an unquoted Function value at the pointer collects
// forward args via full sig matching, just like a word.
func TestRefSuffixDispatchesWithForwardArgs(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "add/r 2 3")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(out), out)
	}
	got, err := eng.AsInteger(out[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 5 {
		t.Errorf("add/r 2 3 = %d, want 5", got)
	}
}

// TestRefSuffixDispatchesWithStackArgs verifies the stack-side of
// the dispatch — args already on the stack get consumed.
func TestRefSuffixDispatchesWithStackArgs(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "2 3 add/r")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(out), out)
	}
	got, _ := eng.AsInteger(out[0])
	if got != 5 {
		t.Errorf("2 3 add/r = %d, want 5", got)
	}
}

// TestRefSuffixOnSimpleValueBinding: /r is uniform across binding
// shapes — a simple-value def returns the value itself.
func TestRefSuffixOnSimpleValueBinding(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "answer/r")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1", len(out))
	}
	got, err := eng.AsInteger(out[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestRefSuffixUndefinedNameErrors(t *testing.T) {
	r := freshRegistry(t)
	err := runSrcErr(t, r, "nope/r")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "undefined") && !strings.Contains(err.Error(), "not bound") {
		t.Errorf("error=%q, want undefined/not-bound", err.Error())
	}
}

// --- the stable-map-lookup demonstration ------------------------------

// TestRefStableInMap proves the captured Function value retains its
// FnDef payload through map storage. The captured value is now
// UNQUOTED (live call-site shape); the test asserts identity, not
// Quoted state.
func TestRefStableInMap(t *testing.T) {
	r := freshRegistry(t)

	ops := eng.NewOrderedMap()
	for _, name := range []string{"add", "mul"} {
		v, ok := resolveViaSlashR(t, r, name)
		if !ok {
			t.Fatalf("resolveViaSlashR(%q): not bound", name)
		}
		ops.Set(name, v)
	}

	for _, name := range []string{"add", "mul"} {
		v, ok := ops.Get(name)
		if !ok {
			t.Fatalf("ops[%q] missing", name)
		}
		if !v.Parent.Equal(eng.TFunction) {
			t.Errorf("ops[%q].Parent=%s, want Function", name, v.Parent.String())
		}
		if v.Quoted {
			t.Errorf("ops[%q] is Quoted — captured Function should be unquoted (live call-site)", name)
		}
		fnDef, ok := v.Data.(eng.FnDefInfo)
		if !ok {
			t.Fatalf("ops[%q] payload=%T, want FnDefInfo", name, v.Data)
		}
		if fnDef.Name != name {
			t.Errorf("ops[%q] fnDef.Name=%q", name, fnDef.Name)
		}
	}

	// Stability under shadowing: push a non-fn binding on top of `add`
	// (without popping the underlying FnDef). The map entry still
	// holds the original Function value — the map stores referents,
	// not names that get re-resolved.
	r.Defs.Push("add", eng.NewString("shadowed"))
	v, _ := ops.Get("add")
	if !v.Parent.Equal(eng.TFunction) {
		t.Fatalf("after shadow, ops[add].Parent=%s, want Function still", v.Parent.String())
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "add" {
		t.Errorf("after shadow, captured fnDef.Name=%q, want %q", fnDef.Name, "add")
	}
	if len(fnDef.Signatures) == 0 {
		t.Errorf("captured FnDef has no Signatures — the captured handle wouldn't dispatch")
	}

	// Hard-stability check: even after popping the underlying binding
	// entirely (so `add/r` would now fail), the previously captured
	// value still carries the full FnDef payload.
	if !r.Defs.Pop("add") {
		t.Fatal("Defs.Pop(shadow) returned false")
	}
	if !r.Defs.Pop("add") {
		t.Fatal("Defs.Pop(original) returned false")
	}
	if _, ok := r.Defs.Top("add"); ok {
		t.Fatal("expected add binding to be gone after double-pop")
	}
	stillThere, _ := ops.Get("add")
	stillFn, _ := stillThere.Data.(eng.FnDefInfo)
	if stillFn.Name != "add" || len(stillFn.Signatures) == 0 {
		t.Errorf("post-undef captured fn lost shape: name=%q sigs=%d", stillFn.Name, len(stillFn.Signatures))
	}
}

// resolveViaSlashR runs `<name>/r` through the engine and returns the
// resulting value. The /r expression sits at end-of-program; with no
// following args its sig doesn't match anything and it falls through
// as data — that's how we get the captured value out.
func resolveViaSlashR(t *testing.T, r *eng.Registry, name string) (eng.Value, bool) {
	t.Helper()
	out := runSrc(t, r, name+"/r")
	if len(out) != 1 {
		return eng.Value{}, false
	}
	return out[0], true
}

// TestResolveRefDirect exercises the exported helper independently of
// the parser. The returned Function is unquoted — same contract as
// the /r suffix path.
func TestResolveRefDirect(t *testing.T) {
	r := freshRegistry(t)
	v, ok := eng.ResolveRef(r, "mul")
	if !ok {
		t.Fatal("ResolveRef(mul): not bound")
	}
	if !v.Parent.Equal(eng.TFunction) {
		t.Errorf("Parent=%s, want Function", v.Parent.String())
	}
	if v.Quoted {
		t.Error("returned function is Quoted — should be unquoted")
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "mul" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "mul")
	}

	if _, ok := eng.ResolveRef(r, "nope"); ok {
		t.Error("ResolveRef(nope): expected not-bound, got ok")
	}
}
