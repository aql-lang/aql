package clean

// clean_seam6e_test.go — seam-arm test (design/TEST-SEAMS.10.md) for the
// RemoveAll failure arm, which cannot be forced via permissions as root.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRemoveAllError(t *testing.T) {
	dir := t.TempDir()
	boruDir := filepath.Join(dir, ".boru")
	if err := os.MkdirAll(filepath.Join(boruDir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := osRemoveAll
	osRemoveAll = func(string) error { return errors.New("remove boom") }
	t.Cleanup(func() { osRemoveAll = orig })

	var stdout, stderr bytes.Buffer
	if code := Run([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "remove boom") {
		t.Errorf("stderr = %q, want remove boom", stderr.String())
	}
}
