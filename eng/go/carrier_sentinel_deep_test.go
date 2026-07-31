package eng

import "testing"

// White-box arms of the transitive sentinel scan (bodyHasSentinelDeep) that
// source-level fixtures cannot reach: the nil-registry decline and a
// CONSTRUCTED FnDefInfo payload sitting in a scanned body (source bodies
// carry raw tokens; a payload appears only in a programmatically-built
// body). The def-bound rows double as a minimal harness for the Lookup
// resolution (a pushed fn binding IS the name's dispatch table).
func TestBodyHasSentinelDeepArms(t *testing.T) {
	breakFn := NewFnDef(FnDefInfo{Name: "zzleak", Signatures: []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}}, Returns: []*Type{TInteger},
		Impl: BORU([]Value{NewWord("break"), NewInteger(7)}), BarrierPos: BarrierAllForward,
	}}})
	cleanFn := NewFnDef(FnDefInfo{Name: "zzclean", Signatures: []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}}, Returns: []*Type{TInteger},
		Impl: BORU([]Value{NewWord("x")}), BarrierPos: BarrierAllForward,
	}}})

	// nil registry: no resolution possible — only the direct scan applies.
	if bodyHasSentinelDeep(nil, NewList([]Value{NewWord("zzleak")})) {
		t.Error("nil registry: word must not resolve")
	}
	if !bodyHasSentinelDeep(nil, NewList([]Value{NewWord("break")})) {
		t.Error("nil registry: the direct scan must still see a bare break")
	}

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A constructed FnDefInfo payload IN the body scans its sig bodies.
	if !bodyHasSentinelDeep(r, NewList([]Value{breakFn, NewInteger(1)})) {
		t.Error("constructed breaking-fn payload in the body must leak")
	}
	if bodyHasSentinelDeep(r, NewList([]Value{cleanFn, NewInteger(1)})) {
		t.Error("constructed sentinel-free fn payload must not leak")
	}

	// A def-BOUND fn value resolves through r.Defs.Top (not r.Lookup).
	r.Defs.Push("zzleak", breakFn)
	r.Defs.Push("zzclean", cleanFn)
	if !bodyHasSentinelDeep(r, NewList([]Value{NewWord("zzleak"), NewInteger(1)})) {
		t.Error("def-bound breaking fn must leak through the Defs.Top arm")
	}
	if bodyHasSentinelDeep(r, NewList([]Value{NewWord("zzclean"), NewInteger(1)})) {
		t.Error("def-bound sentinel-free fn must not leak")
	}
}
