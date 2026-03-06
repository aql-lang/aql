package repl

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestHistoryFile(t *testing.T) {
	path := historyFile()
	// Should return a non-empty path containing ".aql_history".
	if path == "" {
		t.Fatal("historyFile() returned empty string")
	}
	if !strings.Contains(path, ".aql_history") {
		t.Errorf("historyFile() = %q, want path containing .aql_history", path)
	}
}

func TestToReadCloserWithReadCloser(t *testing.T) {
	// An io.ReadCloser should be returned as-is.
	rc := io.NopCloser(strings.NewReader("test"))
	got := toReadCloser(rc)
	if got != rc {
		t.Error("expected same ReadCloser back")
	}
}

func TestToReadCloserWithReader(t *testing.T) {
	// A plain io.Reader should be wrapped in NopCloser.
	r := strings.NewReader("test")
	got := toReadCloser(r)
	if got == nil {
		t.Fatal("toReadCloser returned nil")
	}
	// Verify it reads correctly.
	buf := make([]byte, 4)
	n, err := got.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "test" {
		t.Errorf("got %q, want %q", string(buf[:n]), "test")
	}
}

func TestStartWithEOF(t *testing.T) {
	// Provide empty input to trigger immediate EOF.
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	Start(in, out)
	// Should exit gracefully without panic.
}

func TestStartWithExpression(t *testing.T) {
	// Provide a simple expression followed by EOF.
	in := strings.NewReader("1 add 2\n")
	out := &bytes.Buffer{}
	Start(in, out)
	// The output should contain "3".
	if !strings.Contains(out.String(), "3") {
		t.Errorf("expected output to contain '3', got %q", out.String())
	}
}

func TestStartWithParseError(t *testing.T) {
	in := strings.NewReader("\"unterminated\n")
	out := &bytes.Buffer{}
	Start(in, out)
	if !strings.Contains(out.String(), "parse error") {
		t.Errorf("expected parse error in output, got %q", out.String())
	}
}

func TestStartWithEngineError(t *testing.T) {
	in := strings.NewReader("10 div 0\n")
	out := &bytes.Buffer{}
	Start(in, out)
	if !strings.Contains(out.String(), "error") {
		t.Errorf("expected error in output, got %q", out.String())
	}
}
