package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestW8DispatchRematchNoneLiteralWindow — a `none` word in the failed
// window resolves to the None value exactly as the match's forward walk
// resolved it (the known-literal arm); the record still declines here
// because the written tuple is EMPTY (the forward walk stops at the word
// and the stack prefix is bare) — no render bound exists for it.
func TestW8DispatchRematchNoneLiteralWindow(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(core.NativeFunc{Name: "w8rn", Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger, core.TNone}, BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewWord("w8rn"), core.NewWord("none"), core.NewCarrier(core.TInteger)}, core.StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8rn")
	if fn == nil {
		t.Fatal("w8rn not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(core.WordInfo{Name: "w8rn", ArgCount: -1}, fn, core.SrcPos{}) {
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
	r.RegisterNativeFunc(core.NativeFunc{Name: "w8dt", Signatures: []core.Signature{{
		Args: []*core.Type{core.TList}, BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	reach := core.NewReachFromKeys(core.NewWord("m"), []core.Value{core.NewString("a")})
	e.Tape = core.NewTape([]core.Value{core.NewWord("w8dt"), reach}, core.StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8dt")
	if fn == nil {
		t.Fatal("w8dt not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(core.WordInfo{Name: "w8dt", ArgCount: -1}, fn, core.SrcPos{}) {
		t.Error("a raw Reach window token must decline the trap/rematch record")
	}
}

// TestW8DispatchRematchFnShapeDeclines — the fn-shape typed-binding hint is
// TAPE state the runtime rebuild has no access to: a carrier no-match inside
// a def whose constraint is a function-shape type must decline the rematch
// record so the interpreter's suggestion-bearing error stays canonical.
func TestW8DispatchRematchFnShapeDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", core.NewCarrier(core.TFnUndef))
	r.RegisterNativeFunc(core.NativeFunc{Name: "w8rf", Signatures: []core.Signature{{
		Args: []*core.Type{core.TString}, BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return nil, nil
		}),
	}}})
	sig := &core.Signature{Params: []core.FnParam{{Type: core.TMap}, {Type: core.TAny}}, BarrierPos: -1}
	m := core.NewOrderedMap()
	m.Set("f", core.NewWord("MyShape"))
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	// The def Forward sits below the failing word (both back-walks skip it);
	// its typed-name map rides at FuncIndex-CollectedArgs — above the
	// pointer, so the stack window stays empty and the carrier forms the
	// forward window; the trailing word bounds the written walk.
	e.Tape = core.NewTape([]core.Value{
		core.NewForward(core.ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 5}),
		core.NewWord("w8rf"),
		core.NewCarrier(core.TInteger),
		core.NewWord("zz-stop"),
		core.NewMap(m),
	}, core.StackHeadroom)
	e.Pointer = 1
	if !e.IsFnShapeTypedBindingContext() {
		t.Fatal("setup: expected the fn-shape typed-binding context")
	}
	if e.TryRecordUnmatchedDispatchTrap(core.WordInfo{Name: "w8rf", ArgCount: -1}, r.Lookup("w8rf"), core.SrcPos{}) {
		t.Error("the fn-shape context must decline the rematch record")
	}
}
