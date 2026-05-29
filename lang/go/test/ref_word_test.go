package test

import (
	"strings"
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

// TestRefOnNonFunctionBindingIsIllegal: both surfaces (`ref` and the
// `/r` suffix) are legal ONLY for function words. Referencing a non-fn
// binding raises illegal_ref — there is no call/value asymmetry to
// break for a plain value, so the reference is meaningless.
func TestRefOnNonFunctionBindingIsIllegal(t *testing.T) {
	for _, src := range []string{`answer/r`, `ref answer`} {
		_, err := runNativeSteps(t, nil, []string{
			`def answer 42`,
			src,
		})
		if err == nil {
			t.Errorf("%s: expected illegal_ref error, got nil", src)
			continue
		}
		if !strings.Contains(err.Error(), "function word") {
			t.Errorf("%s: error=%q, want mention of 'function word'", src, err.Error())
		}
	}
}

// --- direct dispatch of unquoted Function values --------------------

// TestRefSuffixHoldsArgsUndispatched: `/r` is a pure reference and does
// NOT dispatch — it advances the pointer, so `myadd/r 2 3` holds the
// function and leaves the args untouched: [Function, 2, 3]. The call is
// written `myadd 2 3` (bare word) or via `apply` (TestApplyOnQuotedCapture).
func TestRefSuffixHoldsArgsUndispatched(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`myadd/r 2 3`,
	})
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3 [Function 2 3]: %v", len(result), result)
	}
	if !result[0].Parent.Equal(eng.TFunction) {
		t.Errorf("result[0].Parent=%s, want Function (held, not dispatched)", result[0].Parent.String())
	}
	if a, _ := eng.AsInteger(result[1]); a != 2 {
		t.Errorf("result[1]=%v, want 2 (arg not consumed)", result[1])
	}
	if b, _ := eng.AsInteger(result[2]); b != 3 {
		t.Errorf("result[2]=%v, want 3 (arg not consumed)", result[2])
	}
	// Call path: the bare word still dispatches.
	bare, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`myadd 2 3`,
	})
	if err != nil {
		t.Fatalf("bare: %v", err)
	}
	if got, _ := eng.AsInteger(bare[0]); got != 5 {
		t.Errorf("myadd 2 3 = %d, want 5", got)
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

// TestRefInListHoldsFunctionAnyArity pins the pure-reference contract:
// `/r` advances the pointer and never dispatches, so a function
// referenced inside a list is held as data regardless of arity — a 0-arg
// fn is NOT fired in place. (To run it, call the bare word, `apply` it,
// or access it as a member where `get` brings the value live.)
func TestRefInListHoldsFunctionAnyArity(t *testing.T) {
	cases := []struct{ name, def string }{
		{"0-arg", `def f fn [[] [Integer] [42]]`},
		{"2-arg", `def f fn [[a:Integer b:Integer] [Integer] [a add b]]`},
	}
	for _, c := range cases {
		res, err := runNativeSteps(t, nil, []string{c.def, `[f/r]`})
		if err != nil {
			t.Fatalf("%s [f/r]: %v", c.name, err)
		}
		l, _ := eng.AsList(res[0])
		if l.Len() != 1 || !l.Get(0).Parent.Equal(eng.TFunction) {
			t.Errorf("%s [f/r] = %v, want [Function] (held, not dispatched)", c.name, res[0])
		}
	}
}
