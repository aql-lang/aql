package native

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// enableFsFlag flips one __sys.fs boolean on a fresh engine context —
// the BORU `context dot __sys dot fs set <flag> true` toggle, driven from Go.
func enableFsFlag(t *testing.T, r *Registry, flag string) {
	t.Helper()
	e := New(r)
	if _, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("fs"),
		NewWord("set"), NewWord(flag), NewBoolean(true),
	}); err != nil {
		t.Fatalf("enable __sys.fs.%s: %v", flag, err)
	}
}

// TestOverlayToggleRoutesUnion pins __sys.fs.overlay: reads fall through
// to the HOST fileops, writes land only in the mem upper, deletes are
// whiteouts — and the host layer is never mutated.
func TestOverlayToggleRoutesUnion(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// The "host" layer is itself a MemFileOps here so the test stays
	// hermetic; the overlay stacks the same way over the real OSFileOps.
	host := capabilities.NewMem()
	if err := host.WriteFile("fixture.txt", []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r, host)
	enableFsFlag(t, r, "overlay")

	ops := EffectiveFileOps(r)
	if _, ok := ops.(*capabilities.OverlayFileOps); !ok {
		t.Fatalf("EffectiveFileOps under overlay toggle = %T, want *OverlayFileOps", ops)
	}
	// The overlay instance is CACHED: a second resolution returns the
	// same union (whiteouts and upper writes persist across calls).
	if again := EffectiveFileOps(r); again != ops {
		t.Error("overlay not cached across invocations")
	}

	// Read through the union → the host fixture.
	res := runBORU(t, r, []Value{NewWord("read"), pathV("fixture.txt")})
	if res[0].String() != "'real'" {
		t.Errorf("union read = %v", res[0])
	}
	// Shadow it and delete it — host unchanged throughout.
	runBORU(t, r, []Value{NewWord("write"), pathV("fixture.txt"), NewString("shadow")})
	res = runBORU(t, r, []Value{NewWord("read"), pathV("fixture.txt")})
	if res[0].String() != "'shadow'" {
		t.Errorf("shadowed read = %v", res[0])
	}
	runBORU(t, r, []Value{NewWord("remove"), pathV("fixture.txt")})
	if err := runBORUError(t, r, []Value{NewWord("read"), pathV("fixture.txt")}); err == nil {
		t.Error("whited-out fixture still readable")
	}
	if b, err := host.ReadFile("fixture.txt"); err != nil || string(b) != "real" {
		t.Errorf("HOST layer mutated through the overlay: %q (%v)", b, err)
	}
}

// TestOverlayToggleOverRealDisk is the promised use case end-to-end: real
// fixtures on disk, mutations in memory only.
func TestOverlayToggleOverRealDisk(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	root := t.TempDir()
	fixture := filepath.Join(root, "data.txt")
	if err := os.WriteFile(fixture, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	enableFsFlag(t, r, "overlay")

	res := runBORU(t, r, []Value{NewWord("read"), pathV(fixture)})
	if res[0].String() != "'disk'" {
		t.Errorf("read of real fixture through overlay = %v", res[0])
	}
	runBORU(t, r, []Value{NewWord("write"), pathV(fixture), NewString("changed")})
	if b, err := os.ReadFile(fixture); err != nil || string(b) != "disk" {
		t.Errorf("REAL file mutated: %q (%v)", b, err)
	}
	res = runBORU(t, r, []Value{NewWord("read"), pathV(fixture)})
	if res[0].String() != "'changed'" {
		t.Errorf("union read after shadow write = %v", res[0])
	}
}

// TestMemToggleWinsOverOverlay pins the precedence: {mem:true} is the
// stricter isolation and wins when both flags are set.
func TestMemToggleWinsOverOverlay(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	enableFsFlag(t, r, "overlay")
	enableFsFlag(t, r, "mem")
	if _, ok := EffectiveFileOps(r).(*capabilities.MemFileOps); !ok {
		t.Errorf("mem+overlay routed to %T, want *MemFileOps", EffectiveFileOps(r))
	}
}
