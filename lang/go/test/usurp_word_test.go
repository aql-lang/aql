package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The `usurp` word and `/u` suffix wrap a function so its signature
// argument order is reversed: a usurped fn called `usurped a b c`
// dispatches the original as `f c b a`. Like `ref`/`/r`, the wrapper is an
// unquoted Function value — it dispatches when args follow, and stays inert
// under `quote` or when reffed (`/ur`).

// lastInt returns the integer value of the final stack entry.
func lastInt(t *testing.T, res []native.Value) int64 {
	t.Helper()
	if len(res) == 0 {
		t.Fatalf("empty result stack")
	}
	n, err := native.AsInteger(res[len(res)-1])
	if err != nil {
		t.Fatalf("final value is not an integer: %v (%s)", err, res[len(res)-1].String())
	}
	return n
}

// lastString returns the string value of the final stack entry.
func lastString(t *testing.T, res []native.Value) string {
	t.Helper()
	if len(res) == 0 {
		t.Fatalf("empty result stack")
	}
	s, err := native.AsString(res[len(res)-1])
	if err != nil {
		t.Fatalf("final value is not a string: %v (%s)", err, res[len(res)-1].String())
	}
	return s
}

// TestUsurpWordReversesArgs checks the `usurp` word reverses arg delivery
// for a non-commutative fn: sub computes a-b, so usurp(sub) 10 3 = 3-10.
func TestUsurpWordReversesArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`usurp (ref sub2) 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("usurp (ref sub2) 10 3 = %d, want -7", got)
	}
}

// TestUsurpModifierReversesArgs checks the /u suffix reverses the same way.
func TestUsurpModifierReversesArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`sub2/u 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("sub2/u 10 3 = %d, want -7", got)
	}
}

// TestUsurpThreeArgFullReversal checks every position is reversed.
func TestUsurpThreeArgFullReversal(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def cat3 fn [[a:String b:String c:String] [String] [a add b add c]]`,
		`cat3/u 'x' 'y' 'z'`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, res); got != "zyx" {
		t.Errorf("cat3/u 'x' 'y' 'z' = %q, want \"zyx\"", got)
	}
}

// TestUsurpHeterogeneousSig checks the usurped sig matches reversed types:
// f's sig is [a:Integer b:String], so f/u expects [String Integer] and
// still binds a=Integer, b=String on the original.
func TestUsurpHeterogeneousSig(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def f fn [[a:Integer b:String] [String] [b add (a convert String)]]`,
		`f/u 'hi' 7`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, res); got != "hi7" {
		t.Errorf("f/u 'hi' 7 = %q, want \"hi7\"", got)
	}
}

// TestUsurpOneArgIsNoOp checks usurp of a 1-arg fn dispatches normally.
func TestUsurpOneArgIsNoOp(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`inc/u 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != 6 {
		t.Errorf("inc/u 5 = %d, want 6", got)
	}
}

// TestUsurpReffedIsInertData checks `/ur` (usurp + ref) with no following
// args leaves the wrapper on the stack as a Function value rather than
// dispatching it.
func TestUsurpReffedIsInertData(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`inc/ur`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("inc/ur left %d values, want 1 (the wrapper)", len(res))
	}
	if !res[0].Parent.Equal(eng.TFunction) {
		t.Errorf("inc/ur top is %s, want Function", res[0].Parent.String())
	}
}

// TestUsurpReffedStaysInertWithTrailingArgs checks `/ur` mirrors `/r`: the
// reffed wrapper is left on the stack as data and bare trailing values just
// pile up beside it (no auto-dispatch). To invoke a reffed wrapper, wrap it
// in a paren — `(usurp (ref f)) a b` — or store and dot-dispatch it.
func TestUsurpReffedStaysInertWithTrailingArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`sub2/ur 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// [wrapper 10 3] — the wrapper sits inert below the two literals.
	if len(res) != 3 {
		t.Fatalf("sub2/ur 10 3 left %d values, want 3 (wrapper + 2 literals)", len(res))
	}
	if !res[0].Parent.Equal(eng.TFunction) {
		t.Errorf("bottom value is %s, want the inert Function wrapper", res[0].Parent.String())
	}
}

// TestUsurpReffedInvokesViaParen checks a reffed wrapper dispatches when
// grouped in a paren with its args (the canonical way to call a captured
// function value).
func TestUsurpReffedInvokesViaParen(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`(usurp (ref sub2)) 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("(usurp (ref sub2)) 10 3 = %d, want -7", got)
	}
}

// TestUsurpQuotedIsInertData checks `quote (usurp …)` captures the wrapper
// as data without invoking it.
func TestUsurpQuotedIsInertData(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`quote (usurp (ref inc))`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("quoted usurp left %d values, want 1", len(res))
	}
	if !res[0].Parent.Equal(eng.TFunction) {
		t.Errorf("quoted usurp top is %s, want Function", res[0].Parent.String())
	}
}

// TestUsurpSurvivesRedefinition checks a usurped wrapper captured into a
// map is a stable handle to the fn it was minted from: dot-dispatching it
// after the original name is redefined still reverses the ORIGINAL
// subtraction (not the new addition).
func TestUsurpSurvivesRedefinition(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def ops {rev: (usurp (ref sub2))}`,
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`ops.rev 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("captured usurped sub2 after redef = %d, want -7 (reversed subtraction)", got)
	}
}

// TestUsurpStoredInMapDispatches checks the canonical store-and-reuse path:
// a usurped wrapper captured in a map and dot-dispatched against args.
func TestUsurpStoredInMapDispatches(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def ops {rev: (usurp (ref sub2))}`,
		`ops.rev 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("ops.rev 10 3 = %d, want -7", got)
	}
}

// TestUsurpOnUnboundNameErrors checks /u on an unbound name is an error.
func TestUsurpOnUnboundNameErrors(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`nope/u`}); err == nil {
		t.Fatalf("nope/u should error (undefined_word)")
	}
}

// TestUsurpOnNonFunctionIsIllegal checks /u on a plain value is rejected.
func TestUsurpOnNonFunctionIsIllegal(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`def x 5`, `x/u`}); err == nil {
		t.Fatalf("x/u on a non-fn binding should error (illegal_ref)")
	}
}

// TestUsurpWordOnNonFunctionIsIllegal checks the usurp word rejects a
// non-function argument.
func TestUsurpWordOnNonFunctionIsIllegal(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`def x 5`, `usurp x`}); err == nil {
		t.Fatalf("usurp x on a non-fn value should error (illegal_ref)")
	}
}

// TestUsurpCheckModeClean verifies the /u modifier flows through static
// check mode without an error-severity diagnostic (the /u stepWord branch
// mirrors /r's check-mode handling). Like the whole ref family, /u does not
// itself count as a "use" of the name, so an unused_def *warning* for the
// defined fn is expected and tolerated — we only reject errors.
func TestUsurpCheckModeClean(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`
		def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]
		sub2/u 10 3
	`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("clean /u program produced an error diagnostic: %+v", d)
		}
	}
}

// TestUsurpCheckModeUndefined verifies /u on an unbound name surfaces a
// diagnostic in check mode rather than panicking.
func TestUsurpCheckModeUndefined(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`nope/u`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Diagnostics) == 0 {
		t.Errorf("nope/u should produce a diagnostic in check mode")
	}
}
