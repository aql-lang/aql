package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArgsScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "args.aql")
	src := "import \"aql:io\"\nIO.args\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// Positionals after the script path reach the program as IO.args.
func TestExecuteScriptArgs(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{file, "notes.json", "--fast"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "notes.json") || !strings.Contains(got, "--fast") {
		t.Errorf("stdout %q does not carry the script args", got)
	}
}

// No positionals → IO.args prints as an empty list, not an error.
func TestExecuteScriptArgsEmpty(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "[]") {
		t.Errorf("stdout %q, want an empty list", out.String())
	}
}

// In -e mode every positional is a script argument.
func TestExecuteEvalModeArgs(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{"-e", `import "aql:io"  IO.args`, "alpha"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "alpha") {
		t.Errorf("stdout %q does not carry the -e args", out.String())
	}
}
