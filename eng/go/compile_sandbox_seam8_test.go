package eng

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
	r.Modules.MarkLoaded("aql:w8", ModuleDesc{ID: "mod_w8"})
	s := r.SnapshotForCompile() // exercises the loaded-module copy loop
	// Mutate then restore.
	r.Modules.MarkLoaded("aql:extra", ModuleDesc{ID: "mod_extra"})
	r.RestoreForCompile(s)
	if !r.Modules.IsLoaded("aql:w8") {
		t.Fatal("snapshot must preserve the originally-loaded module")
	}
	if r.Modules.IsLoaded("aql:extra") {
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
