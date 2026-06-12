package eng

import "testing"

// Synthetic-tape tests for the shell-variant elision mechanism: the
// scanned tail's effects must equal the parked markers' effects (state
// pops + undefs), the marker tokens must vanish, and the shell — the
// frame's open paren, ReturnCheck, and close paren — must survive.

func TestElideTailFrameRunsTeardownAndKeepsShell(t *testing.T) {
	f := newProbeFixture(t)

	// Frame entry state, as buildFnBodyHandler would have pushed it:
	// a baseline, an args list, and the param binding the tail undefs.
	f.r.PushFnBaseline(f.r.Defs.Snapshot())
	if err := f.r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	InstallDef(f.r, "n", NewInteger(1))
	// A body-local def ABOVE the snapshot the __DC marker carries —
	// eager DefCleanup must truncate it exactly like the parked one.
	snap := f.r.Defs.Snapshot()
	InstallDef(f.r, "loc", NewInteger(9))
	dc := NewDefCleanup(DefCleanupInfo{Snapshot: snap, Registry: f.r})

	// (ₘ 1 f __DC __pa undef n __RC )
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
		dc, NewWord("__pa"), f.und, NewWord("n"), f.rc, NewCloseParen(),
	}
	e := NewTop(f.r)
	e.tape = NewTape(tokens, 8)
	e.pointer = 2

	scan, ok := e.probeTailCall([]int{1}, 1)
	if !ok {
		t.Fatal("probe declined the canonical shape")
	}
	if err := e.elideTailFrame(scan); err != nil {
		t.Fatalf("elideTailFrame: %v", err)
	}

	// State: all three per-call effects happened.
	if f.r.TopFnBaseline() != nil {
		t.Error("FnBaseline not popped")
	}
	if _, ok, _ := f.r.Args.Top(); ok {
		t.Error("Args entry not popped")
	}
	if _, bound := f.r.Defs.Top("loc"); bound {
		t.Error("body-local def not truncated by the eager DefCleanup")
	}
	if _, bound := f.r.Defs.Top("n"); bound {
		t.Error("param binding not undefined by the eager undef tail")
	}

	// Tape: markers gone, shell intact — (ₘ 1 f __RC )
	isInt := func(v Value) bool { return v.Parent.Equal(TInteger) && IsConcrete(v) }
	want := []func(Value) bool{IsFrameOpen, isInt, IsWord, IsReturnCheck, IsCloseParen}
	if e.tape.Len() != len(want) {
		t.Fatalf("tape has %d tokens after elision, want %d", e.tape.Len(), len(want))
	}
	for i, pred := range want {
		if !pred(e.tape.At(i)) {
			t.Errorf("tape[%d] = %v, wrong token class", i, e.tape.At(i))
		}
	}
	if e.pointer != 2 {
		t.Errorf("pointer moved to %d; ahead-of-pointer edits must not move it", e.pointer)
	}
}

func TestElideTailFrameNoReturnCheck(t *testing.T) {
	f := newProbeFixture(t)
	f.r.PushFnBaseline(f.r.Defs.Snapshot())
	if err := f.r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	dc := NewDefCleanup(DefCleanupInfo{Snapshot: f.r.Defs.Snapshot(), Registry: f.r})

	// (ₘ f __DC __pa )  — no declared returns, no params.
	tokens := []Value{
		NewFrameOpen(f.meta), NewWord("f"), dc, NewWord("__pa"), NewCloseParen(),
	}
	e := NewTop(f.r)
	e.tape = NewTape(tokens, 8)
	e.pointer = 1

	scan, ok := e.probeTailCall(nil, 0)
	if !ok {
		t.Fatal("probe declined")
	}
	if err := e.elideTailFrame(scan); err != nil {
		t.Fatal(err)
	}
	// Shell without RC: (ₘ f )
	if e.tape.Len() != 3 || !IsFrameOpen(e.tape.At(0)) || !IsWord(e.tape.At(1)) || !IsCloseParen(e.tape.At(2)) {
		t.Fatalf("tape after elision = %d tokens, want (ₘ f )", e.tape.Len())
	}
}

func TestTCOEligibleGates(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)
	sig := &Signature{FnFrame: f.meta}
	base := frameTailScan{Meta: f.meta}
	muts := f.r.Defs.Mutations()

	if !e.tcoEligible(base, sig, muts) {
		t.Fatal("baseline scan must be eligible")
	}
	// Kill switch.
	f.r.TCO.Disable = true
	if e.tcoEligible(base, sig, muts) {
		t.Error("Disable must gate elision off")
	}
	f.r.TCO.Disable = false
	// Not self-recursion: a different overload's frame.
	other := frameTailScan{Meta: &FnFrameMeta{Name: "g"}}
	if e.tcoEligible(other, sig, muts) {
		t.Error("a different overload's frame must decline (mutual recursion nests at this stage)")
	}
	// Generic fn.
	genMeta := &FnFrameMeta{Name: "h", HasGen: true}
	if e.tcoEligible(frameTailScan{Meta: genMeta}, &Signature{FnFrame: genMeta}, muts) {
		t.Error("generic fns must decline")
	}
	// Binding mutation during arg auto-eval.
	if e.tcoEligible(base, sig, muts-1) {
		t.Error("binding mutations during auto-eval must decline")
	}
	// Capitalised / builtin teardown names.
	capScan := base
	capScan.UndefNames = []string{"Foo"}
	if e.tcoEligible(capScan, sig, muts) {
		t.Error("capitalised teardown names must decline (type-retire path)")
	}
}
