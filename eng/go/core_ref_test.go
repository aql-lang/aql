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
// `apply` word, neither of which lives here any more. They test
// stepWord's ForceRef branch and eng.ResolveRef directly.
func freshRegistry(t *testing.T) *eng.Registry {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// `add` and `mul`: typical binary integer fns. RegisterNativeFunc
	// pushes a FnDefInfo entry into DefTable, which is precisely the
	// shape a user-level `def add fn […]` produces — so ResolveRef
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
	// A non-function binding, so we can prove /r on a plain value
	// just returns the value verbatim.
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

// TestBareWordInvokesFnBinding pins the existing behavior /r is
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

// TestRefSuffixReturnsFunctionValue is the core claim of /r: it
// yields a Quoted Function value carrying the FnDefInfo, not the
// result of invocation.
func TestRefSuffixReturnsFunctionValue(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "add/r")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1", len(out))
	}
	v := out[0]
	if !v.Parent.Equal(eng.TFunction) {
		t.Errorf("top.Parent=%s, want Function", v.Parent.String())
	}
	fnDef, ok := v.Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("payload=%T, want FnDefInfo", v.Data)
	}
	if fnDef.Name != "add" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "add")
	}
	if !v.Quoted {
		t.Errorf("function value not Quoted — would auto-invoke on stack")
	}
}

// TestRefSuffixOnSimpleValueBinding: /r is uniform across binding
// shapes — a simple-value def is returned as itself.
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

// TestRefSuffixUndefinedNameErrors covers the "name is not bound"
// path — same error code as the regular undefined-word path.
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

// TestRefStableInMap proves the point that motivated the feature: a
// map built with /r-resolved entries keeps each function as a stable
// referent. Mutations to the binding (shadow, pop) don't propagate to
// the values already captured into the map.
func TestRefStableInMap(t *testing.T) {
	r := freshRegistry(t)

	// Parser context inside `{...}` is *data*, so bare names in the
	// value positions become atoms, not words. To get the ref-resolved
	// function values into the map we build the OrderedMap directly,
	// populate it through the /r path, and stash it. (Map-building via
	// AQL surface lives in the language layer; the kernel test just
	// exercises eng primitives.)
	ops := eng.NewOrderedMap()
	for _, name := range []string{"add", "mul"} {
		v, ok := resolveViaSlashR(t, r, name)
		if !ok {
			t.Fatalf("resolveViaSlashR(%q): not bound", name)
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
	// entirely (so `add/r` would now fail), the previously captured
	// value still carries the full FnDef payload. The runtime can
	// surface that payload to a consumer (filter, walk, or apply) and
	// dispatch independently of the registry.
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
// resulting value. Using the production /r path here keeps the
// map-build test honest about how a real program would populate the
// dispatch table.
func resolveViaSlashR(t *testing.T, r *eng.Registry, name string) (eng.Value, bool) {
	t.Helper()
	out := runSrc(t, r, name+"/r")
	if len(out) != 1 {
		return eng.Value{}, false
	}
	return out[0], true
}

// TestResolveRefDirect exercises the exported helper independently of
// the parser. Same surface, different entry — proves lang's `ref`
// handler can rely on identical semantics when it calls in.
func TestResolveRefDirect(t *testing.T) {
	r := freshRegistry(t)
	v, ok := eng.ResolveRef(r, "mul")
	if !ok {
		t.Fatal("ResolveRef(mul): not bound")
	}
	if !v.Parent.Equal(eng.TFunction) {
		t.Errorf("Parent=%s, want Function", v.Parent.String())
	}
	if !v.Quoted {
		t.Error("returned function not Quoted")
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "mul" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "mul")
	}

	// Non-bound name reports false rather than panicking.
	if _, ok := eng.ResolveRef(r, "nope"); ok {
		t.Error("ResolveRef(nope): expected not-bound, got ok")
	}
}
