package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/voxgig/model/go"
)

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "model" {
		t.Errorf("Name = %q, want model", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestHelpFlagExitsZero(t *testing.T) {
	code, _, errOut := run(t, "-h")
	if code != 0 {
		t.Errorf("exit = %d, want 0 (flag.ErrHelp)", code)
	}
	if !strings.Contains(errOut, "usage: aql model") {
		t.Errorf("stderr should carry the usage; got %q", errOut)
	}
}

func TestUnknownFlagExitsTwo(t *testing.T) {
	code, _, _ := run(t, "--nope")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestTooManyArgsIsUsageError(t *testing.T) {
	code, _, errOut := run(t, "a.jsonic", "b.jsonic")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "usage:") {
		t.Errorf("stderr = %q", errOut)
	}
}

// TestModelBaseFlag pins --base: output JSON goes to the base dir,
// not the source dir.
func TestModelBaseFlag(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	src := filepath.Join(srcDir, "m.jsonic")
	if err := os.WriteFile(src, []byte("a: 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run(t, "--base", outDir, src)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	want := filepath.Join(outDir, "m.json")
	if !strings.Contains(out, want) {
		t.Errorf("stdout %q should name %q", out, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("output not written under --base: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "m.json")); !os.IsNotExist(err) {
		t.Error("output must not be written next to the source when --base is set")
	}
}

func TestOutputPath(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"", "/work/m.jsonic", filepath.Join("/work", "m.json")},
		{"/out", "/work/m.jsonic", filepath.Join("/out", "m.json")},
		{"", "/work/noext", filepath.Join("/work", "noext.json")},
		{"/out", "rel/dir/x.jsonic", filepath.Join("/out", "x.json")},
	}
	for _, c := range cases {
		spec := model.ModelSpec{Base: c.base}
		if got := outputPath(spec, c.path); got != c.want {
			t.Errorf("outputPath(base=%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}
