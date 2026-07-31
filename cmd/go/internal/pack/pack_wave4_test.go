// Wave-4 coverage for the pack subcommand: the happy path (prep +
// zip), the metadata validation errors (missing name / files), the
// prep failure, missing listed files, and the Command wrapper
// methods.
package pack

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule creates a minimal publishable module in a temp dir.
func writeModule(t *testing.T, jsonic string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte(jsonic), 0644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goodJsonic = `name: w4mod
major: 1
minor: 2
patch: 3
files: [index.boru]
`

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "pack" {
		t.Errorf("Name() = %q, want pack", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4PackHappyPath(t *testing.T) {
	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1 add 2"})

	var stdout, stderr bytes.Buffer
	code := New().Run([]string{dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}

	zipPath := strings.TrimSpace(stdout.String())
	if !strings.HasSuffix(zipPath, filepath.Join(".boru", "_pack", "w4mod-1.2.3.zip")) {
		t.Fatalf("unexpected zip path %q", zipPath)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %s", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["boru.jsonic"] || !names["index.boru"] {
		t.Errorf("zip entries = %v, want boru.jsonic and index.boru", names)
	}
}

func TestW4PackCwdDefault(t *testing.T) {
	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1"})
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".boru", "_pack", "w4mod-1.2.3.zip")); err != nil {
		t.Errorf("zip not written: %s", err)
	}
}

func TestW4PackPrepFails(t *testing.T) {
	dir := t.TempDir() // no boru.jsonic at all
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a prep error", stderr.String())
	}
}

func TestW4PackMissingName(t *testing.T) {
	dir := writeModule(t, "major: 1\nfiles: [index.boru]\n", map[string]string{"index.boru": "1"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing name") {
		t.Errorf("stderr = %q, want missing-name error", stderr.String())
	}
}

func TestW4PackMissingFilesList(t *testing.T) {
	dir := writeModule(t, "name: w4mod\nmajor: 1\n", nil)
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing files list") {
		t.Errorf("stderr = %q, want missing-files error", stderr.String())
	}
}

func TestW4PackListedFileMissing(t *testing.T) {
	// files names index.boru but the file does not exist on disk.
	dir := writeModule(t, goodJsonic, nil)
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a read error", stderr.String())
	}
}

func TestW4PackNonStringFilesSkipped(t *testing.T) {
	// A non-string entry in files is skipped rather than fatal.
	jsonic := `name: w4mod
major: 0
minor: 0
patch: 1
files: [index.boru, 42]
`
	dir := writeModule(t, jsonic, map[string]string{"index.boru": "1"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "w4mod-0.0.1.zip") {
		t.Errorf("stdout = %q, want zip path", stdout.String())
	}
}

func TestW4PackZipPathBlocked(t *testing.T) {
	// The target zip path exists as a DIRECTORY, so os.Create fails.
	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1"})
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "_pack", "w4mod-1.2.3.zip"), 0755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a create error", stderr.String())
	}
}

func TestW4PackPackDirBlocked(t *testing.T) {
	// .boru/_pack exists as a FILE, so MkdirAll fails.
	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1"})
	if err := os.MkdirAll(filepath.Join(dir, ".boru"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".boru", "_pack"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a mkdir error", stderr.String())
	}
}
