package lsp

import (
	"errors"
	"io"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestS7n_ComputeDiagnosticsLangNewFailure drives computeDiagnostics's
// init-failure arm (diagnostics.go) by swapping the langNew seam so
// lang.New fails; the server must synthesise a single boru/init
// diagnostic carrying the underlying error message.
func TestS7n_ComputeDiagnosticsLangNewFailure(t *testing.T) {
	orig := langNew
	t.Cleanup(func() { langNew = orig })
	langNew = func(opts ...lang.Options) (*lang.Boru, error) {
		return nil, errors.New("boom-init")
	}

	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	diags := s.computeDiagnostics("1 add 2")
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want exactly 1", diags)
	}
	d := diags[0]
	if d.Code != "boru/init" {
		t.Errorf("Code = %q, want boru/init", d.Code)
	}
	if d.Severity != severityError {
		t.Errorf("Severity = %v, want severityError", d.Severity)
	}
	if !strings.Contains(d.Message, "boom-init") {
		t.Errorf("Message = %q, want it to contain boom-init", d.Message)
	}
}
