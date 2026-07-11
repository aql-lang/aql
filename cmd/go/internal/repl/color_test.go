package repl

import (
	"errors"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// The REPL's error rendering: a structured AqlError gets the ANSI
// palette only when color is resolved on; plain errors and the
// color-off path keep the historical text (never ANSI).
func TestRenderREPLError(t *testing.T) {
	ae := native.MakeAqlError("signature_error", "boom detail", "w", "w x", "")
	if got := renderREPLError(ae, false); strings.Contains(got, "\x1b[") {
		t.Errorf("color-off rendering must be plain: %q", got)
	}
	if got := renderREPLError(ae, true); !strings.Contains(got, "\x1b[") {
		t.Errorf("color-on rendering must carry ANSI: %q", got)
	}
	plain := errors.New("plain failure")
	if got := renderREPLError(plain, true); got != "plain failure" {
		t.Errorf("non-AqlError must keep its text even with color on: %q", got)
	}
}
