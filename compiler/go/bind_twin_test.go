package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordBindTwin's contract (§6.5's inert-emission stage): unconditional
// append — no Active()/Suspend gate, because NoteBindTransition already
// applied every ledger suppression and a second filter would be a second
// source of truth — nil-receiver safe like every EmitState method, and the
// finalized Program carries a COPY, so a later append on the same EmitState
// cannot mutate a delivered Program.
func TestRecordBindTwinAppendsUnconditionally(t *testing.T) {
	var nilES *EmitState
	nilES.RecordBindTwin(core.BindTransition{Name: "x"}, core.DefEntry{}) // must not panic

	es := NewEmitState()
	resume := es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1},
		core.DefEntry{Body: core.NewInteger(7)})
	resume()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindUndef, Name: "a", Depth: 0}, core.DefEntry{})
	if len(es.bindTwins) != 2 || len(es.bindTwinEntries) != 2 {
		t.Fatalf("twin table = %d/%d entries, want 2/2 (suspension must not filter)",
			len(es.bindTwins), len(es.bindTwinEntries))
	}

	prog, _, ok := es.Finalize(nil)
	if !ok {
		t.Fatal("an empty compilable state must finalize")
	}
	if len(prog.BindTwins) != 2 || prog.BindTwins[0].Name != "a" ||
		prog.BindTwins[0].Kind != core.BindDef || prog.BindTwins[1].Kind != core.BindUndef {
		t.Fatalf("finalized twin table = %v, want the recorded entries verbatim", prog.BindTwins)
	}
	if len(prog.BindTwinEntries) != 2 || !core.IsConcrete(prog.BindTwinEntries[0].Body) ||
		core.IsConcrete(prog.BindTwinEntries[1].Body) {
		t.Fatalf("finalized twin entries = %+v, want the captured push entry and the undef zero", prog.BindTwinEntries)
	}
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "late", Depth: 1}, core.DefEntry{})
	if len(prog.BindTwins) != 2 || len(prog.BindTwinEntries) != 2 {
		t.Fatal("the Program's tables are copies; a later append must not reach them")
	}
}

// countBindTwinOps mirrors Finalize's placement scan for the assertions below.
func countBindTwinOps(p *Program) int {
	placed := 0
	for _, in := range p.Code {
		if in.Op == OpBindTwin {
			placed++
		}
	}
	for i := range p.Fns {
		for _, in := range p.Fns[i].Code {
			if in.Op == OpBindTwin {
				placed++
			}
		}
	}
	return placed
}

// The regime's Finalize contract (§6.5's flip, staged behind BORU_TWIN_REGIME):
// the flag is stamped onto the Program at the recorder's own reading, and a
// regime program refuses unless EVERY twin-table entry has a placed op — a
// table-only twin (suspended recording) is a transition the rollback would
// lose. Outside the regime the same table-only twin stays a legal inert
// residue, which is the invariant the ordered-subset gate already pins.
func TestFinalizeTwinRegimeStampAndPlacementGate(t *testing.T) {
	// Default: the flag off, the stamp false, table-only twins tolerated.
	es := NewEmitState()
	resume := es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1},
		core.DefEntry{Body: core.NewInteger(7)})
	resume()
	prog, _, ok := es.Finalize(nil)
	if !ok || prog.TwinRegime {
		t.Fatalf("flag-off finalize = %v, TwinRegime = %v; want ok and unstamped", ok, prog != nil && prog.TwinRegime)
	}

	t.Setenv("BORU_TWIN_REGIME", "1")

	// Fully-placed twins finalize, stamped.
	es = NewEmitState()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1},
		core.DefEntry{Body: core.NewInteger(7)})
	prog, reason, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("fully-placed regime finalize refused: %s", reason)
	}
	if !prog.TwinRegime {
		t.Fatal("the regime recorder must stamp Program.TwinRegime")
	}
	if got := countBindTwinOps(prog); got != len(prog.BindTwins) || got != 1 {
		t.Fatalf("placed %d twin ops for a %d-entry table, want full placement", got, len(prog.BindTwins))
	}

	// A table-only twin refuses under the regime.
	es = NewEmitState()
	resume = es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "b", Depth: 1},
		core.DefEntry{Body: core.NewInteger(9)})
	resume()
	if _, reason, ok := es.Finalize(nil); ok || reason == "" {
		t.Fatalf("a table-only twin must refuse the regime program (ok=%v reason=%q)", ok, reason)
	}
}

// The placement scan counts twin ops in FN-UNIT code too. No twin lands
// there today — FnBodyDepth suppresses fn-body transitions at the ledger, so
// only root code carries them — but the arm-resident increment will place
// twins inside compiled units, and a scan blind to unit code would then
// refuse programs whose tables ARE fully placed. Driven directly because no
// compilable program can produce the shape yet.
func TestTwinsFullyPlacedCountsFnUnitOps(t *testing.T) {
	p := &Program{
		BindTwins: []core.BindTransition{{Kind: core.BindDef, Name: "a", Depth: 1}},
		Fns:       []CompiledFn{{Name: "f", Code: []Instr{{Op: OpBindTwin}}}},
	}
	if !twinsFullyPlaced(p) {
		t.Fatal("a twin op inside a fn unit must count toward placement")
	}
	p.BindTwins = append(p.BindTwins, core.BindTransition{Kind: core.BindUndef, Name: "a"})
	if twinsFullyPlaced(p) {
		t.Fatal("a table wider than the placed ops must fail the scan")
	}
}
