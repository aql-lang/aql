package eng

import "testing"

// singleOutputCall gates which dyn-bound source promoteLateDynBind can seat as
// a lone frame local. The recursion-blind arms (a nil event from a non-recursed
// multi-value arm, a native evCall, a multi-output producer, a non-call event)
// are off the voxgig corpus path, so pin them directly.
func TestSingleOutputCall(t *testing.T) {
	if singleOutputCall(nil) {
		t.Error("a nil event is not a single-output call")
	}
	if !singleOutputCall(&emitEvent{kind: evCall, call: emitCall{nout: 1}}) {
		t.Error("a single-result native call IS a single-output call")
	}
	if singleOutputCall(&emitEvent{kind: evCall, call: emitCall{nout: 2}}) {
		t.Error("a multi-result native call is not single-output")
	}
	if !singleOutputCall(&emitEvent{kind: evCallUser, uc: emitUserCall{nout: 1}}) {
		t.Error("a single-result user call IS a single-output call")
	}
	if singleOutputCall(&emitEvent{kind: evCallUser, uc: emitUserCall{nout: 0}}) {
		t.Error("a zero-result user call is not single-output")
	}
	if singleOutputCall(&emitEvent{kind: evBranch}) {
		t.Error("a branch value is not a call")
	}
}
