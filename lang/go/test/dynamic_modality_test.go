package test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// TestDynamicContextGetGradualMatch demonstrates the bounded dynamic(T)
// modality end to end (design/dynamic-modality-report.0.md): `context
// get` on a statically-untracked key is an escape hatch that emits
// dynamic(Any) — optimistically compatible — instead of strict
// Carry<Any>. That dynamic carrier then matches a typed slot under
// `aql check` (here add's Number) where strict Carry<Any> would not.
//
// `add` has only a [Number, Number] signature (no Any catch-all), so a
// successful match is the gradual not-disjoint rule at work, not a
// fallback. Strict Carry<Any> would fail it (proven directly in
// eng/go/dynamic_match_test.go).
func TestDynamicContextGetGradualMatch(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`context get "k" 1 add`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Fatalf("dynamic(Any) from context get should match the Number slot; got no_signature: %+v", res.Diagnostics)
		}
	}
	if len(res.Stack) != 1 || (res.Stack[0] != "Decimal" && res.Stack[0] != "Integer") {
		t.Fatalf("expected a numeric result from the gradual match, got stack=%v", res.Stack)
	}
}
