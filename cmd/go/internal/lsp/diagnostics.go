// Diagnostics: lang.Check → []lsp.Diagnostic → publishDiagnostics.

package lsp

import (
	"fmt"

	lang "github.com/aql-lang/aql/lang/go"
)

// publishDiagnostics runs the static checker over the cached buffer
// and emits a textDocument/publishDiagnostics notification with the
// translated findings.
func (s *server) publishDiagnostics(uri string) {
	src, ok := s.buffers[uri]
	if !ok {
		return
	}

	diags := s.computeDiagnostics(src)

	// Always publish (even an empty list) so the editor clears any
	// previously reported diagnostics for the URI.
	if err := s.conn.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	}); err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: publishDiagnostics: %s\n", err)
	}
}

// computeDiagnostics runs lang.Check on src and converts each
// CheckDiagnostic to an LSP Diagnostic.
func (s *server) computeDiagnostics(src string) []Diagnostic {
	a, err := lang.New(lang.Options{})
	if err != nil {
		return []Diagnostic{{
			Range:    Range{},
			Severity: severityError,
			Code:     "aql/init",
			Source:   "aql",
			Message:  err.Error(),
		}}
	}

	res, _ := a.Check(src)
	out := make([]Diagnostic, 0, len(res.Diagnostics))
	for _, d := range res.Diagnostics {
		out = append(out, toLSPDiagnostic(d))
	}
	return out
}

// toLSPDiagnostic translates an AQL CheckDiagnostic to LSP shape.
// AQL Row/Col are 1-based; LSP is 0-based. The diagnostic range
// covers the offending word; when Word is empty, fall back to a
// single-character range at (Row, Col) so the editor still has
// somewhere to draw a marker.
func toLSPDiagnostic(d lang.CheckDiagnostic) Diagnostic {
	row := d.Row - 1
	col := d.Col - 1
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	endCol := col + len(d.Word)
	if d.Word == "" {
		endCol = col + 1
	}

	sev := severityInformation
	switch d.Severity {
	case lang.SeverityError:
		sev = severityError
	case lang.SeverityWarning:
		sev = severityWarning
	}

	return Diagnostic{
		Range: Range{
			Start: Position{Line: row, Character: col},
			End:   Position{Line: row, Character: endCol},
		},
		Severity: sev,
		Code:     d.Code,
		Source:   "aql",
		Message:  d.Detail,
	}
}
