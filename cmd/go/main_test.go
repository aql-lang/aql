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
	// Parse failures surface as AQL-voice syntax_errors with the
	// closing-quote help (design/DIAGNOSTICS.0.md phase 2).
	if !strings.Contains(stderr.String(), "[aql/syntax_error]") ||
		!strings.Contains(stderr.String(), "this string is never closed") {
		t.Errorf("expected an AQL-voice syntax_error in stderr, got %q", stderr.String())
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
	code := execute([]string{"do", `"aql:string-util"`, "import", "end", `"hello"`, "StringUtil.upper"}, nil, &stdout, &stderr)
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

// --- help subcommand (CLI overview) ---

func TestExecuteHelpShowsCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Commands:") {
		t.Errorf("expected 'Commands:' header, got %q", out)
	}
	// Spot-check that subcommands are listed and that it points at describe.
	for _, want := range []string{"run", "check", "describe", "aql describe <word>"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in help overview", want)
		}
	}
}

func TestExecuteHelpSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help", "check"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "aql check —") {
		t.Errorf("expected 'aql check —' summary, got %q", out)
	}
}

func TestExecuteHelpUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help", "nonexistent_word"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "unknown command") {
		t.Errorf("expected 'unknown command' message, got %q", stdout.String())
	}
}

// --- describe subcommand (language words and modules) ---

func TestExecuteDescribeListsWordsAndModules(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Words:") || !strings.Contains(out, "Modules") {
		t.Errorf("expected 'Words:' and 'Modules' headers, got %q", out)
	}
	for _, want := range []string{"add", "concat", "import", "math"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in describe listing", want)
		}
	}
}

func TestExecuteDescribeWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "add"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, section := range []string{"add —", "Precedence:", "Signatures:", "Description:", "Examples:"} {
		if !strings.Contains(out, section) {
			t.Errorf("expected %q section in describe output, got:\n%s", section, out)
		}
	}
}

func TestExecuteDescribeModule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "math-util"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "aql:math-util") {
		t.Errorf("expected 'aql:math' header, got %q", out)
	}
	if !strings.Contains(out, "MathUtil.sin") {
		t.Errorf("expected 'MathUtil.sin' export, got %q", out)
	}
}

func TestExecuteDescribeUnknownWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "nonexistent_word"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "no description available") {
		t.Errorf("expected 'no description available' message, got %q", stdout.String())
	}
}

// The no-argument index is grouped by category: category names head the word
// listing, and the drill-in footer points at every describe form.
func TestExecuteDescribeIndexIsCategorised(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"math", "compare", "boolean", "type", // category headers
		"aql describe <category>",          // drill-in guidance
		"aql describe aql:<module>:<word>", // module-word guidance
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in categorised index, got:\n%s", want, out)
		}
	}
}

// `aql describe <category>` lists that category's words with summaries.
func TestExecuteDescribeCategory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "math"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "math —") {
		t.Errorf("expected 'math —' category header, got:\n%s", out)
	}
	// `add` is core; `sqrt` and `pi` come from aql:math-util and are listed
	// under that import. `pi` was previously listed as `math-pi`, a name no
	// word has ever had — the export is MathUtil.pi.
	for _, want := range []string{"add", "sqrt", "pi"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q listed in math category, got:\n%s", want, out)
		}
	}
	// The module words must be shown under the import that binds them, not
	// mixed in with the core words as though they were callable bare.
	if !strings.Contains(out, "import \"aql:math-util\"") {
		t.Errorf("math-util words listed without naming the import they need, got:\n%s", out)
	}
	// A word from another category must not leak into the listing. Anchor on
	// the "  <word> " line format so we don't trip over the substring "concat"
	// inside add's "concatenate" summary.
	if strings.Contains(out, "\n  concat ") {
		t.Errorf("string word 'concat' should not be listed in the math category")
	}
}

// `aql describe aql:<module>` describes a module via its prefixed id.
func TestExecuteDescribeModulePrefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "aql:type-util"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "aql:type-util") || !strings.Contains(out, "TypeUtil.tpartial") {
		t.Errorf("expected module header and an export, got:\n%s", out)
	}
}

// `aql describe aql:<module>:<word>` documents a single module export.
func TestExecuteDescribeModuleWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "aql:type-util:tpartial"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, section := range []string{"TypeUtil.tpartial —", "Module: aql:type-util", "Signatures:"} {
		if !strings.Contains(out, section) {
			t.Errorf("expected %q in module-word output, got:\n%s", section, out)
		}
	}
}

// An unknown native module reports a load error, not "no description".
func TestExecuteDescribeUnknownModule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "aql:nope"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "cannot load module") {
		t.Errorf("expected 'cannot load module' message, got %q", stdout.String())
	}
}

// A known module asked for a word it does not export reports the miss and
// hints at the words it does export.
func TestExecuteDescribeModuleMissingWord(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", "aql:type-util:nosuchword"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "no exported word") {
		t.Errorf("expected 'no exported word' message, got %q", out)
	}
	if !strings.Contains(out, "TypeUtil.tpartial") {
		t.Errorf("expected an exported-words hint, got %q", out)
	}
}

// `aql describe <file.aql>` loads and describes a module that is not compiled
// in — the "attempt to load a module if unknown" path.
func TestExecuteDescribeLoadsFileModule(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.aql")
	src := "def double fn [[n:Integer] [Integer] [n mul 2]]\n" +
		"export \"Lib\" {double: double/r}\n"
	if err := os.WriteFile(lib, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := execute([]string{"describe", lib}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Lib.double") {
		t.Errorf("expected loaded module export 'Lib.double', got:\n%s", stdout.String())
	}
}

// `aql help` introduces both help (the tool) and describe (the language).
func TestExecuteHelpGuidesHelpAndDescribe(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"aql help vault", "aql describe <category>", "documents the language"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in help overview, got:\n%s", want, out)
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

// TestCheckStrictReportsDynamicDispatch verifies that `aql check --strict`
// surfaces the non-gating dynamic_dispatch advisory for a dispatch over a
// dynamic operand, and that the same program is silent without the flag.
func TestCheckStrictReportsDynamicDispatch(t *testing.T) {
	src := `def f fn [[x:Any] [Any] [x]] (f 1) add 1`
	var stdout, stderr bytes.Buffer
	code := execute([]string{"check", "--strict", "-e", src}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("--strict advisory must not gate: exit %d, stderr %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "dynamic_dispatch") {
		t.Errorf("--strict must report dynamic_dispatch, got: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = execute([]string{"check", "-e", src}, nil, &stdout, &stderr)
	if code != 0 || strings.Contains(stderr.String(), "dynamic_dispatch") {
		t.Errorf("without --strict the advisory must be absent (exit %d): %s", code, stderr.String())
	}
}
