package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// White-box arms of the transitive sentinel scan (bodyHasSentinelDeep) that
// source-level fixtures cannot reach: the nil-registry decline and a
// CONSTRUCTED FnDefInfo payload sitting in a scanned body (source bodies
// carry raw tokens; a payload appears only in a programmatically-built
// body). The def-bound rows double as a minimal harness for the Lookup
// resolution (a pushed fn binding IS the name's dispatch table).
func TestBodyHasSentinelDeepArms(t *testing.T) {
	breakFn := core.NewFunction(core.FnDefInfo{Name: "zzleak", Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger},
		Impl: core.Boru([]core.Value{core.NewWord("break"), core.NewInteger(7)}), BarrierPos: core.BarrierAllForward,
	}}})
	cleanFn := core.NewFunction(core.FnDefInfo{Name: "zzclean", Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger},
		Impl: core.Boru([]core.Value{core.NewWord("x")}), BarrierPos: core.BarrierAllForward,
	}}})

	// nil registry: no resolution possible — only the direct scan applies.
	if check.BodyHasSentinelDeep(nil, core.NewList([]core.Value{core.NewWord("zzleak")})) {
		t.Error("nil registry: word must not resolve")
	}
	if !check.BodyHasSentinelDeep(nil, core.NewList([]core.Value{core.NewWord("break")})) {
		t.Error("nil registry: the direct scan must still see a bare break")
	}

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A constructed FnDefInfo payload IN the body scans its sig bodies.
	if !check.BodyHasSentinelDeep(r, core.NewList([]core.Value{breakFn, core.NewInteger(1)})) {
		t.Error("constructed breaking-fn payload in the body must leak")
	}
	if check.BodyHasSentinelDeep(r, core.NewList([]core.Value{cleanFn, core.NewInteger(1)})) {
		t.Error("constructed sentinel-free fn payload must not leak")
	}

	// A def-BOUND fn value resolves through r.Defs.Top (not r.Lookup).
	r.Defs.Push("zzleak", breakFn)
	r.Defs.Push("zzclean", cleanFn)
	if !check.BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzleak"), core.NewInteger(1)})) {
		t.Error("def-bound breaking fn must leak through the Defs.Top arm")
	}
	if check.BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzclean"), core.NewInteger(1)})) {
		t.Error("def-bound sentinel-free fn must not leak")
	}
}
