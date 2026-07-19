package capabilities

import (
	"errors"
	"os"
	"testing"
)

// overlay_meta_test.go — P2 union semantics: chown / xattr routing,
// copy-up metadata carry, and whiteout hiding.

func metaOverlay(t *testing.T) (*OverlayFileOps, *MemFileOps, *MemFileOps) {
	t.Helper()
	upper, lower := NewMem(), NewMem()
	// Root identity on both layers so chown / metadata carry is allowed.
	upper.SetIdentity(func() int { return 0 }, func() int { return 0 })
	lower.SetIdentity(func() int { return 0 }, func() int { return 0 })
	return NewOverlay(upper, lower), upper, lower
}

func TestOverlayXattrRouting(t *testing.T) {
	o, upper, lower := metaOverlay(t)
	if err := lower.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lower.XattrSet("f", "kept", []byte("low")); err != nil {
		t.Fatal(err)
	}

	// Reads route to the owning layer (lower here).
	if v, err := o.XattrGet("f", "kept"); err != nil || string(v) != "low" {
		t.Errorf("lower xattr through union = %q (%v)", v, err)
	}
	if names, err := o.XattrList("f"); err != nil || len(names) != 1 {
		t.Errorf("union xattr list = %v (%v)", names, err)
	}

	// A set COPIES UP first and carries the existing attributes and
	// ownership; the lower stays pristine.
	if err := lower.Chown("f", 42, 7, true); err != nil {
		t.Fatal(err)
	}
	if err := o.XattrSet("f", "new", []byte("up")); err != nil {
		t.Fatalf("union xattr-set: %v", err)
	}
	if !oInUpper(o, "f") {
		t.Fatal("xattr-set did not copy up")
	}
	if v, err := upper.XattrGet("f", "kept"); err != nil || string(v) != "low" {
		t.Errorf("copy-up dropped prior xattr: %q (%v)", v, err)
	}
	if fi, _ := upper.Stat("f", true); fi.UID != 42 || fi.GID != 7 {
		t.Errorf("copy-up dropped ownership: %d/%d", fi.UID, fi.GID)
	}
	if v, err := o.XattrGet("f", "new"); err != nil || string(v) != "up" {
		t.Errorf("upper xattr = %q (%v)", v, err)
	}
	if _, err := lower.XattrGet("f", "new"); err == nil {
		t.Error("lower gained an upper attribute")
	}

	// Remove routes to the upper post-copy-up.
	if err := o.XattrRemove("f", "kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.XattrGet("f", "kept"); !errors.Is(err, errXattrAbsent) {
		t.Errorf("removed attr still readable: %v", err)
	}
	if v, _ := lower.XattrGet("f", "kept"); string(v) != "low" {
		t.Error("union remove mutated the lower layer")
	}
}

func TestOverlayXattrWhiteoutAndAbsent(t *testing.T) {
	o, _, lower := metaOverlay(t)
	if err := lower.WriteFile("gone", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lower.XattrSet("gone", "a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove("gone", false); err != nil {
		t.Fatal(err)
	}
	// A whiteout hides the lower's attributes and mutations refuse.
	if _, err := o.XattrGet("gone", "a"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whiteout xattr-get: %v", err)
	}
	if _, err := o.XattrList("gone"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whiteout xattr-list: %v", err)
	}
	if err := o.XattrSet("gone", "a", nil); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whiteout xattr-set: %v", err)
	}
	if err := o.Chown("gone", -1, -1, true); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whiteout chown: %v", err)
	}
	// A path in neither layer refuses identically.
	if _, err := o.XattrGet("ghost", "a"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent xattr-get: %v", err)
	}
}

func TestOverlayChownCopyUp(t *testing.T) {
	o, upper, lower := metaOverlay(t)
	if err := lower.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Chown("f", 9, 9, true); err != nil {
		t.Fatalf("union chown: %v", err)
	}
	if fi, _ := upper.Stat("f", true); fi.UID != 9 {
		t.Errorf("chown landed at %d in upper", fi.UID)
	}
	if fi, _ := lower.Stat("f", true); fi.UID == 9 {
		t.Error("union chown mutated the lower layer")
	}
	// An upper-resident path re-owns in place (no second copy-up path).
	if err := o.Chown("f", 5, -1, true); err != nil {
		t.Fatal(err)
	}
	if fi, _ := upper.Stat("f", true); fi.UID != 5 {
		t.Errorf("in-place chown = %d", fi.UID)
	}
}

// TestOverlayCopyUpChownBestEffort pins the best-effort posture: an upper
// whose identity cannot chown (non-root) still completes the copy-up and
// the triggering mutation succeeds — metadata carry is sacrificed, the
// operation is not.
func TestOverlayCopyUpChownBestEffort(t *testing.T) {
	upper, lower := NewMem(), NewMem()
	lower.SetIdentity(func() int { return 0 }, func() int { return 0 })
	upper.SetIdentity(func() int { return 1000 }, func() int { return 1000 })
	o := NewOverlay(upper, lower)
	if err := lower.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lower.Chown("f", 42, 42, true); err != nil {
		t.Fatal(err)
	}
	if err := o.Chmod("f", 0o600); err != nil {
		t.Fatalf("mutation failed because metadata carry failed: %v", err)
	}
	if fi, _ := upper.Stat("f", true); fi.Mode.Perm() != 0o600 {
		t.Errorf("chmod did not land: %v", fi.Mode)
	}
}

// oInUpper reads the unexported inUpper through the public surface.
func oInUpper(o *OverlayFileOps, path string) bool {
	_, err := o.Upper.Stat(path, false)
	return err == nil
}

// TestOverlayXattrDerefLoop covers readSideXattr's union-deref error arm.
func TestOverlayXattrDerefLoop(t *testing.T) {
	o, _, lower := metaOverlay(t)
	if err := lower.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.XattrGet("cyc", "a"); err == nil {
		t.Error("union xattr-get through a symlink loop should error")
	}
}
