package core

import "testing"

// RestoreBindingsForReplay restores from a CLONE: the snapshot is a Program's
// rollback base (compiler.Program.ReplayBase) and a program may run more than
// once, so a second restore must roll back to the SAME base — not to the
// table the first run mutated after the restore installed it live. The
// negative half is the snapshot's own table, which no run may touch.
func TestRestoreBindingsForReplayRestoresFromAClone(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("base", NewInteger(0))
	snap := r.SnapshotBindings()
	r.Defs.Push("x", NewInteger(1)) // the check pass's install
	r.RestoreBindingsForReplay(snap)
	if r.Defs.Has("x") {
		t.Fatal("the first restore must roll the pass's install back")
	}
	r.Defs.Push("x", NewInteger(2)) // the first run's replay
	if d := r.Defs.Depth("x"); d != 1 {
		t.Fatalf("x depth after the first run = %d, want 1", d)
	}
	r.RestoreBindingsForReplay(snap) // a second run's rollback
	if r.Defs.Has("x") {
		t.Fatal("the second restore must roll back to the pristine base — the first run's install must not survive in the snapshot")
	}
	if snap.defs.Has("x") {
		t.Fatal("the snapshot's own table must never be mutated by a run")
	}
	if d := r.Defs.Depth("base"); d != 1 {
		t.Fatalf("the pre-snapshot binding must survive every restore: depth %d, want 1", d)
	}
}
