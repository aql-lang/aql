package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// Direct pins for the dry-pass plumbing (drypass.go): operand
// canonicalisation and the concreteness gate.

func TestEmitIndexOOBCaughtBodyReattributed(t *testing.T) {
	// Inside a `do [...]` body the consuming word's runtime error is
	// trapped, so the provable-OOB finding is re-attributed: downgraded to
	// info with CaughtAtRuntime stamped (never an error verdict), while
	// the same finding outside the region keeps SeverityError.
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Check.Begin())
	r.Check.CaughtBodyDepth = 1
	check.CheckListIndex(r, core.NewInteger(5), core.NewList([]core.Value{core.NewInteger(1)}), "getr")
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("caught-body OOB not recorded: %+v", r.Check.Diagnostics)
	}
	if d := r.Check.Diagnostics[0]; d.Severity != core.SeverityInfo || !d.CaughtAtRuntime || !d.RuntimeMirror {
		t.Fatalf("caught-body OOB not re-attributed (want info + CaughtAtRuntime + RuntimeMirror): %+v", d)
	}
	r.Check.CaughtBodyDepth = 0
	check.CheckListIndex(r, core.NewInteger(5), core.NewList([]core.Value{core.NewInteger(1)}), "getr")
	last := r.Check.Diagnostics[len(r.Check.Diagnostics)-1]
	if last.Severity != core.SeverityError || last.CaughtAtRuntime {
		t.Fatalf("uncaught OOB must keep the error verdict: %+v", last)
	}
}

func TestBeginCompilePassArmsTheRitual(t *testing.T) {
	// Nil receiver: total, like Begin (the host may hold a zero registry).
	if done := (*core.CheckState)(nil).BeginCompilePass(); done == nil {
		t.Fatal("nil-receiver BeginCompilePass must still return a done func")
	} else {
		done()
	}

	// The shared compile-pass helper must arm every piece of the ritual —
	// a hand-rolled copy missing one (Vm.compile shipped without the
	// Compiling flag) is the bug class this helper retires.
	c := &core.CheckState{FnSummaries: map[string][]core.Value{"x": nil}, FnInflight: map[string]bool{"x": true}}
	done := c.BeginCompilePass()
	defer done()
	if !c.Mode || !c.Compiling {
		t.Fatalf("Mode/Compiling not armed: mode=%v compiling=%v", c.Mode, c.Compiling)
	}
	if c.Emit == core.TheInactiveEmit || c.Emit == nil {
		t.Fatal("a real EmitState must be installed")
	}
	if c.FnSummaries != nil || c.FnInflight != nil {
		t.Fatal("fn-body memos must be dropped so bodies re-record under this pass")
	}
	done()
	if c.Mode {
		t.Fatal("done() must switch check mode off")
	}
}
