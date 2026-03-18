package main

import (
	"bytes"
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
