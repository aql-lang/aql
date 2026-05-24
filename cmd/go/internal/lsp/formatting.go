// Formatting: formatter.Format(src) → single full-document TextEdit.

package lsp

import (
	"strings"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// buildFormattingEdits returns the TextEdit slice for textDocument/
// formatting. We always emit a single TextEdit that replaces the
// whole document; if the formatter didn't change anything, an empty
// slice is returned so the editor doesn't mark the file dirty.
func (s *server) buildFormattingEdits(src string) []TextEdit {
	formatted := formatter.Format(src)
	if formatted == src {
		return []TextEdit{}
	}
	return []TextEdit{{
		Range:   wholeDocumentRange(src),
		NewText: formatted,
	}}
}

// wholeDocumentRange computes the LSP Range covering [start, end)
// of the full document. Lines are 0-based; the end column is the
// length of the last line.
func wholeDocumentRange(src string) Range {
	lines := strings.Split(src, "\n")
	lastIdx := len(lines) - 1
	if lastIdx < 0 {
		lastIdx = 0
	}
	lastLen := 0
	if lastIdx < len(lines) {
		lastLen = len(lines[lastIdx])
	}
	return Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: lastIdx, Character: lastLen},
	}
}
