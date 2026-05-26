package eng_test

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// freshRegistry builds an eng-only Registry pre-populated with the
// kernel-shipped `ref` word plus a tiny set of probe natives the
// tests below need. No lang words, no parser-side typing rules other
// than what eng itself provides — a minimal surface so the tests
// exercise the ref path in isolation.
func freshRegistry(t *testing.T) *eng.Registry {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// `add` and `mul`: typical binary integer fns. RegisterNativeFunc
	// pushes a FnDefInfo entry into DefTable, which is precisely the
	// shape a user-level `def add fn […]` produces — so resolveRef
	// behaves identically whether the binding came from native code or
	// from user code.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "add",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[1])
				b, _ := eng.AsInteger(args[0])
				return []eng.Value{eng.NewInteger(a + b)}, nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "mul",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[1])
				b, _ := eng.AsInteger(args[0])
				return []eng.Value{eng.NewInteger(a * b)}, nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
	// A non-function binding, so we can prove ref on a plain value
	// just returns the value verbatim.
	r.Defs.Push("answer", eng.NewInteger(42))
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	return r
}

// runSrc parses src through the eng parser and runs it through a
// fresh engine on r. Returns the final stack. Fails the test on any
// parse / run error so each call site stays focused on assertions.
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

// runSrcErr is the error-path counterpart: parses and runs, returns
// any Run error (parse errors still fail the test directly).
func runSrcErr(t *testing.T, r *eng.Registry, src string) error {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	_, err = eng.NewTop(r).Run(prog)
	return err
}

// --- the asymmetry the new word exists to address ----------------------

// TestBareWordInvokesFnBinding pins the existing behavior `ref` is
// designed to opt out of: a bare word for an fn binding fires
// dispatch, not data substitution.
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

// TestRefWordReturnsFunctionValue is the core claim: `ref add` and
// `add/r` yield a Quoted Function value carrying the FnDefInfo, not
// the result of invocation.
func TestRefWordReturnsFunctionValue(t *testing.T) {
	r := freshRegistry(t)
	for _, src := range []string{"ref add", "add/r"} {
		out := runSrc(t, r, src)
		if len(out) != 1 {
			t.Fatalf("%s: got %d values, want 1", src, len(out))
		}
		v := out[0]
		if !v.Parent.Equal(eng.TFunction) {
			t.Errorf("%s: top.Parent=%s, want Function", src, v.Parent.String())
		}
		fnDef, ok := v.Data.(eng.FnDefInfo)
		if !ok {
			t.Fatalf("%s: payload=%T, want FnDefInfo", src, v.Data)
		}
		if fnDef.Name != "add" {
			t.Errorf("%s: fnDef.Name=%q, want %q", src, fnDef.Name, "add")
		}
		if !v.Quoted {
			t.Errorf("%s: function value not Quoted — would auto-invoke on stack", src)
		}
	}
}

// TestRefOnSimpleValueBinding shows ref is uniform across binding
// shapes: a simple-value def is returned as itself.
func TestRefOnSimpleValueBinding(t *testing.T) {
	r := freshRegistry(t)
	for _, src := range []string{"ref answer", "answer/r"} {
		out := runSrc(t, r, src)
		if len(out) != 1 {
			t.Fatalf("%s: got %d values, want 1", src, len(out))
		}
		got, err := eng.AsInteger(out[0])
		if err != nil {
			t.Fatalf("%s: AsInteger: %v", src, err)
		}
		if got != 42 {
			t.Errorf("%s: got %d, want 42", src, got)
		}
	}
}

// TestRefUndefinedNameErrors covers the "name is not bound" path —
// same error code the regular undefined-word path raises.
func TestRefUndefinedNameErrors(t *testing.T) {
	r := freshRegistry(t)
	for _, src := range []string{"ref nope", "nope/r"} {
		err := runSrcErr(t, r, src)
		if err == nil {
			t.Fatalf("%s: expected error, got nil", src)
		}
		if !strings.Contains(err.Error(), "undefined") && !strings.Contains(err.Error(), "not bound") {
			t.Errorf("%s: error=%q, want undefined/not-bound", src, err.Error())
		}
	}
}

// --- the stable-map-lookup demonstration ------------------------------

// TestRefStableInMap proves the point that motivated the word: a map
// built with ref-resolved entries keeps each function as a stable
// referent. Mutations to the binding (rebind, undef) don't propagate
// to the values already captured into the map.
func TestRefStableInMap(t *testing.T) {
	r := freshRegistry(t)

	// Parser context inside `{...}` is *data*, so bare names in the
	// value positions become atoms, not words. To get the ref-resolved
	// function values into the map we use an explicit constructor:
	// build the OrderedMap directly, populate it via ref's handler, and
	// stash it on DefTable. (No lang map-building word exists at the
	// eng layer.)
	ops := eng.NewOrderedMap()
	for _, name := range []string{"add", "mul"} {
		v, ok := resolveViaRef(t, r, name)
		if !ok {
			t.Fatalf("resolveViaRef(%q): not bound", name)
		}
		ops.Set(name, v)
	}

	// Both stored entries are Function values, not raw words.
	for _, name := range []string{"add", "mul"} {
		v, ok := ops.Get(name)
		if !ok {
			t.Fatalf("ops[%q] missing", name)
		}
		if !v.Parent.Equal(eng.TFunction) {
			t.Errorf("ops[%q].Parent=%s, want Function", name, v.Parent.String())
		}
		if !v.Quoted {
			t.Errorf("ops[%q] not Quoted — would auto-invoke", name)
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
	// (without popping the underlying FnDef). The map entry we already
	// captured must still hold the original Function value — the map
	// stores referents, not names that get re-resolved.
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
	// entirely (so `ref add` would now fail), the previously captured
	// value still carries the full FnDef payload. The runtime can
	// surface that payload to a consumer (filter, walk, a future
	// `apply`) and dispatch independently of the registry.
	if !r.Defs.Pop("add") { // pop the shadow
		t.Fatal("Defs.Pop(shadow) returned false")
	}
	if !r.Defs.Pop("add") { // pop the original
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

// resolveViaRef runs `<name>/r` through the engine and returns the
// resulting value. This is the production path the suffix-resolving
// integration tests should exercise; using it here keeps the
// map-build test honest about how a real program would populate the
// dispatch table.
func resolveViaRef(t *testing.T, r *eng.Registry, name string) (eng.Value, bool) {
	t.Helper()
	out := runSrc(t, r, name+"/r")
	if len(out) != 1 {
		return eng.Value{}, false
	}
	return out[0], true
}

// TestRefHandlerDirect exercises the ref-word path (as opposed to the
// /r suffix path) with a parsed program. Both arrive at resolveRef;
// asserting them separately catches regressions in either entry
// point.
func TestRefHandlerDirect(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "ref mul")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1", len(out))
	}
	v := out[0]
	if !v.Parent.Equal(eng.TFunction) {
		t.Fatalf("Parent=%s, want Function", v.Parent.String())
	}
	if !v.Quoted {
		t.Error("returned function not Quoted")
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "mul" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "mul")
	}
}

// TestRefWordRegistered guards that the kernel actually shipped the
// `ref` word — protects against the registration drifting out of
// NewRegistry.
func TestRefWordRegistered(t *testing.T) {
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, ok := r.Defs.Top("ref"); !ok {
		t.Fatal("ref word missing from fresh registry")
	}
}
