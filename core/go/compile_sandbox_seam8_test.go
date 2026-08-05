package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for the nil-guard,
// module-copy loop, and CheckState-restore else arm of compile_sandbox.go.
// Per design/TEST-SEAMS.10.md.

import "testing"

func TestW8SnapshotForCompileNilRegistry(t *testing.T) {
	var r *Registry
	if s := r.SnapshotForCompile(); s.valid {
		t.Fatal("nil registry yields an invalid snapshot")
	}
}

func TestW8RestoreForCompileNilAndInvalid(t *testing.T) {
	var r *Registry
	r.RestoreForCompile(CompileSandbox{})        // nil receiver: no-op
	w8reg(t).RestoreForCompile(CompileSandbox{}) // invalid snapshot: no-op
}

func TestW8SnapshotRestoreModuleRoundTrip(t *testing.T) {
	r := w8reg(t)
	if r.Modules == nil {
		t.Skip("registry has no module registry")
	}
	r.Modules.MarkLoaded("boru:w8", ModuleDesc{ID: "mod_w8"})
	s := r.SnapshotForCompile() // exercises the loaded-module copy loop
	// Mutate then restore.
	r.Modules.MarkLoaded("boru:extra", ModuleDesc{ID: "mod_extra"})
	r.RestoreForCompile(s)
	if !r.Modules.IsLoaded("boru:w8") {
		t.Fatal("snapshot must preserve the originally-loaded module")
	}
	if r.Modules.IsLoaded("boru:extra") {
		t.Fatal("restore must drop the post-snapshot module")
	}
}

func TestW8RestoreForCompileNilCheck(t *testing.T) {
	r := w8reg(t)
	s := r.SnapshotForCompile()
	// r.Check == nil at restore time takes the else arm (assign, not
	// in-place copy).
	r.Check = nil
	r.RestoreForCompile(s)
	if r.Check == nil {
		t.Fatal("restore must reinstall the snapshot's CheckState")
	}
}

// TestW8RestoreForCompileClearsDispatchCache pins that a compile rollback
// drops the registry's dispatchCache entries. DefTable.Clone starts a fresh
// gen timeline at 0, so reinstalling the clone under a cache keyed by
// (name, gen) could otherwise serve a gen-0 entry cached before the rollback
// for a name whose restored binding differs. The cache must be EMPTIED IN
// PLACE (holder kept, entries dropped) so the fallback interpreter rebuilds
// every aggregate from the restored state without swapping the pointer a
// shared registry's concurrent readers hold.
func TestW8RestoreForCompileClearsDispatchCache(t *testing.T) {
	r := w8reg(t)
	r.upsertFnDef("w", Signature{Args: []*Type{TInteger}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil })})
	s := r.SnapshotForCompile()
	// Populate the runtime-mode dispatch cache.
	if r.Lookup("w") == nil {
		t.Fatal("Lookup(w) = nil")
	}
	holder := r.dispatchCache
	if holder == nil {
		t.Fatal("precondition: NewRegistry did not allocate the dispatch cache holder")
	}
	holder.mu.Lock()
	populated := len(holder.m)
	holder.mu.Unlock()
	if populated == 0 {
		t.Fatal("precondition: dispatch cache not populated")
	}
	r.RestoreForCompile(s)
	if r.dispatchCache != holder {
		t.Fatal("RestoreForCompile swapped the dispatch cache holder; must reset in place")
	}
	holder.mu.Lock()
	remaining := len(holder.m)
	holder.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("RestoreForCompile left %d dispatch cache entries", remaining)
	}
	// A subsequent lookup rebuilds cleanly against the restored table.
	if r.Lookup("w") == nil {
		t.Fatal("Lookup(w) after restore = nil")
	}
}
