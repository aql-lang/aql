package lang

import (
	"strings"
	"testing"
)

// TestGenericRecordReturnConformsToMap pins the check-mode typing of `make`
// over a GENERIC record schema (`gen [R] refine Record […]`). The schema node
// is minted in the Ideal branch (Ideal/Record), but the runtime instance is
// built by MakeRecordR and carries the TMap tag — so a fn declaring a `[Map]`
// return and returning `make Rule {…}` passes at run time and must check
// clean too. Before the ReturnsFreshInstance fix the check carrier was typed
// at the schema node and failed Map conformance: "return value 1: expected
// Map, got Rule" (the voxgig decision make-rule/make-table/make-leaf FPs).
func TestGenericRecordReturnConformsToMap(t *testing.T) {
	const src = `def Rule gen [R] refine Record [w:Map t:R]
def mk fn [[w:Map t:Map] [Map] [make Rule {w:w t:t}]]
def r (mk {} {})`
	a, _ := New()
	res, _ := a.Check(src)
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("generic-record return against [Map] must check clean (runtime tags the instance TMap): %s: %s", d.Code, d.Detail)
		}
	}

	// NEGATIVE: the faithful tag must not become a blank pass — a
	// generic-record instance returned against a SCALAR declared return is a
	// genuine mismatch the runtime raises on, and must stay flagged.
	const bad = `def Rule gen [R] refine Record [w:Map t:R]
def mk fn [[w:Map t:Map] [Integer] [make Rule {w:w t:t}]]
def r (mk {} {})`
	b, _ := New()
	res2, _ := b.Check(bad)
	flagged := false
	for _, d := range res2.Diagnostics {
		if d.Severity == "error" && (strings.Contains(d.Detail, "return") || d.Code == "type_error") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Error("a generic-record instance returned against [Integer] must stay flagged")
	}
}

// PR #218 review (Codex P2s): the faithful Map tag must not swallow a
// CONCRETE construction the runtime rejects. With fully-literal data the
// checker validates statically (infer + instantiate + MakeRecordR) and a
// failure keeps the schema-node carrier, whose failed Map conformance flags
// the enclosing [Map] return.
func TestGenericRecordConcreteConstructionFailuresFlag(t *testing.T) {
	cases := []struct{ name, src string }{
		{"uninferable param", `def Pair gen [K V] refine Record [key:K value:V]
def mk fn [[m:Map] [Map] [make Pair {key:"a"}]]
(mk {})`},
		{"unknown extra field", `def Pair gen [K] refine Record [key:K]
def mk fn [[m:Map] [Map] [make Pair {key:"a" extra:1}]]
(mk {})`},
	}
	for _, c := range cases {
		a, _ := New()
		res, _ := a.Check(c.src)
		flagged := false
		for _, d := range res.Diagnostics {
			if d.Severity == "error" {
				flagged = true
				break
			}
		}
		if !flagged {
			t.Errorf("%s: a concrete generic-record construction the runtime rejects must flag", c.name)
		}
	}

	// Carrier-bearing construction data (field values statically unknown —
	// the decision with-policy shape) stays on the optimistic Map carrier.
	const gradual = `def DT gen [R] refine Record [kind:String hp:R]
def wp fn [[p:String t:Map] [Map] [make DT {kind:(t get "kind") hp:p}]]
(wp "first" {kind:"table"})`
	b, _ := New()
	res2, _ := b.Check(gradual)
	for _, d := range res2.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("carrier-bearing generic-record construction must stay unflagged: %s", d.Detail)
		}
	}
}
