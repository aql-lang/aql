package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The `ref` word and `/r` suffix break the asymmetry between value
// bindings (where a bare name pushes the value) and fn bindings (where
// a bare name invokes). In the unified dispatch model, both `/r` and
// `ref` produce UNQUOTED Function values — these dispatch with full
// signature matching when the engine processes them, the same as a
// word lookup would. `quote (foo/r)` produces an inert Quoted Function
// value that `apply` can invoke explicitly.

// TestRefBuildsDispatchTable: two fns captured into a map by `/r`,
// retrieved by key. The map slots hold Function VALUES (unquoted —
// they're live call-sites the engine will dispatch when given args).
func TestRefBuildsDispatchTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def mymul fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
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
			t.Errorf("ops[%q].Parent = %s, want Function", key, v.Parent.String())
		}
		if v.Quoted {
			t.Errorf("ops[%q] is Quoted — captured Function should be unquoted (live call-site)", key)
		}
		fnDef, ok := v.Data.(eng.FnDefInfo)
		if !ok {
			t.Fatalf("ops[%q] payload type = %T, want FnDefInfo", key, v.Data)
		}
		wantName := map[string]string{"plus": "myadd", "times": "mymul"}[key]
		if fnDef.Name != wantName {
			t.Errorf("ops[%q] fnDef.Name = %q, want %q", key, fnDef.Name, wantName)
		}
		if len(fnDef.Sigs) == 0 {
			t.Errorf("ops[%q] captured FnDef has no Sigs — handle is hollow", key)
		}
	}
}

// TestRefMapRetrievalViaDotInvokesWithForwardArgs is the headline new
// behavior: `ops.plus 2 3` retrieves the Function value and invokes it
// against the trailing 2 and 3 via full sig matching. Postfix dispatch
// at last.
func TestRefMapRetrievalViaDotInvokesWithForwardArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {plus: myadd/r}`,
		`ops.plus 2 3`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	got, err := eng.AsInteger(result[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 5 {
		t.Errorf("ops.plus 2 3 = %d, want 5", got)
	}
}

// TestRefMapRetrievalAsData: when nothing follows that matches the
// captured fn's signature, the retrieved Function value sits on the
// stack as data.
func TestRefMapRetrievalAsData(t *testing.T) {
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
		t.Fatalf("ops.plus type = %s, want Function", v.Parent.String())
	}
	fnDef, _ := v.Data.(eng.FnDefInfo)
	if fnDef.Name != "myadd" {
		t.Errorf("ops.plus fnDef.Name = %q, want %q", fnDef.Name, "myadd")
	}
}

// TestRefSurvivesRedefinition: rebinding the underlying name doesn't
// change a map entry that captured the original fn via /r.
func TestRefSurvivesRedefinition(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myop fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {go: myop/r}`,
		// Replace myop with multiplication instead of addition.
		`undef myop`,
		`def myop fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
		// The map's captured fn still adds; the new myop multiplies.
		`ops.go 2 3`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := eng.AsInteger(result[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 5 {
		t.Errorf("captured ops.go (add) on 2,3 = %d, want 5 — late-binding leaked into map", got)
	}
}

// TestRefOnUndefinedNameErrors: undefined-name resolution errors via
// both surface forms.
func TestRefOnUndefinedNameErrors(t *testing.T) {
	for _, src := range []string{`ref nope`, `nope/r`} {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%s: expected error, got nil", src)
		}
	}
}

// TestRefOnSimpleValueBindingPassesThrough: ref/r on a non-fn binding
// returns the value verbatim.
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

// --- direct dispatch of unquoted Function values --------------------

// TestRefSuffixDispatchesPrefixArgs: `add/r 2 3` is the unified-rule
// payoff — the Function value at the pointer collects 2 and 3 as
// forward args, just as `add 2 3` would.
func TestRefSuffixDispatchesPrefixArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`myadd/r 2 3`,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := eng.AsInteger(result[0])
	if got != 5 {
		t.Errorf("myadd/r 2 3 = %d, want 5", got)
	}
}

// TestInlineFnLiteralDispatchesWithStackArgs: a bare `(fn [...])`
// expression dispatches with preceding stack args. Anonymous fns
// don't ship compiled Signatures (no name → no InstallFnDef pass),
// so forward collection isn't available — but the FnSig stack-match
// path picks them up.
func TestInlineFnLiteralDispatchesWithStackArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`2 3 (fn [[a:Integer b:Integer] [Integer] [a add b]])`,
	})
	if err != nil {
		t.Fatalf("inline fn: %v", err)
	}
	got, _ := eng.AsInteger(result[0])
	if got != 5 {
		t.Errorf("inline fn dispatch = %d, want 5", got)
	}
}

// --- apply: invokes Quoted Function values explicitly --------------

// TestApplyOnQuotedCapture: a Quoted Function is inert until `apply`
// flips the Quoted flag. The engine then dispatches via full sig
// matching against preceding stack args. We use `(quote (myadd/r))`
// to evaluate /r first (producing an unquoted Function), then wrap
// in `quote` to mark it as data.
func TestApplyOnQuotedCapture(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`2 3 (quote (myadd/r)) apply`,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := eng.AsInteger(result[0])
	if got != 5 {
		t.Errorf("2 3 (quote (myadd/r)) apply = %d, want 5", got)
	}
}

// TestApplyErrorsOnNonFunction: type check still rejects non-fn
// values.
func TestApplyErrorsOnNonFunction(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`42 apply`,
	})
	if err == nil {
		t.Fatal("expected error applying to Integer, got nil")
	}
}

// TestZeroArgRefFiresInListButHeldInMap pins the documented /r boundary
// (REFERENCE.md "Dotted access binds tightly"): /r yields a
// *dispatchable* function value, so a 0-arg fn referenced by /r fires
// when it is stepped — in a list it runs in place. A >=1-arg fn has no
// 0-arg signature, so it is held until its args arrive. (A direct map
// value holds even a 0-arg fn as data — covered by the module-export
// tests; here we pin the list behavior.)
func TestZeroArgRefFiresInListButHeldInMap(t *testing.T) {
	// 0-arg fn: fires in place inside a list.
	res, err := runNativeSteps(t, nil, []string{
		`def zero fn [[] [Integer] [42]]`,
		`[zero/r]`,
	})
	if err != nil {
		t.Fatalf("[zero/r]: %v", err)
	}
	l, _ := eng.AsList(res[0])
	if l.Len() != 1 {
		t.Fatalf("[zero/r] len=%d, want 1", l.Len())
	}
	if got, err := eng.AsInteger(l.Get(0)); err != nil || got != 42 {
		t.Errorf("[zero/r][0] = %v, want 42 (a 0-arg /r fires in a list)", l.Get(0))
	}

	// >=1-arg fn: held as a Function value in a list (no 0-arg sig).
	res, err = runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`[myadd/r]`,
	})
	if err != nil {
		t.Fatalf("[myadd/r]: %v", err)
	}
	l, _ = eng.AsList(res[0])
	if l.Len() != 1 || !l.Get(0).Parent.Equal(eng.TFunction) {
		t.Errorf("[myadd/r][0] = %v, want a held Function", l.Get(0))
	}
}
