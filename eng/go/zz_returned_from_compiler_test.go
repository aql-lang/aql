package eng

import (
	"testing"
)

// TestW8DispatchRematchNoneLiteralWindow — a `none` word in the failed
// window resolves to the None value exactly as the match's forward walk
// resolved it (the known-literal arm); the record still declines here
// because the written tuple is EMPTY (the forward walk stops at the word
// and the stack prefix is bare) — no render bound exists for it.
func TestW8DispatchRematchNoneLiteralWindow(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8rn", Signatures: []Signature{{
		Args: []*Type{TInteger, TNone}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("w8rn"), NewWord("none"), NewCarrier(TInteger)}, StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8rn")
	if fn == nil {
		t.Fatal("w8rn not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8rn", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("the word-narrowed written tuple must decline the rematch record")
	}
}

// TestW8DispatchTrapDeferredTokenDeclines — a RAW deferred-expression token
// in the failed window (a Reach here) EXPANDS at dispatch/step time, so
// neither a serialized trap nor a runtime rematch models what the runtime
// match examines (flex.tsv L88/L95): the definiteness screen declines. The
// graduated word-splice trap removed the screen's only corpus reach, so this
// pins the arm directly. (A PARKED __SP splice marker deliberately no longer
// declines — see TestUnmatchedDispatchTrapSpliceGraduated in lang/go.)
func TestW8DispatchTrapDeferredTokenDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8dt", Signatures: []Signature{{
		Args: []*Type{TList}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	reach := NewReachFromKeys(NewWord("m"), []Value{NewString("a")})
	e.Tape = NewTape([]Value{NewWord("w8dt"), reach}, StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8dt")
	if fn == nil {
		t.Fatal("w8dt not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8dt", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("a raw Reach window token must decline the trap/rematch record")
	}
}

// TestW8DispatchRematchFnShapeDeclines — the fn-shape typed-binding hint is
// TAPE state the runtime rebuild has no access to: a carrier no-match inside
// a def whose constraint is a function-shape type must decline the rematch
// record so the interpreter's suggestion-bearing error stays canonical.
func TestW8DispatchRematchFnShapeDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	r.RegisterNativeFunc(NativeFunc{Name: "w8rf", Signatures: []Signature{{
		Args: []*Type{TString}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	// The def Forward sits below the failing word (both back-walks skip it);
	// its typed-name map rides at FuncIndex-CollectedArgs — above the
	// pointer, so the stack window stays empty and the carrier forms the
	// forward window; the trailing word bounds the written walk.
	e.Tape = NewTape([]Value{
		NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 5}),
		NewWord("w8rf"),
		NewCarrier(TInteger),
		NewWord("zz-stop"),
		NewMap(m),
	}, StackHeadroom)
	e.Pointer = 1
	if !e.IsFnShapeTypedBindingContext() {
		t.Fatal("setup: expected the fn-shape typed-binding context")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8rf", ArgCount: -1}, r.Lookup("w8rf"), SrcPos{}) {
		t.Error("the fn-shape context must decline the rematch record")
	}
}
