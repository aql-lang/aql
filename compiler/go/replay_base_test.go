package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// BindRegistry captures the program registry's bindings at the FIRST bind —
// the rollback base Finalize stamps on the Program (ReplayBase / ReplayReg)
// so RunProgram can roll the check pass's installs back before the twins
// replay (§6.5; Codex P1 on #426). A later re-bind — a module-body or island
// sub-engine binding a foreign sub-registry — neither re-captures nor
// re-targets it; an unbound recorder finalizes with no base (nothing to roll
// back, and the restore of an invalid base is a no-op); a nil receiver is a
// no-op.
func TestBindRegistryCapturesReplayBase(t *testing.T) {
	var nilES *EmitState
	nilES.BindRegistry(nil)

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("pre", core.NewInteger(1))
	es := NewEmitState()
	es.BindRegistry(r)
	r.Defs.Push("passinstall", core.NewInteger(2)) // a check-pass transition after the bind
	other, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.BindRegistry(other) // a sub-engine re-bind: reg moves, the base does not
	prog, reason, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("finalize: %s", reason)
	}
	if prog.ReplayReg != r {
		t.Fatal("ReplayReg must be the FIRST-bound (program) registry, not the re-bound sub-registry")
	}
	r.RestoreBindingsForReplay(prog.ReplayBase)
	if r.Defs.Has("passinstall") || r.Defs.Depth("pre") != 1 {
		t.Fatal("ReplayBase must be the bindings as they stood at the first bind: the later install gone, the earlier one kept")
	}

	// An unbound recorder: no base, no registry — restoring it changes nothing.
	other.Defs.Push("keep", core.NewInteger(3))
	p2, reason, ok := NewEmitState().Finalize(nil)
	if !ok {
		t.Fatalf("finalize: %s", reason)
	}
	if p2.ReplayReg != nil {
		t.Fatal("an unbound recorder must stamp no replay registry")
	}
	other.RestoreBindingsForReplay(p2.ReplayBase)
	if other.Defs.Depth("keep") != 1 {
		t.Fatal("an invalid (zero) base must restore nothing")
	}
}
