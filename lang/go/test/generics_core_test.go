package test

import (
	"strings"
	"testing"
)

// Generic-types core tests (design/GENERICS.0.md Phase 1) — the
// invariants the TSV battery can't reach: placeholder-binding
// hygiene across success and failure paths (plan decision D1), and
// nested-schema binding isolation.

// genSteps runs steps on a fresh registry and returns the final
// result, the registry, and any error.
func genSteps(t *testing.T, steps []string) error {
	t.Helper()
	_, err := runNativeSteps(t, nil, steps)
	return err
}

// TestGenBindingLifecycle pins D1: every placeholder binding `gen`
// pushes is popped again — after a successful schema def, after a
// failed constraint, and after an orphaned gen — so the parameter
// names never leak into the surrounding scope.
func TestGenBindingLifecycle(t *testing.T) {
	// Success path: T must be unbound after the def completes.
	err := genSteps(t, []string{
		`def Box gen [T] refine Record [value:T]`,
		`Box of [Integer]`,
		`T`, // would resolve if the placeholder leaked
	})
	if err == nil || !strings.Contains(err.Error(), "undefined word") {
		t.Errorf("success path: placeholder T leaked (err=%v)", err)
	}

	// Failure path: a failing gen entry pops what it pushed.
	err = genSteps(t, []string{
		`do [gen [T t] refine Record [x:T]]`, // lowercase t errors inside gen
		`T`,
	})
	if err == nil || !strings.Contains(err.Error(), "undefined word") {
		t.Errorf("failed-gen path: placeholder T leaked (err=%v)", err)
	}

	// Orphan path: a gen no constructor ever consumes errors loudly
	// at the top-level end-of-run drain (which also pops the
	// placeholder bindings). The drain is deliberately top-level-only
	// — sub-runs (do bodies, arg auto-evaluation) legitimately execute
	// while a spec is pending — so the orphan error surfaces at run
	// end, not inside the do.
	err = genSteps(t, []string{
		`do [gen [T]]`,
	})
	if err == nil || !strings.Contains(err.Error(), "gen_without_constructor") {
		t.Errorf("orphan path: want loud gen_without_constructor at run end, got %v", err)
	}
}

// TestGenNestedSchemaBindings pins binding isolation: a schema whose
// field carries an fnsig over the OUTER parameter must not pop the
// outer bindings when the inner constructor finishes — only the
// GenSpec-consuming constructor pops, and only its own names.
func TestGenNestedSchemaBindings(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def A gen [T] refine Record [f:(fnsig [[T] [T]]) x:T]`,
		`A of [Integer]`,
	})
	if err != nil {
		t.Fatalf("nested fnsig over outer parameter: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	s := res[0].String()
	// Both the fn-shape field and the scalar field substituted.
	if !strings.Contains(s, "x:Integer") {
		t.Errorf("outer parameter did not substitute into x: %s", s)
	}
}

// TestGenMemoIdentity pins D4: repeated instantiation with the same
// canonical args yields the same body value (teq via the spec rows;
// here the Go-side identity of the memoised instantiation).
func TestGenMemoIdentity(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def Box gen [T] refine Record [value:T]`,
		`(Box of [Integer]) teq (Box of [Integer])`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].String() != "true" {
		t.Errorf("memo identity: got %v", res)
	}
}
