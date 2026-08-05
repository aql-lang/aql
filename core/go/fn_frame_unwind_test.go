package core

import "testing"

// Direct kernel pins for unwindLiveFrames / unwindFrameTail (fn_frame.go):
// a flow-control rewrite discarding a region must replay the canonical
// cleanup tail of every frame still OPEN in it — __DC truncation, the
// __pa Args/FnBaseline pop, and the force-forward undef pairs — and must
// NOT touch completed frames, user-written undef tokens, or nested
// content below the frame's own paren depth. The language-level twins
// (break/continue through live frames) live in
// lang/go/native/args_inert_test.go; these tests pin the tape mechanics
// over hand-built frames, kernel-only.

// unwindFixture builds an engine whose tape holds one spliced frame in
// the canonical shape, with the frame's per-call state (args list,
// baseline, one param binding, one body-local def) LIVE on the registry,
// and the pointer parked inside the frame body.
func unwindFixture(t *testing.T) (*Engine, *Registry) {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)

	// Simulate frame entry exactly as buildFnBodyHandler performs it.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(7)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwx", NewInteger(7))
	snap := r.Defs.Snapshot() // after param install: __DC truncates body-locals only
	r.Defs.Push("uwlocal", NewInteger(1))

	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 1),
		NewInteger(7), // the resolved unnamed arg
		NewWord("uwbody"),
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r,
		Snapshot: snap,
		Names:    []string{"uwx"},
		Returns:  []*Type{TInteger},
		FuncName: "uw",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 2 // parked at the body word: frame open stepped, tail not reached
	return e, r
}

func TestUnwindLiveFrameReplaysCleanupTail(t *testing.T) {
	e, r := unwindFixture(t)
	e.unwindLiveFrames(0, e.Tape.Len())

	if _, ok, _ := r.Args.Top(); ok {
		t.Error("per-call args list not popped by the unwind")
	}
	if r.Defs.Has("uwx") {
		t.Error("param binding not undef'd by the unwind")
	}
	if r.Defs.Has("uwlocal") {
		t.Error("body-local def not truncated by the unwind (__DC)")
	}
}

func TestUnwindSkipsCompletedFrame(t *testing.T) {
	// A frame whose close paren sits BELOW the pointer already ran its
	// tail; the unwind must not double-pop. Sentinel args entry stands in
	// for the enclosing call's state.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	if err := r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewInteger(9),
		NewCloseParen(),
		NewInteger(9), // residual past the completed frame
	}
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 3 // past the close: the frame is NOT live
	e.unwindLiveFrames(0, e.Tape.Len())
	if _, ok, _ := r.Args.Top(); !ok {
		t.Error("unwind popped state belonging to a COMPLETED frame")
	}
}

func TestUnwindNestedFramesPopLIFO(t *testing.T) {
	// Two live frames, inner inside outer: both tails replay, and the
	// depth gate keeps each tail owned by its own frame (the inner
	// frame's markers are depth 2 relative to the outer open).
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)

	// Outer frame entry.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwout", NewInteger(1))
	outerSnap := r.Defs.Snapshot()
	// Inner frame entry.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(2)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwin", NewInteger(2))
	innerSnap := r.Defs.Snapshot()

	inner := []Value{NewFrameOpenSpan(fnValueFrameMeta, 0), NewWord("uwibody")}
	inner = AppendFrameTail(inner, FrameTailSpec{
		Registry: r, Snapshot: innerSnap, Names: []string{"uwin"}, FuncName: "uwi",
	})
	inner = append(inner, NewCloseParen())

	tokens := []Value{NewFrameOpenSpan(fnValueFrameMeta, 0)}
	tokens = append(tokens, inner...)
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: outerSnap, Names: []string{"uwout"}, FuncName: "uwo",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 2 // inside the INNER frame's body: both frames live

	e.unwindLiveFrames(0, e.Tape.Len())
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("nested unwind left a per-call args entry")
	}
	if r.Defs.Has("uwin") || r.Defs.Has("uwout") {
		t.Error("nested unwind left a param binding installed")
	}
}

func TestUnwindIgnoresUserUndefTokens(t *testing.T) {
	// A user-written `undef name` in the (skipped) body region carries no
	// ForceForward flag; the unwind must not execute it — only the
	// machine-generated tail pairs tear bindings down.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("uwkeep", NewInteger(3))
	snap := r.Defs.Snapshot()

	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewWord("undef"), NewWord("uwkeep"), // USER tokens, not the tail's pairs
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: snap, Names: nil, FuncName: "uwu",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 1

	e.unwindLiveFrames(0, e.Tape.Len())
	if !r.Defs.Has("uwkeep") {
		t.Error("unwind executed a USER undef token from the skipped body")
	}
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("frame tail __pa not replayed")
	}
}

func TestUnwindClampsRegionAndHandlesOpenEndedFrame(t *testing.T) {
	// A frame whose close paren lies outside the discarded region (the
	// loop rewrite cuts mid-frame) is still live and still unwinds; the
	// region end is clamped to the tape.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewWord("uwbody"),
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: r.Defs.Snapshot(), Names: nil, FuncName: "uwc",
	})
	// Deliberately NO close paren: the frame extends past the region.
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 1

	e.unwindLiveFrames(0, e.Tape.Len()+10) // over-long region clamps
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("open-ended live frame's __pa not replayed")
	}
}
