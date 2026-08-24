package core

import "testing"

// fnReturnPark decides whether a collapsing paren left a single unquoted
// Function value, in which case stepCloseParen's rewind steps PAST the value
// instead of re-stepping it into a call — parens place values, they never
// re-step them, reference and inline literal alike; only a reach-lowered
// group is excluded, because its re-step IS the dispatch (NUR073's BROAD
// verdict, clause 3; design/FN-VALUE-OPEN-WORK.0.md §2).
//
// The end-to-end behaviour is pinned above this module — `lang/spec/ref.tsv`
// §8 and `lang/go/fn_return_park_test.go`, on both engines. This test exists
// for the arm ITSELF: core/go is gated by its OWN suite at a 100% floor
// (`make cover-gate-core`, core/go/CLAUDE.md), so a decision function reached
// only from above reads as uncovered there however well the merged profile
// scores it. Same failure mode, and same remedy, as commit cdc7a11's
// canon_stage5_test.go.
//
// Each row also carries a reason, because every `return 0` here is a
// deliberate negative: the whole risk of this change is a park that fires one
// case too wide and swallows a value the main loop still owes work on.
func TestFnReturnPark(t *testing.T) {
	e := &Engine{}

	fnv := NewFunction(FnDefInfo{Name: "parked"})

	quoted := NewFunction(FnDefInfo{Name: "quoted"})
	quoted.Quoted = true

	// A Function value REPARENTED to a refined function type. stepLiteral's
	// dispatch guard requires Parent == TFunction, so this one would be
	// PUSHED, not called — parking it would drop its OnPushLit.
	refined := ReparentValue(NewFunction(FnDefInfo{Name: "refined"}), TAny)

	// A named fn collapsed out of a reach group (tagReachCollapsedFn):
	// dot access to a function is a call, so the park must decline it.
	reachTagged := NewFunction(FnDefInfo{Name: "m.f"})
	reachTagged.ReachGroup = true

	cases := []struct {
		name          string
		tape          []Value
		closeIdx      int
		notReachGroup bool
		want          int
		why           string
	}{
		{
			name: "fn frame returning a Function parks", tape: []Value{fnv},
			closeIdx: 2, notReachGroup: true, want: 1,
			why: "the frame is delivering a return, which is not a fresh use of the value",
		},
		{
			name: "user paren parks too (BROAD)", tape: []Value{fnv},
			closeIdx: 2, notReachGroup: true, want: 1,
			why: "a paren places its collapsed value — grouping is not a fresh use (NUR073 clause 3)",
		},
		{
			name: "reach group re-steps", tape: []Value{fnv},
			closeIdx: 2, notReachGroup: false, want: 0,
			why: "a reach-lowered group's re-step IS the dispatch (dot access must still call)",
		},
		{
			name: "reach-TAGGED value re-steps", tape: []Value{reachTagged},
			closeIdx: 2, notReachGroup: true, want: 0,
			why: "a dot-accessed named fn in a user paren is a CALL (NUR038) — the value-borne twin of the group exclusion",
		},
		{
			name: "quoted Function is data already", tape: []Value{quoted},
			closeIdx: 2, notReachGroup: true, want: 0,
			why: "a Quoted value would not have dispatched on re-step, so there is nothing to suppress",
		},
		{
			name: "reparented Function is pushed, not called", tape: []Value{refined},
			closeIdx: 2, notReachGroup: true, want: 0,
			why: "stepLiteral requires Parent == TFunction to dispatch; parking this loses its OnPushLit",
		},
		{
			name: "non-Function survivor is untouched", tape: []Value{NewInteger(42)},
			closeIdx: 2, notReachGroup: true, want: 0,
			why: "the park applies only to a Function residual — every ordinary call returns through here",
		},
		{
			name: "more than one survivor is untouched", tape: []Value{fnv, NewInteger(1)},
			closeIdx: 3, notReachGroup: true, want: 0,
			why: "the park is the exactly-one-survivor case; stepping past a multi-value collapse strands the rest",
		},
		{
			name: "empty collapse is untouched", tape: []Value{},
			closeIdx: 2, notReachGroup: true, want: 0,
			why: "nothing survived, so idx is past the tape end — the bounds guard, not a value decision",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e.Tape = NewTape(tc.tape, 4)
			if got := e.fnReturnPark(0, tc.closeIdx, tc.notReachGroup); got != tc.want {
				t.Errorf("fnReturnPark = %d, want %d — %s", got, tc.want, tc.why)
			}
		})
	}
}

// IsFrameOpen tells a fn frame's collapse from a user paren's for the frame
// machinery (teardown, probes); since the BROAD park it no longer gates
// fnReturnPark, but the payload must stay unforgeable from source text. Pin
// that here: a parser-produced open paren must not read as a frame, and both
// machine-generated constructors must.
func TestFrameOpenIsMachineGeneratedOnly(t *testing.T) {
	if IsFrameOpen(NewOpenParen()) {
		t.Error("a plain source-written open paren reads as a fn frame — frame teardown would fire on user parens")
	}
	meta := &FnFrameMeta{}
	if !IsFrameOpen(NewFrameOpen(meta)) {
		t.Error("NewFrameOpen does not read as a fn frame — frame machinery would never engage")
	}
	if !IsFrameOpen(NewFrameOpenSpan(meta, 2)) {
		t.Error("NewFrameOpenSpan does not read as a fn frame — frames with a resolved-arg span would not engage")
	}
}
