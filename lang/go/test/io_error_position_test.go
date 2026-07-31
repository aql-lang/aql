package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// runIO runs src on a production-wired instance (lang.New installs the
// native module resolver, which `import "boru:io"` needs) and returns the
// error. The plain runWithSource harness in error_format_test.go builds a
// bare registry with no resolver.
func runIO(t *testing.T, src string) error {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Run(src)
	return err
}

// boruErrorWithCode asserts err is a *BoruError carrying code.
func boruErrorWithCode(t *testing.T, err error, code string) *native.BoruError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	ae, ok := err.(*native.BoruError)
	if !ok {
		t.Fatalf("expected *BoruError, got %T: %v", err, err)
	}
	if ae.Code != code {
		t.Fatalf("expected code %q, got %q: %s", code, ae.Code, ae.Error())
	}
	return ae
}

// A failed boru:io operation must carry BOTH an [boru/…] code and a source
// position. Neither half was true before: IO.read's file-open failures
// returned a bare fmt.Errorf with no code at all (while the rarely-taken
// "cannot read from an output stream" guard beside them DID carry
// read_error — the same word reporting two different ways depending on
// which branch failed), and nothing in boru:io reported a position.
//
// Position is the harder half, and it needs two things that are easy to
// get half-right:
//   - the reporting site must use BoruErrorAt with a real SrcPos, and
//   - the VALUE being blamed must actually carry one. `make Pathon "/x"`
//     constructs a fresh value, so the constructor has to copy the
//     literal's position onto it (micron.go micronInstantiate). Threading
//     the reporting site alone leaves "source position unknown" — which
//     is exactly what the first cut of this change produced.
func TestIOReadErrorCarriesCodeAndPosition(t *testing.T) {
	err := runIO(t, "import \"boru:io\"\nIO.read (make Pathon \"/nonexistent/boru-test-missing.txt\")\n")
	ae := boruErrorWithCode(t, err, "read_error")

	if ae.Row == 0 {
		t.Errorf("read_error has no source position (Row 0) — it renders as "+
			"'source position unknown'; got: %s", ae.Error())
	}
	// The blame belongs to the path literal on line 2, not line 1's import.
	if ae.Row != 2 {
		t.Errorf("expected the error to blame line 2 (the path), got row %d:\n%s", ae.Row, ae.Error())
	}
	if !strings.Contains(ae.Error(), "--> 2:") {
		t.Errorf("rendered error should carry the arrow, got:\n%s", ae.Error())
	}
}

// The position follows the VALUE, not the call site: a path bound on one
// line and read on another blames where the bad path was written. That is
// the more useful attribution, and it only works because the constructor
// carries the position onto the instance.
func TestIOReadErrorBlamesPathDefinitionSite(t *testing.T) {
	err := runIO(t, "import \"boru:io\"\ndef p (make Pathon \"/nonexistent/boru-test-missing.txt\")\nIO.read p\n")
	ae := boruErrorWithCode(t, err, "read_error")
	if ae.Row != 2 {
		t.Errorf("expected blame on line 2 (where the path was written), got row %d:\n%s",
			ae.Row, ae.Error())
	}
}

// Negative half: reading a directory fails with the same coded, positioned
// shape rather than an untagged error. Pins that the code is attached to
// the operation, not to one specific errno.
func TestIOReadDirectoryIsCodedAndPositioned(t *testing.T) {
	err := runIO(t, "import \"boru:io\"\nIO.read (make Pathon \"/tmp\")\n")
	if err == nil {
		t.Skip("reading a directory succeeded on this platform; nothing to pin")
	}
	ae := boruErrorWithCode(t, err, "read_error")
	if ae.Row == 0 {
		t.Errorf("directory-read error lost its position:\n%s", ae.Error())
	}
}
