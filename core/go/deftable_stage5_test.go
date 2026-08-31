package core

// Stage-5 coverage tests for deftable.go: the nil-receiver guard on every
// DefTable method (each is documented as nil-safe, so the guards are part
// of the contract), plus SetAt (whole function), the Truncate negative /
// zero-want arms, and Set's empty-bodies arm. All tests use fresh local
// tables — no globals touched.

import "testing"

func TestStage5DefTableNilReceiverGuards(t *testing.T) {
	var dt *DefTable

	if _, ok := dt.Top("x"); ok {
		t.Error("nil Top must report unbound")
	}
	if _, ok := dt.TopEntry("x"); ok {
		t.Error("nil TopEntry must report unbound")
	}
	dt.Push("x", NewInteger(1))                          // no-op, must not panic
	dt.PushType("X", TInteger, NewTypeLiteral(TInteger)) // no-op
	if _, ok := dt.PopEntry("x"); ok {
		t.Error("nil PopEntry must report unbound")
	}
	if got := dt.Mutations(); got != 0 {
		t.Errorf("nil Mutations = %d, want 0", got)
	}
	if !dt.TruncationCoveredBy(nil, func(string) bool { return false }) {
		t.Error("nil TruncationCoveredBy must be vacuously true")
	}
	if dt.Has("x") {
		t.Error("nil Has must be false")
	}
	if dt.IsType("x") {
		t.Error("nil IsType must be false")
	}
	if got := dt.Depth("x"); got != 0 {
		t.Errorf("nil Depth = %d, want 0", got)
	}
	if dt.Replace("x", NewInteger(1)) {
		t.Error("nil Replace must be false")
	}
	if dt.SetAt("x", 1, NewInteger(1)) {
		t.Error("nil SetAt must be false")
	}
	dt.Truncate("x", 0)                 // no-op, must not panic
	dt.Delete("x")                      // no-op
	dt.Set("x", []Value{NewInteger(1)}) // no-op
	if got := dt.Names(); got != nil {
		t.Errorf("nil Names = %v, want nil", got)
	}
	if got := dt.Snapshot(); got != nil {
		t.Errorf("nil Snapshot = %v, want nil", got)
	}
	dt.Restore(map[string]int{"x": 1}) // no-op
	cl := dt.Clone()
	if cl == nil {
		t.Fatal("nil Clone must return a fresh usable table")
	}
	cl.Push("y", NewInteger(2))
	if !cl.Has("y") {
		t.Error("clone of a nil table must be usable")
	}
}

func TestStage5DefTableSetAt(t *testing.T) {
	dt := NewDefTable()

	// depth < 1 is refused outright.
	if dt.SetAt("v", 0, NewInteger(9)) {
		t.Error("SetAt depth 0 must be refused")
	}

	// depth beyond the stack is a no-op false.
	dt.Push("v", NewInteger(1))
	if dt.SetAt("v", 2, NewInteger(9)) {
		t.Error("SetAt past the stack depth must be refused")
	}

	// A valid depth overwrites the body at that 1-based level and
	// preserves the entry's TypeDef.
	dt.PushType("v", TInteger, NewTypeLiteral(TInteger))
	if !dt.SetAt("v", 1, NewInteger(7)) {
		t.Fatal("SetAt depth 1 should succeed")
	}
	stack := dt.Stack("v")
	if len(stack) != 2 {
		t.Fatalf("stack depth = %d, want 2", len(stack))
	}
	if n, err := AsInteger(stack[0]); err != nil || n != 7 {
		t.Errorf("oldest body = %v (%v), want 7", stack[0], err)
	}
	if !dt.SetAt("v", 2, NewInteger(8)) {
		t.Fatal("SetAt depth 2 should succeed")
	}
	e, ok := dt.TopEntry("v")
	if !ok {
		t.Fatal("top entry missing")
	}
	if e.TypeDef == nil || !e.TypeDef.Equal(TInteger) {
		t.Error("SetAt must preserve the entry's TypeDef")
	}
	if n, err := AsInteger(e.Body); err != nil || n != 8 {
		t.Errorf("top body = %v (%v), want 8", e.Body, err)
	}

	// SetAt bumps the generation counter (dispatch-cache invalidation).
	gen := dt.Gen("v")
	dt.SetAt("v", 1, NewInteger(6))
	if dt.Gen("v") <= gen {
		t.Error("SetAt must touch the name's generation")
	}
}

func TestStage5DefTableTruncateArms(t *testing.T) {
	dt := NewDefTable()
	dt.Push("w", NewInteger(1))
	dt.Push("w", NewInteger(2))

	// A negative want clamps to 0 and removes the entry entirely.
	dt.Truncate("w", -3)
	if dt.Has("w") {
		t.Error("Truncate(-3) must clamp to 0 and delete the stack")
	}

	// want == 0 on a live stack deletes the map entry.
	dt.Push("w", NewInteger(3))
	dt.Truncate("w", 0)
	if dt.Has("w") {
		t.Error("Truncate(0) must delete the stack")
	}
}

func TestStage5DefTableSetEmptyBodies(t *testing.T) {
	dt := NewDefTable()
	dt.Push("s", NewInteger(1))
	dt.Set("s", nil)
	if dt.Has("s") {
		t.Error("Set with no bodies must remove the entry")
	}
	if got := dt.Stack("s"); got != nil {
		t.Errorf("Stack after empty Set = %v, want nil", got)
	}
}

// Entries is the bind-twin replay's read: the full DefEntry stack —
// TypeDef and Minted included — where Stack drops to bodies. The copy is
// caller-owned: mutating it must not reach the table.
func TestDefTableEntries(t *testing.T) {
	var nilDT *DefTable
	if nilDT.Entries("x") != nil {
		t.Fatal("nil table must answer nil")
	}
	dt := NewDefTable()
	if dt.Entries("missing") != nil {
		t.Fatal("unbound name must answer nil")
	}
	dt.Push("x", NewInteger(1))
	dt.PushType("x", TInteger, NewInteger(2))
	es := dt.Entries("x")
	if len(es) != 2 || es[0].TypeDef != nil || es[1].TypeDef != TInteger || !es[1].Minted {
		t.Fatalf("entries = %+v, want a value entry under a minted type entry", es)
	}
	es[0].Body = NewInteger(99)
	if fresh := dt.Entries("x"); fresh[0].Body.Data != (IntPayload{N: 1}) {
		t.Fatalf("the returned slice must be a copy; table saw %v", fresh[0].Body)
	}
}

// SnapshotEntries / RestoreEntriesSnapshot is the CONTENT-preserving region
// restore (macro template runs, the dynamic-help eval): unlike the depth
// restore it recreates popped entries and same-depth replacements, unlike a
// table swap it mutates in place, and its gen guard touches only names the
// region actually changed.
func TestSnapshotEntriesRestore(t *testing.T) {
	var nilDT *DefTable
	nilDT.RestoreEntriesSnapshot(nilDT.SnapshotEntries()) // both nil-safe
	dt := NewDefTable()
	dt.RestoreEntriesSnapshot(EntriesSnapshot{}) // invalid snapshot: no-op

	dt.Push("keep", NewInteger(1))
	dt.Push("popme", NewInteger(2))
	dt.PushType("ty", TInteger, NewInteger(3))
	keepGen := dt.Gen("keep")
	snap := dt.SnapshotEntries()

	// The region: pops an entry (the depth restore could NOT undo this),
	// replaces one at the same depth, and introduces a new name.
	dt.Pop("popme")
	dt.Pop("ty")
	dt.Push("ty", NewInteger(9))
	dt.Push("fresh", NewInteger(4))

	dt.RestoreEntriesSnapshot(snap)
	if dt.Depth("popme") != 1 {
		t.Fatalf("a popped entry must be restored: depth = %d", dt.Depth("popme"))
	}
	if e, _ := dt.TopEntry("ty"); e.TypeDef != TInteger || !e.Minted {
		t.Fatalf("a same-depth replacement must be restored: %+v", e)
	}
	if dt.Has("fresh") {
		t.Fatal("a region-introduced name must be deleted")
	}
	if dt.Gen("keep") != keepGen {
		t.Fatal("an untouched name must not be gen-churned by the restore")
	}
	if dt.Gen("popme") == snap.gens["popme"] {
		t.Fatal("a restored name's gen must move FORWARD, never rewind")
	}
}

// The depth-based Snapshot / Restore pair, exercised directly: its former
// main in-package callers (the macro template run, the help eval) moved to
// the content-preserving SnapshotEntries, but the depth pair remains the
// fn-body / carrier-merge sandboxing primitive and keeps its own pin.
func TestStage5DefTableDepthSnapshotRestore(t *testing.T) {
	dt := NewDefTable()
	dt.Push("x", NewInteger(1))
	snap := dt.Snapshot()
	dt.Push("x", NewInteger(2))
	dt.Push("z", NewInteger(3))
	dt.Restore(snap)
	if dt.Depth("x") != 1 {
		t.Fatalf("Restore must truncate x to its snapshot depth: %d", dt.Depth("x"))
	}
	if dt.Has("z") {
		t.Fatal("Restore must delete a name absent from the snapshot")
	}
}
