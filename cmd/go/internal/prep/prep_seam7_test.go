package prep

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestS7n_DoMarshalIndentError drives Do's json.MarshalIndent
// failure arm by swapping the jsonMarshalIndent seam.
func TestS7n_DoMarshalIndentError(t *testing.T) {
	orig := jsonMarshalIndent
	t.Cleanup(func() { jsonMarshalIndent = orig })
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("boom-marshal-indent")
	}

	dir := writeManifest(t, "name: x")
	if _, err := Do(dir); err == nil || err.Error() != "boom-marshal-indent" {
		t.Errorf("Do() err = %v, want boom-marshal-indent", err)
	}
}

// TestS7n_DoWriteFileFailsWhenDstIsDir drives Do's os.WriteFile
// failure arm organically: MkdirAll(".boru") is a no-op when the dir
// already exists, but WriteFile(".boru/boru.json", ...) fails when that
// path is itself a directory.
func TestS7n_DoWriteFileFailsWhenDstIsDir(t *testing.T) {
	dir := writeManifest(t, "name: x")
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "boru.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Do(dir); err == nil {
		t.Error("expected an error when boru.json exists as a directory")
	}
}
