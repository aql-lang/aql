package lang

import (
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// The low-level compile-then-run flow — CompileCheck, then eng.RunProgram
// directly, with none of RunAutoValues' bookkeeping — inherits the twin
// regime's rollback from the Program itself (ReplayBase, §6.5). Codex's P1 on
// #426: before the base rode with the program, this flow replayed `def X
// (refine Integer)` onto the check pass's kept install (depth 2), so a later
// `undef X` left a binding and its type live. The negative half is the
// second request: after `undef X` nothing of X may remain.
func TestLowLevelCompileRunFlowRollsBackBeforeReplay(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	run := func(src string) {
		t.Helper()
		prog, reason, _, err := a.CompileCheck(src)
		if err != nil || prog == nil {
			t.Fatalf("compile %q: err=%v reason=%s", src, err, reason)
		}
		if _, err := eng.RunProgram(prog, reg); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
	}
	run("def X (refine Integer)")
	if d := reg.Defs.Depth("X"); d != 1 {
		t.Fatalf("X depth after the low-level flow = %d, want 1 — the replay must land on the rolled-back base, not on the pass's kept install", d)
	}
	run("undef X")
	if reg.Defs.Has("X") {
		t.Fatal("undef X must leave nothing: a second install stacked by the low-level flow would survive it")
	}
}
