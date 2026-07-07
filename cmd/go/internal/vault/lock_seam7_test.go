package vault

// lock_seam7_test.go — withVaultLock's MkdirAll and flock failure arms
// (lock.go:34-36, 42-44).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestS7_WithVaultLockMkdirError(t *testing.T) {
	home := testHome(t)
	blocker := filepath.Join(home, "s7blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// Point the vault folder beneath a regular file so MkdirAll fails.
	t.Setenv(EnvFolder, filepath.Join(blocker, "sub"))
	called := false
	if err := withVaultLock(home, func() error { called = true; return nil }); err == nil {
		t.Fatal("expected MkdirAll error for a folder under a file")
	}
	if called {
		t.Error("fn ran despite the lock-folder error")
	}
}

func TestS7_WithVaultLockFlockError(t *testing.T) {
	home := testHome(t)
	old := s7lockFile
	s7lockFile = func(*os.File) error { return errS7Boom }
	t.Cleanup(func() { s7lockFile = old })
	called := false
	if err := withVaultLock(home, func() error { called = true; return nil }); !errors.Is(err, errS7Boom) {
		t.Fatalf("flock arm err = %v, want boom", err)
	}
	if called {
		t.Error("fn ran despite the flock error")
	}
}
