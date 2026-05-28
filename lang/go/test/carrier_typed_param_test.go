package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// TestCheckModeCarrierThroughTypedParam is the check-mode negative
// safety net for the `Data == nil` → predicate migration.
//
// In check mode every literal is stripped to a CARRIER (type-only,
// payload elided). A list/map carrier is the subtle case the migration
// turned on: it carries a ChildTypeInfo payload (Data != nil) yet is
// NOT concrete, so it must be classified by IsBareTypeNode /
// IsConcrete, never by a raw `Data == nil` probe. These programs route
// such carriers through typed fn parameters so the migrated guards run
// in their native habitat (matchSignature / signature.go's concrete-
// list/map rule, the refine/predicate unifiers).
//
// Two properties are asserted for every case:
//  1. No panic — the headline guarantee. A carrier reaching AsList /
//     AsMap (which return nil for a ChildTypeInfo payload) must never
//     dereference nil. The migration's IsConcrete / IsBareTypeNode
//     guards are what keep this safe.
//  2. The carrier is accepted or rejected by the param type exactly as
//     a concrete value of the same shape would be — a list carrier
//     fills a [l:List] slot (it is NOT a bare type literal, so the
//     concrete-container rule at signature.go must let it through),
//     and is rejected on a family/refinement mismatch.
func TestCheckModeCarrierThroughTypedParam(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantMatch bool // true: carrier accepted by g's param; false: rejected
	}{
		// List carrier into a concrete-List slot — the load-bearing
		// case. signature.go's concrete-list/map rule rejects bare type
		// literals here; a carrier (IsBareTypeNode == false) must pass.
		{"list carrier -> [l:List]", "def g fn [[l:List] [Integer] [99]] g [1,2,3]", true},
		// Map carrier into a concrete-Map slot — symmetric.
		{"map carrier -> [m:Map]", "def g fn [[m:Map] [Integer] [99]] g {a:1}", true},
		// Any accepts every carrier.
		{"list carrier -> [x:Any]", "def g fn [[x:Any] [Integer] [99]] g [1,2,3]", true},

		// Family mismatches — the carrier must be rejected, not crash.
		{"list carrier -> [n:Integer]", "def g fn [[n:Integer] [Integer] [99]] g [1,2,3]", false},
		{"map carrier -> [l:List]", "def g fn [[l:List] [Integer] [99]] g {a:1}", false},
		// A bare-refine param (bareRefineUnifier) admits its base type
		// (Integer) but must reject a List carrier.
		{"list carrier -> [n:Pos]", "def Pos refine Integer def g fn [[n:Pos] [Integer] [99]] g [1,2,3]", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := checkNoPanic(t, c.src)

			// g always returns Integer; the checker assumes a best-fit
			// candidate and continues even on a mismatch, so the
			// residual stack is [Integer] in every case.
			if len(res.Stack) != 1 || res.Stack[0] != "Integer" {
				t.Fatalf("expected residual stack [Integer], got %v", res.Stack)
			}

			rejected := false
			for _, d := range res.Diagnostics {
				if d.Code == "no_signature" && d.Word == "g" {
					rejected = true
				}
			}
			switch {
			case c.wantMatch && rejected:
				t.Errorf("expected carrier to MATCH g's param, but g was rejected: %+v", res.Diagnostics)
			case !c.wantMatch && !rejected:
				t.Errorf("expected carrier to be REJECTED by g's param, but it matched (diags: %+v)", res.Diagnostics)
			}
		})
	}
}

// checkNoPanic runs a.Check(src) and fails the test (rather than
// crashing the run) if the checker panics — the property the migration
// must preserve when carriers reach the type-vs-value guards.
func checkNoPanic(t *testing.T, src string) (res lang.CheckResult) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("check panicked on %q: %v", src, r)
		}
	}()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err = a.Check(src)
	if err != nil {
		// A returned error is an acceptable, non-panicking outcome; only
		// surface it if it's an unexpected internal failure.
		if !strings.Contains(err.Error(), "signature") {
			t.Fatalf("unexpected check error on %q: %v", src, err)
		}
	}
	return res
}
