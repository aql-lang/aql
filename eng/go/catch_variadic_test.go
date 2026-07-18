package eng

import "testing"

// The catch-variadic latch (L-DO part 1): set by the fallible do body's
// ReturnsFn, consumed exactly once by the dispatch's record path, keyed to
// the CompileFallbackBody sig so it can never mark an unrelated word's
// event.
func TestCatchVariadicLatch(t *testing.T) {
	es := NewEmitState()
	catchSig := &Signature{CompileEffect: CompileFallbackBody}
	plainSig := &Signature{}

	if es.catchVariadicFor(catchSig) {
		t.Fatal("unlatched: must be false")
	}
	es.SetCatchVariadic(true)
	if es.catchVariadicFor(plainSig) {
		t.Fatal("a non-catch sig must not consume the latch")
	}
	if es.catchVariadicFor(nil) {
		t.Fatal("a nil sig must not consume the latch")
	}
	if !es.catchVariadicFor(catchSig) {
		t.Fatal("the catch sig must consume the latch")
	}
	if es.catchVariadicFor(catchSig) {
		t.Fatal("consumed: a second read must be false")
	}
	es.SetCatchVariadic(true)
	es.SetCatchVariadic(false)
	if es.catchVariadicFor(catchSig) {
		t.Fatal("an explicit clear must stick")
	}

	// Nil-receiver discipline (the recorder methods are nil-safe).
	var nilES *EmitState
	nilES.SetCatchVariadic(true)
	if nilES.catchVariadicFor(catchSig) {
		t.Fatal("nil receiver must be inert")
	}
	// The inactive recorder accepts the call as a no-op.
	theInactiveEmit.SetCatchVariadic(true)
	if theInactiveEmit.RecordInterpXml(XmlTmpl{}, nil, Value{}, SrcPos{}) {
		t.Fatal("the inactive recorder must decline RecordInterpXml")
	}
}

// The GENERIC RecordCall fall-through consumes the latch too (the closure
// probe can decline a fallible do shape; its dispatch then records through
// RecordCall and must still be variadic — the caught path nets 1 where the
// static seat expects N).
func TestCatchVariadicGenericRecordCall(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{CompileEffect: CompileFallbackBody}
	a, b := NewInteger(1), NewInteger(2)
	outA, outB := NewInteger(10), NewInteger(20)
	outA.ID, outB.ID = "outA", "outB"

	es.SetCatchVariadic(true)
	es.RecordCall("do", sig, []Value{a, b}, []Value{outA, outB}, SrcPos{}, false, false)
	if es.catchVariadicPending {
		t.Fatal("the latch must be consumed by the generic path")
	}
	marked := false
	for _, f := range es.eventInfo {
		if f.variadicResult {
			marked = true
		}
	}
	if !marked {
		t.Fatal("the generic path must mark the latched dispatch variadic")
	}
}
