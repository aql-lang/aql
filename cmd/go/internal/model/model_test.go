package model

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestModelBuildWritesJSON pins the core job: `boru model <file>` resolves the
// model and writes <base>/<name>.json.
func TestModelBuildWritesJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("a: 1\nb: x: 2"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	code, out, errOut := run(t, src)
	if code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("stdout %q should report the written file", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "m.json"))
	if err != nil {
		t.Fatalf("read output json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["a"] != float64(1) {
		t.Errorf("a = %v, want 1", got["a"])
	}
	if b, ok := got["b"].(map[string]any); !ok || b["x"] != float64(2) {
		t.Errorf("b.x = %v, want 2", got["b"])
	}
}

// TestModelPrint pins --print: the unified model goes to stdout and no file
// is written.
func TestModelPrint(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("greeting: hello"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	code, out, errOut := run(t, "--print", src)
	if code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errOut)
	}
	if !strings.Contains(out, `"greeting": "hello"`) {
		t.Errorf("stdout %q should contain the model JSON", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.json")); !os.IsNotExist(err) {
		t.Errorf("--print must not write the output file")
	}
}

// TestModelDryrun pins --dryrun: a successful resolve that writes nothing.
func TestModelDryrun(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("a: 1"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	code, out, _ := run(t, "--dryrun", src)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "dryrun") {
		t.Errorf("stdout %q should note dryrun", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.json")); !os.IsNotExist(err) {
		t.Errorf("--dryrun must not write the output file")
	}
}

// TestModelErrors pins the failure exits: a missing file and a unification
// conflict both return non-zero with a message; no args is a usage error.
func TestModelErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		code, _, errOut := run(t, filepath.Join(t.TempDir(), "nope.jsonic"))
		if code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(errOut, "error:") {
			t.Errorf("stderr %q should carry an error", errOut)
		}
	})
	t.Run("conflict", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "m.jsonic")
		if err := os.WriteFile(src, []byte("a: 1\na: 2"), 0o600); err != nil {
			t.Fatalf("write src: %v", err)
		}
		code, _, errOut := run(t, src)
		if code != 1 {
			t.Fatalf("exit %d, want 1; stderr=%s", code, errOut)
		}
		if !strings.Contains(errOut, "error:") {
			t.Errorf("stderr %q should report the conflict", errOut)
		}
	})
	t.Run("no args", func(t *testing.T) {
		code, _, errOut := run(t)
		if code != 2 {
			t.Fatalf("exit %d, want 2", code)
		}
		if !strings.Contains(errOut, "usage:") {
			t.Errorf("stderr %q should show usage", errOut)
		}
	})
}
