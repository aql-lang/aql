package vault

// t7_util_seam7_test.go — shared helpers for the TestT7_* seam7 coverage pass
// over the interactive vault TUI. Every identifier is t7-prefixed so the file
// merges cleanly alongside the sibling seam agents' files in this package.

import (
	"os"
	"path/filepath"
	"testing"
)

// t7uninitCtl returns a controller pointed at a home with no initialized vault,
// so requireStore-backed reads and every run* mutation fail.
func t7uninitCtl(t *testing.T) *tuiController {
	t.Helper()
	home := testHome(t)
	ctl := newTUIController(home)
	ctl.setActiveVault(vaultFolder(home), "")
	return ctl
}

// t7corruptStoreCtl returns a controller whose active vault metadata file is
// unparseable, so LoadStore errors (rather than merely returning nil).
func t7corruptStoreCtl(t *testing.T) *tuiController {
	t.Helper()
	home := testHome(t)
	folder := filepath.Join(home, "broken")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "vault.jsonic"), []byte("{ not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctl := newTUIController(home)
	ctl.setActiveVault(folder, "")
	return ctl
}
