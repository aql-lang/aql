// Wave-4 coverage for the fmt subcommand: explicit-file and
// walk-the-tree modes, the .aql-directory skip, the no-files and
// unchanged-file paths, read errors, and the Command wrapper methods.
package fmt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "fmt" {
		t.Errorf("Name() = %q, want fmt", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4FmtExplicitFileReformatted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "messy.aql")
	if err := os.WriteFile(path, []byte("1    add     2"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := New().Run([]string{path}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), path) {
		t.Errorf("reformatted file not reported: %q", stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "1    add     2" {
		t.Error("file content unchanged after fmt")
	}
}

func TestW4FmtUnchangedFileNotReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tidy.aql")
	// Format once to learn the canonical form, then format again.
	if err := os.WriteFile(path, []byte("1 add 2"), 0644); err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	if code := Run([]string{path}, &first, &first); code != 0 {
		t.Fatalf("first Run = %d", code)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{path}, &stdout, &stderr); code != 0 {
		t.Fatalf("second Run = %d; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), path) {
		t.Errorf("unchanged file was reported: %q", stdout.String())
	}
}

func TestW4FmtWalkSkipsDotAql(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".aql"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	// A messy file in the tree, and one under .aql that must be skipped.
	if err := os.WriteFile(filepath.Join(dir, "nested", "a.aql"), []byte("1     add   2"), 0644); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(dir, ".aql", "hidden.aql")
	if err := os.WriteFile(hidden, []byte("3      add    4"), 0644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "a.aql") {
		t.Errorf("walk missed nested/a.aql: %q", stdout.String())
	}
	data, err := os.ReadFile(hidden)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "3      add    4" {
		t.Error(".aql/hidden.aql was formatted; the .aql dir must be skipped")
	}
}

func TestW4FmtWalkNoFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no .aql files found") {
		t.Errorf("stdout = %q, want no-files message", stdout.String())
	}
}

func TestW4FmtMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{filepath.Join(t.TempDir(), "zz-missing.aql")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a read error", stderr.String())
	}
}
