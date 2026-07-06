package model

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestS7n_PrintMarshalIndentError drives the --print path's
// json.MarshalIndent error arm (model.go) by swapping the
// jsonMarshalIndent seam: a successfully-resolved model whose encode
// step fails must report the error and exit 1, not print garbage.
func TestS7n_PrintMarshalIndentError(t *testing.T) {
	orig := jsonMarshalIndent
	t.Cleanup(func() { jsonMarshalIndent = orig })
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("boom-marshal-indent")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("greeting: hello"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--print", src}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "boom-marshal-indent") {
		t.Errorf("stderr = %q, want it to contain boom-marshal-indent", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing written on a marshal failure", stdout.String())
	}
}
