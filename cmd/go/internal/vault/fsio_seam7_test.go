package vault

// fsio_seam7_test.go — writeFileAtomic per-step failure arms and the
// best-effort syncDir open-error arm, driven through the shared file
// seams (design/TEST-SEAMS.10.md).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errS7Boom is the shared sentinel returned by swapped seams across the
// seam7 coverage tests.
var errS7Boom = errors.New("s7 boom")

// s7noTempLeft asserts writeFileAtomic removed its temp file after a
// mid-write failure (the renamed==false defer arm).
func s7noTempLeft(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestS7_WriteFileAtomicChmodError(t *testing.T) {
	old := fileChmod
	fileChmod = func(*os.File, os.FileMode) error { return errS7Boom }
	t.Cleanup(func() { fileChmod = old })
	dir := t.TempDir()
	if err := writeFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0600); !errors.Is(err, errS7Boom) {
		t.Fatalf("chmod arm err = %v, want boom", err)
	}
	s7noTempLeft(t, dir)
}

func TestS7_WriteFileAtomicWriteError(t *testing.T) {
	old := fileWrite
	fileWrite = func(*os.File, []byte) (int, error) { return 0, errS7Boom }
	t.Cleanup(func() { fileWrite = old })
	dir := t.TempDir()
	if err := writeFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0600); !errors.Is(err, errS7Boom) {
		t.Fatalf("write arm err = %v, want boom", err)
	}
	s7noTempLeft(t, dir)
}

func TestS7_WriteFileAtomicSyncError(t *testing.T) {
	old := fileSync
	fileSync = func(*os.File) error { return errS7Boom }
	t.Cleanup(func() { fileSync = old })
	dir := t.TempDir()
	if err := writeFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0600); !errors.Is(err, errS7Boom) {
		t.Fatalf("sync arm err = %v, want boom", err)
	}
	s7noTempLeft(t, dir)
}

func TestS7_WriteFileAtomicCloseError(t *testing.T) {
	old := fileClose
	fileClose = func(*os.File) error { return errS7Boom }
	t.Cleanup(func() { fileClose = old })
	dir := t.TempDir()
	if err := writeFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0600); !errors.Is(err, errS7Boom) {
		t.Fatalf("close arm err = %v, want boom", err)
	}
	s7noTempLeft(t, dir)
}

func TestS7_WriteFileAtomicRenameError(t *testing.T) {
	old := osRename
	osRename = func(string, string) error { return errS7Boom }
	t.Cleanup(func() { osRename = old })
	dir := t.TempDir()
	if err := writeFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0600); !errors.Is(err, errS7Boom) {
		t.Fatalf("rename arm err = %v, want boom", err)
	}
	s7noTempLeft(t, dir)
}

func TestS7_SyncDirOpenError(t *testing.T) {
	// syncDir is best-effort: an un-openable directory is swallowed (no
	// panic, no return value) — this drives the os.Open error arm.
	syncDir(filepath.Join(t.TempDir(), "does-not-exist"))
}
