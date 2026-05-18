package aql

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"-version"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "aql") {
		t.Errorf("expected version output, got %q", stdout.String())
	}
}

func TestExecuteEvalExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"-e", "1 add 2"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Errorf("expected '3' in output, got %q", stdout.String())
	}
}

func TestExecuteEvalEmptyResult(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"-e", "1 drop"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Errorf("expected empty output, got %q", stdout.String())
	}
}

func TestExecuteScriptFile(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.aql")
	os.WriteFile(script, []byte("10 mul 5"), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{script}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "50") {
		t.Errorf("expected '50' in output, got %q", stdout.String())
	}
}

func TestExecuteMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"/nonexistent/file.aql"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error in stderr, got %q", stderr.String())
	}
}

func TestExecuteParseError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"-e", `"unterminated`}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "parse error") {
		t.Errorf("expected 'parse error' in stderr, got %q", stderr.String())
	}
}

func TestExecuteEngineError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"-e", "10 div 0"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected 'error' in stderr, got %q", stderr.String())
	}
}

func TestExecuteREPLWithEOF(t *testing.T) {
	// No args, no -e: should start REPL. Provide empty stdin for immediate EOF.
	in := strings.NewReader("")
	var stdout, stderr bytes.Buffer
	code := execute([]string{}, in, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// Should print version header.
	if !strings.Contains(stdout.String(), "aql") {
		t.Errorf("expected 'aql' version in output, got %q", stdout.String())
	}
}

func TestExecuteBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"--invalid-flag"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestRunSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := run(&buf, "1 add 2", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3' in output, got %q", buf.String())
	}
}

func TestRunParseError(t *testing.T) {
	var buf bytes.Buffer
	err := run(&buf, `"unterminated`, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("expected 'parse error', got %q", err.Error())
	}
}

// --- do subcommand ---

func TestExecuteDoSimple(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"do", "1", "add", "2"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Errorf("expected '3' in output, got %q", stdout.String())
	}
}

func TestExecuteDoStringArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"do", `"hello"`, "upper"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "HELLO") {
		t.Errorf("expected 'HELLO' in output, got %q", stdout.String())
	}
}

func TestExecuteDoEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"do"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "requires an expression") {
		t.Errorf("expected error about expression, got %q", stderr.String())
	}
}

func TestExecuteDoError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"do", "10", "div", "0"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected 'error' in stderr, got %q", stderr.String())
	}
}

func TestRunEngineError(t *testing.T) {
	var buf bytes.Buffer
	err := run(&buf, "10 div 0", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- help subcommand ---

func TestExecuteHelpListsWords(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Available words:") {
		t.Errorf("expected 'Available words:' header, got %q", out)
	}
	// Spot-check a few well-known words appear in the listing.
	for _, word := range []string{"add", "concat", "help", "import"} {
		if !strings.Contains(out, word) {
			t.Errorf("expected word %q in help listing", word)
		}
	}
}

func TestExecuteHelpSpecificWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help", "add"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Should contain the word name and signature section — same as in-language help.
	if !strings.Contains(out, "add") {
		t.Errorf("expected 'add' in output, got %q", out)
	}
	if !strings.Contains(out, "Signatures:") {
		t.Errorf("expected 'Signatures:' section, got %q", out)
	}
	if !strings.Contains(out, "Description:") {
		t.Errorf("expected 'Description:' section, got %q", out)
	}
}

func TestExecuteHelpUnknownWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help", "nonexistent_word"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "no help available") {
		t.Errorf("expected 'no help available' message, got %q", stdout.String())
	}
}

func TestExecuteHelpMatchesHelpFormat(t *testing.T) {
	// The CLI "aql help add" should produce dynamic help output with
	// all expected sections.
	var cliOut bytes.Buffer
	code := execute([]string{"help", "add"}, nil, &cliOut, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("CLI help exit code = %d, want 0", code)
	}

	out := cliOut.String()
	for _, section := range []string{"add —", "Precedence:", "Signatures:", "Description:", "Examples:"} {
		if !strings.Contains(out, section) {
			t.Errorf("expected %q section in help output, got:\n%s", section, out)
		}
	}
}

// --- prep subcommand ---

func TestExecutePrepBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`
name: foo
major: 1
minor: 2
patch: 3
files: [a.aql b.aql]
`), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"prep", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := filepath.Join(dir, ".aql", "aql.json")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read output: %s", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid json: %s", err)
	}

	if m["name"] != "foo" {
		t.Errorf("name = %v, want foo", m["name"])
	}
	if m["major"] != float64(1) {
		t.Errorf("major = %v, want 1", m["major"])
	}
	files, ok := m["files"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("files = %v, want [a.aql b.aql]", m["files"])
	}
	if files[0] != "a.aql" || files[1] != "b.aql" {
		t.Errorf("files = %v, want [a.aql b.aql]", files)
	}
}

func TestExecutePrepDefaultDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`name: bar major: 0 minor: 0 patch: 1 files: [index.aql]`), 0644)

	// Change to temp dir so default "." works.
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"prep"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".aql", "aql.json"))
	if err != nil {
		t.Fatalf("failed to read output: %s", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid json: %s", err)
	}
	if m["name"] != "bar" {
		t.Errorf("name = %v, want bar", m["name"])
	}
}

func TestExecutePrepMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"prep", "/nonexistent/dir"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error in stderr, got %q", stderr.String())
	}
}

func TestExecutePrepInvalidJsonic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`{{{`), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"prep", dir}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "jsonic") {
		t.Errorf("expected jsonic error, got %q", stderr.String())
	}
}

// --- pack subcommand ---

func TestExecutePackBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`
name: mymod
major: 1
minor: 2
patch: 3
files: [main.aql helpers.aql]
`), 0644)
	os.WriteFile(filepath.Join(dir, "main.aql"), []byte("1 add 2"), 0644)
	os.WriteFile(filepath.Join(dir, "helpers.aql"), []byte("def x 1"), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"pack", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	zipPath := filepath.Join(dir, ".aql", "_pack", "mymod-1.2.3.zip")
	if !strings.Contains(stdout.String(), "mymod-1.2.3.zip") {
		t.Errorf("expected zip path in output, got %q", stdout.String())
	}

	// Verify aql.json was also created (prep ran).
	if _, err := os.Stat(filepath.Join(dir, ".aql", "aql.json")); err != nil {
		t.Errorf("expected .aql/aql.json to exist: %s", err)
	}

	// Verify zip contents.
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %s", err)
	}
	defer zr.Close()

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}

	for _, want := range []string{"aql.jsonic", "main.aql", "helpers.aql"} {
		if !names[want] {
			t.Errorf("zip missing %q, has %v", want, names)
		}
	}
	if len(zr.File) != 3 {
		t.Errorf("expected 3 files in zip, got %d", len(zr.File))
	}
}

func TestExecutePackOverwrites(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`name: x major: 0 minor: 0 patch: 1 files: [a.aql]`), 0644)
	os.WriteFile(filepath.Join(dir, "a.aql"), []byte("old"), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"pack", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first pack failed: %s", stderr.String())
	}

	// Update file and re-pack.
	os.WriteFile(filepath.Join(dir, "a.aql"), []byte("new content here"), 0644)
	stdout.Reset()
	stderr.Reset()
	code = execute([]string{"pack", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second pack failed: %s", stderr.String())
	}

	// Verify the zip contains the updated content.
	zipPath := filepath.Join(dir, ".aql", "_pack", "x-0.0.1.zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %s", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == "a.aql" {
			rc, _ := f.Open()
			var buf bytes.Buffer
			buf.ReadFrom(rc)
			rc.Close()
			if buf.String() != "new content here" {
				t.Errorf("expected updated content, got %q", buf.String())
			}
		}
	}
}

func TestExecutePackMissingFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`name: x major: 0 minor: 0 patch: 0 files: [missing.aql]`), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"pack", dir}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error in stderr, got %q", stderr.String())
	}
}

// --- clean subcommand ---

func TestExecuteCleanBasic(t *testing.T) {
	dir := t.TempDir()
	aqlDir := filepath.Join(dir, ".aql")
	os.MkdirAll(aqlDir, 0755)

	// Create files and dirs that should be removed.
	os.WriteFile(filepath.Join(aqlDir, "aql.json"), []byte(`{}`), 0644)
	os.MkdirAll(filepath.Join(aqlDir, "_pack"), 0755)
	os.WriteFile(filepath.Join(aqlDir, "_pack", "x.zip"), []byte("zip"), 0644)
	os.MkdirAll(filepath.Join(aqlDir, "color"), 0755)
	os.WriteFile(filepath.Join(aqlDir, "color", "color.aql"), []byte("1"), 0644)

	// Create a dotfile that should survive.
	os.WriteFile(filepath.Join(aqlDir, ".gitkeep"), []byte(""), 0644)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"clean", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Dotfile should still exist.
	if _, err := os.Stat(filepath.Join(aqlDir, ".gitkeep")); err != nil {
		t.Errorf("expected .gitkeep to survive: %s", err)
	}

	// Everything else should be gone.
	for _, name := range []string{"aql.json", "_pack", "color"} {
		if _, err := os.Stat(filepath.Join(aqlDir, name)); err == nil {
			t.Errorf("expected %s to be removed", name)
		}
	}
}

func TestExecuteCleanNoAqlDir(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := execute([]string{"clean", dir}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestExecuteCleanDefaultDir(t *testing.T) {
	dir := t.TempDir()
	aqlDir := filepath.Join(dir, ".aql")
	os.MkdirAll(aqlDir, 0755)
	os.WriteFile(filepath.Join(aqlDir, "aql.json"), []byte(`{}`), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"clean"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(aqlDir, "aql.json")); err == nil {
		t.Error("expected aql.json to be removed")
	}
}

// TestCheckStrictExitNonZeroOnError verifies that `aql check` (the
// default "strict" mode) returns a non-zero exit code when the
// program produces an Error-severity diagnostic.
func TestCheckStrictExitNonZeroOnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"check", "-e", "upper 42"}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for strict check with errors; stderr=%s stdout=%s",
			stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "check failed") {
		t.Errorf("expected 'check failed' in stderr, got %q", stderr.String())
	}
}

// TestCheckSoftExitZeroOnError verifies that `aql check --soft` reports
// errors but still exits zero (advisory mode).
func TestCheckSoftExitZeroOnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"check", "--soft", "-e", "upper 42"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit for soft check; stderr=%s stdout=%s",
			stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "[error]") {
		t.Errorf("expected error diagnostic in stderr, got %q", stderr.String())
	}
}

// TestCheckStrictExitZeroOnClean verifies that `aql check` exits zero
// when no error-severity diagnostics are emitted.
func TestCheckStrictExitZeroOnClean(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"check", "-e", "1 add 2"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit for clean program; stderr=%s stdout=%s",
			stderr.String(), stdout.String())
	}
}
