package lsp

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// Notes and suggestions ride the LSP Message as extra lines; a
// diagnostic without them keeps the bare Detail (no trailing noise).
func TestToLSPDiagnosticNotesAndSuggestions(t *testing.T) {
	d := lang.CheckDiagnostic{
		Row: 1, Col: 1, Code: "undefined_word", Word: "uppr",
		Detail:      "undefined word: uppr",
		Notes:       []string{"a note"},
		Suggestions: []lang.DiagSuggestion{{Message: "did you mean `upper`?"}},
	}
	out := toLSPDiagnostic(d)
	if !strings.Contains(out.Message, "undefined word: uppr\nnote: a note\nhelp: did you mean `upper`?") {
		t.Errorf("message = %q, want detail + note + help lines", out.Message)
	}
	bare := toLSPDiagnostic(lang.CheckDiagnostic{Row: 1, Col: 1, Code: "x", Detail: "just detail"})
	if bare.Message != "just detail" {
		t.Errorf("bare message = %q, want unchanged detail", bare.Message)
	}
}
