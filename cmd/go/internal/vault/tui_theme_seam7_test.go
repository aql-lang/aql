package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestT7_SaveTUIPrefsMkdirError drives saveTUIPrefs' MkdirAll error arm by
// making ~/.boru a regular file, so creating it as a directory fails with
// ENOTDIR (root-proof: a type conflict, not a permission check).
func TestT7_SaveTUIPrefsMkdirError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".boru"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveTUIPrefs(dir, tuiPrefs{Theme: "dark"}); err == nil {
		t.Error("saveTUIPrefs should fail when ~/.boru is a file, not a directory")
	}
}

// TestT7_SaveTUIPrefsMarshalError drives saveTUIPrefs' MarshalIndent error arm
// via the t7jsonMarshalIndent seam (tuiPrefs never fails to marshal in
// production).
func TestT7_SaveTUIPrefsMarshalError(t *testing.T) {
	dir := t.TempDir()
	orig := t7jsonMarshalIndent
	t.Cleanup(func() { t7jsonMarshalIndent = orig })
	t7jsonMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("t7 marshal boom")
	}
	if err := saveTUIPrefs(dir, tuiPrefs{Theme: "dark"}); err == nil {
		t.Error("saveTUIPrefs should surface a marshal error")
	}
}
