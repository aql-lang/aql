package core

import "testing"

// skipRecorder is a minimal Recorder + RecorderSkipper for the credit test.
type skipRecorder struct{ skipped int }

func (s *skipRecorder) OnPushLit(Value)         {}
func (s *skipRecorder) OnCall(string, int, int) {}
func (s *skipRecorder) Skip(n int)              { s.skipped += n }

// TestCreditParenSurvivorSkipsOffMainLoop covers creditParenSurvivorSkips'
// OFF-MAIN-LOOP span start (reStepped false), which the BROAD park (NUR073
// clause 3) made reachable for USER parens: a parked survivor is still
// COLLECTED — the collection hook fires OnPushLit for it at match
// completion — so its in-paren emission is owed the same credit, and the
// span must start at openIdx rather than the post-park pointer. core/go is
// gated by its own suite (core/go/CLAUDE.md), so the arm needs a core-side
// test even though lang/go exercises it end to end.
func TestCreditParenSurvivorSkipsOffMainLoop(t *testing.T) {
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	e := NewTop(reg)
	rec := &skipRecorder{}
	e.SetRecorder(rec)
	e.Tape = NewTape([]Value{NewInteger(1), NewInteger(2)}, 4)
	e.Pointer = 1

	// reStepped=false: the span starts at openIdx (0), so the survivor
	// BEHIND the parked pointer is credited too.
	e.creditParenSurvivorSkips(0, 2, false)
	off := rec.skipped
	if off == 0 {
		t.Fatal("an off-main-loop collapse must credit its survivors")
	}

	// reStepped=true: the span starts at the pointer, so the value the main
	// loop already stepped past is NOT credited again.
	rec.skipped = 0
	e.creditParenSurvivorSkips(0, 2, true)
	if rec.skipped >= off {
		t.Errorf("main-loop collapse credited %d, want fewer than the off-loop %d", rec.skipped, off)
	}
}
