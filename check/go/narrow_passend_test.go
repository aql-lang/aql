package check

// The live-depth oracle (test/go/langspec/bind_ledger_live_oracle_test.go)
// caught two ways the pass's registry drifted from its own ledger, and both
// fixes live in this module. These pin them directly:
//
//   - a narrowing push is an ANALYSIS artifact (the same runtime value under
//     a tighter static bound) and must not outlive the pass — left in place
//     it shadowed the real binding for every later Run on the same instance
//     (module-io.tsv:223 — a later read of `l` answered the leaked dynamic
//     carrier instead of the bound Lock);
//   - AnalyseLoopBody's final joined pushes ARE the post-loop environment
//     the pass leaves, so the bind ledger records them, exactly as
//     InstallJoinedDefs records a branch join.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A narrowing whose entry is still on top at pass end is popped: the pass
// leaves the binding it found, not its own refinement of it.
func TestNarrowingPushPoppedAtPassEnd(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()

	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("npq")
	r.Defs.Push("npq", v)

	sig := &core.Signature{Args: []*core.Type{core.TList}, BarrierPos: 1}
	narrowDynamicUses(r, "", sig, []core.Value{v})
	if r.Defs.Depth("npq") != 2 {
		t.Fatalf("the narrowing must push during the pass: depth = %d, want 2", r.Defs.Depth("npq"))
	}

	done()
	if r.Defs.Depth("npq") != 1 {
		t.Fatalf("the narrowing must be popped at pass end: depth = %d, want 1", r.Defs.Depth("npq"))
	}
	got, _ := r.Defs.Top("npq")
	if got.Parent != nil && got.Parent.Equal(core.TList) {
		t.Error("pass end must restore the ORIGINAL binding, not keep the narrowed one")
	}
}

// A narrowing buried under a later real binding is left alone: popping by
// name would remove the real binding, and removing the buried entry would
// shift every ledger depth recorded above it.
func TestNarrowingPopSkipsBuriedEntry(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()

	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("nbq")
	r.Defs.Push("nbq", v)
	sig := &core.Signature{Args: []*core.Type{core.TList}, BarrierPos: 1}
	narrowDynamicUses(r, "", sig, []core.Value{v})

	// A later real rebind of the same name lands on top of the narrowing.
	r.Defs.Push("nbq", core.NewInteger(9))
	depth := r.Defs.Depth("nbq")

	done()
	if r.Defs.Depth("nbq") != depth {
		t.Fatalf("a buried narrowing must be left alone: depth = %d, want %d", r.Defs.Depth("nbq"), depth)
	}
	if got, _ := r.Defs.Top("nbq"); got.Dynamic {
		t.Error("the top binding must remain the later real rebind")
	}
}

// The joined post-loop pushes are ledgered at their live depths — the gap the
// (previously malformed) synthetic while row could not see.
func TestAnalyseLoopBodyLedgersJoinedInstalls(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	done := r.Check.Begin()
	defer done()

	r.Defs.Push("ljaccq", core.NewCarrier(core.TInteger))
	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("ljaccq"), core.NewString("s"),
		core.NewWord("pushq"), core.NewWord("ljfreshq"), core.NewInteger(7),
	})
	AnalyseLoopBody(r, body, nil, nil, false)

	found := map[string]int{}
	for _, tr := range r.Check.BindLedger {
		if tr.Kind == core.BindDef {
			found[tr.Name] = tr.Depth
		}
	}
	if found["ljaccq"] != r.Defs.Depth("ljaccq") || r.Defs.Depth("ljaccq") != 2 {
		t.Errorf("the joined rebind must be ledgered at its live depth: ledger %d, live %d, want 2",
			found["ljaccq"], r.Defs.Depth("ljaccq"))
	}
	if found["ljfreshq"] != r.Defs.Depth("ljfreshq") || r.Defs.Depth("ljfreshq") != 1 {
		t.Errorf("the fresh body binding must be ledgered at its live depth: ledger %d, live %d, want 1",
			found["ljfreshq"], r.Defs.Depth("ljfreshq"))
	}
}
