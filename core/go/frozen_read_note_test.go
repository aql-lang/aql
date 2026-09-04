package core

import "testing"

// The freeze discipline's two CORE-side note sites — stepWord's type-literal
// arm and stepWordVal's `/v` arm — are unreachable from core's own suite by
// ordinary means: both sit behind `analysisActive()`, and core runs its
// programs with check mode off. That is the same `cover-gate-core` inversion
// the bind-ledger notes hit, and the same answer: drive the arms directly.
//
// They are not merely coverage. Each site is a MISCOMPILE the tree carried
// until it existed, and the `/v` pair is the one review caught: `T/v` and
// `k/v` resolve through stepWordVal, which reaches neither of stepWord's
// substitution branches, so both bakes escaped the latch entirely.

// frozenRecorder captures NoteFrozenRead. It embeds the inactive recorder so
// every other method stays a no-op — only this one arm is under test.
type frozenRecorder struct {
	inactiveEmit
	got map[string]FrozenBake
}

func (f *frozenRecorder) NoteFrozenRead(name string, bake FrozenBake) {
	if f.got == nil {
		f.got = map[string]FrozenBake{}
	}
	f.got[name] = bake
}

// frozenReadEngine arms a compile pass with a capturing recorder and returns
// the engine plus the recorder.
func frozenReadEngine(t *testing.T) (*Engine, *frozenRecorder) {
	t.Helper()
	r := compileCheckRegistry(t)
	rec := &frozenRecorder{}
	r.Check.Emit = rec
	return NewTop(r), rec
}

// A module-scope TYPE read inside an analysis bakes its identity as an
// OpPushType, so the ordinary spelling notes it. Without this the tree
// answered `true true` where the interpreter answers `true false`.
func TestStepWordTypeReadNotesFrozenBake(t *testing.T) {
	e, rec := frozenReadEngine(t)
	e.Registry.Defs.PushType("T", TInteger, NewTypeLiteral(TInteger))

	e.Tape = NewTape([]Value{NewWord("T")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord errored: %v", err)
	}
	if got, ok := rec.got["T"]; !ok || got != FrozenBakeType {
		t.Errorf("a module-scope type read must note a TYPE bake; got %v (present=%v)", got, ok)
	}
}

// The `/v` spelling of the same read resolves through stepWordVal, which
// reaches neither substitution branch. Both bakes must be noted there too, or
// `T/v` and `k/v` walk straight past the latch — which is exactly what they
// did until 2026-09-04.
func TestStepWordValNotesBothBakes(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		e, rec := frozenReadEngine(t)
		e.Registry.Defs.PushType("T", TInteger, NewTypeLiteral(TInteger))

		e.Tape = NewTape([]Value{NewWord("T")}, StackHeadroom)
		if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "T", ArgCount: -1, ForceVal: true}); err != nil {
			t.Fatalf("/v step errored: %v", err)
		}
		if got, ok := rec.got["T"]; !ok || got != FrozenBakeType {
			t.Errorf("`T/v` must note a TYPE bake; got %v (present=%v)", got, ok)
		}
	})

	t.Run("value", func(t *testing.T) {
		e, rec := frozenReadEngine(t)
		e.Registry.Defs.Push("k", NewInteger(5))

		e.Tape = NewTape([]Value{NewWord("k")}, StackHeadroom)
		if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "k", ArgCount: -1, ForceVal: true}); err != nil {
			t.Fatalf("/v step errored: %v", err)
		}
		if got, ok := rec.got["k"]; !ok || got != FrozenBakeValue {
			t.Errorf("`k/v` must note a VALUE bake; got %v (present=%v)", got, ok)
		}
	})
}

// The negatives. A read of a name bound INSIDE an enclosing fn is a frame
// local, not a module binding, so nothing may be noted — the latch's whole
// premise is that the name still resolves where the unit runs.
func TestStepWordValFrameLocalReadNotesNothing(t *testing.T) {
	e, rec := frozenReadEngine(t)
	e.Registry.Defs.Push("k", NewInteger(5))
	// Push a fn baseline BELOW the binding's depth: ModuleScopeBinding is
	// `Depth(name) <= baseline[name]`, so a binding deeper than the baseline
	// reads as fn-local.
	e.Registry.FnBaselines = append(e.Registry.FnBaselines, map[string]int{"k": 0})

	e.Tape = NewTape([]Value{NewWord("k")}, StackHeadroom)
	if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "k", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("/v step errored: %v", err)
	}
	if got, ok := rec.got["k"]; ok {
		t.Errorf("a fn-local read must note nothing; got %v", got)
	}
}
