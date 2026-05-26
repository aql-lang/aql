package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The `ref` word and `/r` suffix exist to break the asymmetry between
// value bindings (where a bare name pushes the value) and fn bindings
// (where a bare name invokes). Programs that need a stable handle to
// a function — a dispatch table, an event-handler map, anything that
// wants to store callables by key — depend on this opt-out. These
// tests drive the feature end-to-end through real AQL source so that
// the parser, def stack, and map-builder all stay honest.

// TestRefBuildsDispatchTable is the headline demonstration: two
// functions defined under names, both captured into one map by
// `/r`, then looked up by key. The retrieved values must be
// Function values (not the result of invocation) and the map
// entries must carry the same FnDef payload as the defs they
// reference.
func TestRefBuildsDispatchTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def mymul fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
		// Both ref forms in one map — proves the parser routes /r and
		// `ref name` through the same resolveRef path inside data
		// context, and that autoEvalMap evaluates the resulting
		// WordRef / atom-bearing-ref slots correctly.
		`def ops {plus: myadd/r times: (ref mymul)}`,
		`ops`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	m, _ := native.AsMap(result[0])
	if m == nil {
		t.Fatalf("expected map, got %s", result[0].Parent.String())
	}

	for _, key := range []string{"plus", "times"} {
		v, ok := m.Get(key)
		if !ok {
			t.Fatalf("ops[%q] missing", key)
		}
		if !v.Parent.Equal(eng.TFunction) {
			t.Errorf("ops[%q].Parent = %s, want Function — value was invoked, not captured", key, v.Parent.String())
		}
		if !v.Quoted {
			t.Errorf("ops[%q] is not Quoted — would auto-invoke when pushed back on the stack", key)
		}
		fnDef, ok := v.Data.(eng.FnDefInfo)
		if !ok {
			t.Fatalf("ops[%q] payload type = %T, want FnDefInfo", key, v.Data)
		}
		wantName := map[string]string{"plus": "myadd", "times": "mymul"}[key]
		if fnDef.Name != wantName {
			t.Errorf("ops[%q] fnDef.Name = %q, want %q", key, fnDef.Name, wantName)
		}
		// AQL-defined fns ship with FnSig overloads; this is what
		// makes the captured value invocable later (via TFunction sig
		// slots in higher-order words, or a future apply primitive).
		if len(fnDef.Sigs) == 0 {
			t.Errorf("ops[%q] captured FnDef has no Sigs — handle is hollow", key)
		}
	}
}

// TestRefMapRetrievalViaDot proves the dispatch-table pattern with
// the surface syntax a user would actually write: `ops.plus` (which
// expands to `get plus ops`) returns the stable Function value.
func TestRefMapRetrievalViaDot(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {plus: myadd/r}`,
		`ops.plus`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	v := result[0]
	if !v.Parent.Equal(eng.TFunction) {
		t.Fatalf("ops.plus type = %s, want Function (would mean dot-access invoked the value)", v.Parent.String())
	}
	if !v.Quoted {
		t.Error("ops.plus not Quoted — would auto-invoke on the stack")
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "myadd" {
		t.Errorf("ops.plus fnDef.Name = %q, want %q", fnDef.Name, "myadd")
	}
}

// TestRefSurvivesRedefinition proves the "stable" half of the
// stable-map claim: rebinding the original name with a *different*
// fn does not change the map entry. The map holds the referent at
// the moment ref ran, not a deferred name lookup.
func TestRefSurvivesRedefinition(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myop fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {go: myop/r}`,
		// Replace myop with a completely different fn — multiplication
		// instead of addition. The map should still hold the addition
		// referent.
		`undef myop`,
		`def myop fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
		`ops.go`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	v := result[0]
	if !v.Parent.Equal(eng.TFunction) {
		t.Fatalf("ops.go type = %s, want Function", v.Parent.String())
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "myop" {
		t.Errorf("ops.go captured name = %q, want %q", fnDef.Name, "myop")
	}
	if len(fnDef.Sigs) == 0 {
		t.Fatal("captured Sigs empty — can't verify body identity")
	}
	// The captured FnDef body's first instruction tells us whether
	// we have the original (add) or the replacement (mul). Reading
	// the body verbatim is brittle to FnSig layout changes but for
	// this simple shape it's a clear identity proof.
	body := fnDef.Sigs[0].Body
	if len(body) == 0 {
		t.Fatal("captured Sigs[0].Body empty")
	}
	// The body for [a add b] contains a Word "add". The replacement's
	// body would contain "mul".
	var saw string
	for _, tok := range body {
		if eng.IsWord(tok) {
			w, _ := eng.AsWord(tok)
			if w.Name == "add" || w.Name == "mul" {
				saw = w.Name
				break
			}
		}
	}
	if saw != "add" {
		t.Errorf("captured body op = %q, want %q (rebind leaked into map)", saw, "add")
	}
}

// TestRefOnUndefinedNameErrors aligns ref's failure mode with bare-
// word resolution: undefined name is an error, not a silent atom.
func TestRefOnUndefinedNameErrors(t *testing.T) {
	for _, src := range []string{`ref nope`, `nope/r`} {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%s: expected error, got nil", src)
			continue
		}
	}
}

// TestRefOnSimpleValueBindingPassesThrough demonstrates ref's
// uniformity — for a non-fn binding, ref returns the same value
// that a bare name would have substituted in.
func TestRefOnSimpleValueBindingPassesThrough(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def answer 42`,
		`answer/r`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	v := result[0]
	got, err := eng.AsInteger(v)
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 42 {
		t.Errorf("answer/r = %d, want 42", got)
	}
}
