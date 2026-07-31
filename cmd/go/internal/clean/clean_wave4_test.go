// Wave-4 coverage for the clean subcommand: deleting .boru contents
// while preserving dotfiles, the explicit-dir and cwd forms, the
// missing-directory error, and the Command wrapper methods.
package clean

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupBoruDir creates dir/.boru populated with a plain file, a
// subdirectory, and a dotfile.
func setupBoruDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	boru := filepath.Join(dir, ".boru")
	if err := os.MkdirAll(filepath.Join(boru, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"plain.json", filepath.Join("sub", "nested.txt"), ".keepme"} {
		if err := os.WriteFile(filepath.Join(boru, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "clean" {
		t.Errorf("Name() = %q, want clean", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4CleanExplicitDir(t *testing.T) {
	dir := setupBoruDir(t)
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "cleaned ") {
		t.Errorf("missing cleaned message: %q", stdout.String())
	}

	boru := filepath.Join(dir, ".boru")
	if _, err := os.Stat(filepath.Join(boru, "plain.json")); !os.IsNotExist(err) {
		t.Error("plain.json survived clean")
	}
	if _, err := os.Stat(filepath.Join(boru, "sub")); !os.IsNotExist(err) {
		t.Error("sub/ survived clean")
	}
	if _, err := os.Stat(filepath.Join(boru, ".keepme")); err != nil {
		t.Errorf("dotfile was deleted: %s", err)
	}
}

func TestW4CleanCwdDefault(t *testing.T) {
	dir := setupBoruDir(t)
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".boru", "plain.json")); !os.IsNotExist(err) {
		t.Error("plain.json survived clean")
	}
}

func TestW4CleanMissingBoruDir(t *testing.T) {
	dir := t.TempDir() // no .boru inside
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("missing error output: %q", stderr.String())
	}
}
