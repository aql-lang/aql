package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// callResultPlacedIn's arms, driven directly: the shapes that PARK (a
// single-output user call, a user-member arrival apply) and every exclusion
// (nil recorder, no producer, no event, a non-call event, a multi-output
// user call, a dyn-method over a non-user word, a re-stepped lead, a
// def-read lead, a member-read lead). The end-to-end rows are in
// lang/go/returned_closure_park_test.go.
func TestCallResultPlacedArms(t *testing.T) {
	var nilES *EmitState
	if nilES.callResultPlaced(core.NewInteger(1)) {
		t.Error("a nil recorder places nothing")
	}
	es := NewEmitState()
	v := core.NewInteger(1)
	if es.callResultPlaced(v) {
		t.Error("an identity-less value has no producer")
	}
	v.ID = "v-res"
	if es.callResultPlaced(v) {
		t.Error("a value no event produced is not a call result")
	}
	es.producedBy = map[string]producer{"v-res": {seq: 41}}
	if es.callResultPlaced(v) {
		t.Error("a producer seq with no event in the frame resolves to nothing")
	}

	es.frames[0] = append(es.frames[0], EmitEvent{seq: 41, kind: evCallUser, uc: emitUserCall{nout: 1}})
	if !es.callResultPlaced(v) {
		t.Error("a single-output user call's result parks")
	}
	es.frames[0][0].uc.nout = 2
	if es.callResultPlaced(v) {
		t.Error("a multi-output user call's frame rewound over its survivors: not parked")
	}
	es.frames[0][0] = EmitEvent{seq: 41, kind: evBranch}
	if es.callResultPlaced(v) {
		t.Error("a branch merge is not a call result")
	}

	// The exclusions on a parking event.
	es.frames[0][0] = EmitEvent{seq: 41, kind: evCallUser, uc: emitUserCall{nout: 1}}
	es.defReads = map[string]string{"v-res": "h"}
	if es.callResultPlaced(v) {
		t.Error("a def-read lead dispatches (a bare name always calls)")
	}
	delete(es.defReads, "v-res")
	es.NoteMemberFnRead("v-res", core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "f"}})
	if es.callResultPlaced(v) {
		t.Error("a member-read lead is the arrival model's")
	}
	es = NewEmitState()
	es.producedBy = map[string]producer{"v-res": {seq: 41}}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 41, kind: evCallUser, uc: emitUserCall{nout: 1}})
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	r.Check.ParenReSteppedFnIDs = map[string]bool{"v-res": true}
	if es.callResultPlaced(v) {
		t.Error("a lead an enclosing paren re-stepped is a call after all")
	}
	r.Check.ParenReSteppedFnIDs = nil

	// A captured fragment (a fn unit's own residual at finish) is searched
	// before the frame stack.
	frag := &EmitFragment{events: []EmitEvent{{seq: 77, kind: evCallUser, uc: emitUserCall{nout: 1}}}}
	w := core.NewInteger(2)
	w.ID = "v-frag"
	es.producedBy["v-frag"] = producer{seq: 77}
	if !es.callResultPlacedIn(w, frag) {
		t.Error("a unit's own call result is found in its captured fragment")
	}
	if es.callResultPlaced(w) {
		t.Error("...and not on the frame stack, where the seq is absent")
	}

	// The arrival apply of a USER member parks; a dyn-method over a word
	// that is not a user fn (a Go-impl member, the paren-apply pseudo-word)
	// keeps its arm, as does a multi-output one.
	r.Defs.Push("mkr", core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "mkr",
		Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}}}}})
	es.frames[0][0] = EmitEvent{seq: 41, kind: evCall, call: emitCall{word: "mkr", nout: 1,
		dynMethod: &DynMethodSpec{Word: "mkr", NArgs: 1, NOut: 1}}}
	if !es.callResultPlaced(v) {
		t.Error("a user member's arrival-apply result parks")
	}
	es.frames[0][0].call.nout = 2
	if es.callResultPlaced(v) {
		t.Error("a multi-output dyn-method is not parked")
	}
	es.frames[0][0].call.nout = 1
	es.frames[0][0].call.word = "(paren apply)"
	if es.callResultPlaced(v) {
		t.Error("the paren-apply pseudo-word names no user fn")
	}
	es.frames[0][0].call.dynMethod = nil
	es.frames[0][0].call.word = "mkr"
	if es.callResultPlaced(v) {
		t.Error("a plain native call result keeps its arm (a native word's returned fn re-steps)")
	}
}

func TestUserMemberFn(t *testing.T) {
	var nilES *EmitState
	if nilES.userMemberFn("x") {
		t.Error("a nil recorder resolves nothing")
	}
	es := NewEmitState()
	if es.userMemberFn("x") {
		t.Error("a recorder without a registry resolves nothing")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	if es.userMemberFn("") || es.userMemberFn("unbound") {
		t.Error("an empty or unbound name is not a user fn")
	}
	r.Defs.Push("n", core.NewInteger(5))
	if es.userMemberFn("n") {
		t.Error("a scalar binding is not a user fn")
	}
	r.Defs.Push("g", core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g",
		Signatures: []core.Signature{{}}}})
	if es.userMemberFn("g") {
		t.Error("a fn with no boru body (a Go-impl fn) is not a user fn")
	}
	r.Defs.Push("u", core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "u",
		Signatures: []core.Signature{{}, {Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}}}}})
	if !es.userMemberFn("u") {
		t.Error("a boru body on any overload makes a user fn")
	}
}
