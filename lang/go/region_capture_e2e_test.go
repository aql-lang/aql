package lang

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
)

// TestRegionCaptureFiresOnRealPrograms is the end-to-end pin for Stage 4's
// region capture. The unit tests in compiler/go drive tryRecordRegion and
// completeRegion directly over hand-built windows, which proves the functions
// but not the WIRING — that core's RegionRecorder seam is installed, fires
// from the real dispatch path, reaches the live EmitState, is CLAIMED by the
// dispatch that walked it, and rides out on the Program.
//
// It lives in lang because that is the only layer where all three exist at
// once: the interpreter that fires the seam, the compiler that seats on it,
// and a registry a program actually ran on.
//
// The subject is the finished table rather than the pending map, and that is
// the wiring change Phase B made: a capture is an OFFER, claimed at
// RecordCall and gone from the map afterwards, so an assertion on what is
// still pending would now be asserting that the join FAILED.
func TestRegionCaptureFiresOnRealPrograms(t *testing.T) {
	compile := func(t *testing.T, src string) *compiler.Program {
		t.Helper()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, _, _, cerr := b.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("compile %q: %v", src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q did not compile — the pin needs a Program to read", src)
		}
		return prog
	}

	t.Run("a forward-collecting dispatch is captured and claimed", func(t *testing.T) {
		prog := compile(t, `add 1 2`)
		if len(prog.Regions) == 0 {
			t.Fatal("`add 1 2` forward-collects, so it must produce a region descriptor")
		}
		var d *compiler.RegionDesc
		for i := range prog.Regions {
			if prog.Regions[i].Word == "add" {
				d = &prog.Regions[i]
				break
			}
		}
		if d == nil {
			t.Fatal("no descriptor for `add` — the (word, pos) join Phase B looks up by")
		}
		if d.Lead != compiler.LeadWord {
			t.Errorf("lead = %v, want LeadWord", d.Lead)
		}
		if len(d.Slots) != 2 {
			t.Fatalf("captured %d slots for `add 1 2`, want 2", len(d.Slots))
		}
		// Both operands were written forward, so the claim covers the span.
		if d.NFwd != 2 {
			t.Errorf("NFwd = %d, want 2 — both slots were the dispatch's operands", d.NFwd)
		}
		for i := 0; i < d.NFwd; i++ {
			if d.Slots[i].Source != compiler.SlotConst {
				t.Errorf("slot %d source = %v, want SlotConst", i, d.Slots[i].Source)
			}
		}
		if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
			t.Errorf("the emitted descriptor must validate against the program: %v", err)
		}
	})

	// A fn body's own dispatch is described too, and its word slots are the
	// case the model would get WRONG if it kept them live. Inside the body the
	// analysis binds each param into the def stack, so `a` and `b` resolve
	// during the pass — but the emitted body reads them from the FRAME, and at
	// run time the def stack holds no such binding. The completion takes the
	// operand's frame slot instead, which is the design's "a word slot that
	// resolves to a param lowers as SlotLocal rather than SlotWordRef".
	t.Run("a fn body's param slots describe the frame, not the def stack", func(t *testing.T) {
		prog := compile(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
		var d *compiler.RegionDesc
		for i := range prog.Regions {
			if prog.Regions[i].Word == "add" {
				d = &prog.Regions[i]
				break
			}
		}
		if d == nil {
			t.Fatal("the body's `add a b` must produce a descriptor")
		}
		if d.NFwd != 2 {
			t.Fatalf("NFwd = %d, want 2", d.NFwd)
		}
		for i, want := range []int{0, 1} {
			if d.Slots[i].Source != compiler.SlotLocal || d.Slots[i].Idx != want {
				t.Errorf("slot %d = %v/%d, want SlotLocal/%d — a param lives in the frame",
					i, d.Slots[i].Source, d.Slots[i].Idx, want)
			}
		}
		for i := range prog.Regions {
			if err := prog.Regions[i].Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
				t.Errorf("descriptor %d does not validate: %v", i, err)
			}
		}
	})

	// The seat is RecordCall's, and only RecordCall's. A USER fn call records
	// through RecordUserCall, and the poly, dyn-apply and dyn-method families
	// have their own entry points — none of them claims a capture yet, so
	// `f 1 2` above contributes no descriptor of its own. That is a stated
	// bound on what the table covers, not a silent one: a later seat widens
	// it, and this pin fails if one lands without updating the claim.
	t.Run("only RecordCall claims a capture today", func(t *testing.T) {
		prog := compile(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
		for i := range prog.Regions {
			if prog.Regions[i].Word == "f" {
				t.Fatalf("a user-fn call produced a descriptor — Phase B has gained a seat "+
					"beyond RecordCall; widen this pin and the census's stated bound (%v)",
					prog.Regions[i].Pos)
			}
		}
	})
}

// TestRegionTableIsInLoweringOrder pins where an index comes from. The table
// is appended in lowerCall, so a descriptor's index is its position in the
// LOWERED code — the same discipline Dispatches follows, and the reason
// neither needs a rollback of its own. An index assigned at record time would
// shift under a discarded loop-analysis round; one assigned here cannot,
// because a round that is not lowered contributes nothing.
//
// This matters before OpCollect exists, not after: the op's Arg will be that
// index, and an off-by-one there is a descriptor describing another dispatch.
func TestRegionTableIsInLoweringOrder(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := b.CompileCheck(`add 1 2  sub 4 3  mul 5 6`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	var got []string
	for i := range prog.Regions {
		got = append(got, prog.Regions[i].Word)
	}
	want := []string{"add", "sub", "mul"}
	if len(got) != len(want) {
		t.Fatalf("descriptors %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descriptors %v, want %v — the table is not in lowering order", got, want)
		}
	}
}
