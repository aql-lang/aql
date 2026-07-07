package eng

import "testing"

// W9 method_shape.go coverage: the pure evaluation-fixed-window classifier,
// NoteMethodShape's inactive gate, shapedMethodApplyWindow's early declines,
// tryRecordMethodApply's word-mismatch / record-fail arms, and
// tryDynamicFnValueDispatch's non-inert window decline.

func TestW9EvalFixedWindowToken(t *testing.T) {
	// Non-inert modes → false (carrier / dynamic / undefined).
	if evalFixedWindowToken(NewCarrier(TInteger)) {
		t.Error("carrier is not evaluation-fixed")
	}
	if evalFixedWindowToken(NewDynamicCarrier(TInteger)) {
		t.Error("dynamic carrier is not evaluation-fixed")
	}
	und := NewInteger(1)
	und.Undefined = true
	if evalFixedWindowToken(und) {
		t.Error("undefined value is not evaluation-fixed")
	}

	// A list of all-inert elements → true; a list with a word → false.
	if !evalFixedWindowToken(NewList([]Value{NewInteger(1), NewString("x")})) {
		t.Error("a list of inert scalars is evaluation-fixed")
	}
	if evalFixedWindowToken(NewList([]Value{NewWord("w")})) {
		t.Error("a list containing a word is not evaluation-fixed")
	}

	// A nil-backed map payload → false.
	if evalFixedWindowToken(NewMap(nil)) {
		t.Error("a nil-backed map is not evaluation-fixed")
	}
	// A map carrying computed keys (Meta "ck") → false.
	ck := NewOrderedMap()
	ck.Set("k", NewInteger(1))
	ck.Meta = map[string]any{"ck": map[string]bool{"k": true}}
	if evalFixedWindowToken(NewMap(ck)) {
		t.Error("a map with computed keys is not evaluation-fixed")
	}
	// A map with a non-inert value → false.
	bad := NewOrderedMap()
	bad.Set("k", NewWord("w"))
	if evalFixedWindowToken(NewMap(bad)) {
		t.Error("a map with a word value is not evaluation-fixed")
	}
	// A fully inert map → true.
	good := NewOrderedMap()
	good.Set("k", NewInteger(1))
	if !evalFixedWindowToken(NewMap(good)) {
		t.Error("an inert map is evaluation-fixed")
	}
}

func TestW9NoteMethodShapeInactive(t *testing.T) {
	r := newTestRegistry(t)
	// The check is inactive on a fresh registry, so the annotation is a no-op.
	r.Check.NoteMethodShape(NewCarrier(TAny), NewFunction(FnDefInfo{Name: "m", Registry: r}))
	if r.Check.MethodShapes != nil {
		t.Error("an inactive check must not record a method shape")
	}
}

func TestW9ShapedMethodApplyWindowRejects(t *testing.T) {
	r := newTestRegistry(t)
	e := &Engine{registry: r}
	e.tape = NewTape([]Value{NewCarrier(TAny), NewInteger(1)}, stackHeadroom)
	// Member payload is not an FnDefInfo → decline.
	if _, _, ok := e.shapedMethodApplyWindow(0, NewInteger(1)); ok {
		t.Error("a non-FnDef member must decline")
	}
	// FnDefInfo whose owning registry does not resolve the name → decline.
	member := NewFunction(FnDefInfo{Registry: r, Name: "w9missing"})
	if _, _, ok := e.shapedMethodApplyWindow(0, member); ok {
		t.Error("an unresolvable member fn must decline")
	}
}

func TestW9TryRecordMethodApplyWordMismatch(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: NewCarrier(TAny), Word: "owner"}
	if tryRecordMethodApply(r, "other", nil, nil, SrcPos{}) {
		t.Error("a word mismatch must leave the pending for its owner")
	}
	if r.Check.PendingMethodApply == nil {
		t.Error("the pending must survive a word mismatch")
	}
}

func TestW9TryRecordMethodApplyRecordFails(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	r.Check.Emit = es
	// Origin carrier with no producing event → RecordDynMethod fails and the
	// program is marked uncompilable.
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: NewCarrier(TAny), Word: "m"}
	if !tryRecordMethodApply(r, "m", nil, nil, SrcPos{}) {
		t.Error("a matching pending must be consumed")
	}
	if es.Compilable {
		t.Error("a failed RecordDynMethod must mark the program uncompilable")
	}
}

func TestW9TryDynamicFnValueDispatchNonInertWindow(t *testing.T) {
	r := newTestRegistry(t)
	e := &Engine{registry: r}
	// A dynamic Function-bearing carrier followed by a WORD (non-inert): the
	// window admission declines the model.
	e.tape = NewTape([]Value{NewDynamicCarrier(TFunction), NewWord("w")}, stackHeadroom)
	if e.tryDynamicFnValueDispatch(0) {
		t.Error("a word in the forward window must decline the dynamic-fn model")
	}
}
