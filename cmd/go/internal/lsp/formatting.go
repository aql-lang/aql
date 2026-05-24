// Formatting: formatter.Format(src) → single full-document TextEdit.

package lsp

import (
	"strings"
	"unicode/utf16"

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
// number of UTF-16 code units in the last line (the unit LSP
// Position.Character uses by default — counting bytes pushes the
// end position past EOL for any non-ASCII content and some clients
// reject the edit).
func wholeDocumentRange(src string) Range {
	lines := strings.Split(src, "\n")
	lastIdx := len(lines) - 1
	if lastIdx < 0 {
		lastIdx = 0
	}
	lastLen := 0
	if lastIdx < len(lines) {
		lastLen = utf16Len(lines[lastIdx])
	}
	return Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: lastIdx, Character: lastLen},
	}
}

// utf16Len returns the number of UTF-16 code units required to
// encode s — the default unit for LSP Position.Character. BMP
// runes take 1 unit; runes outside the BMP (e.g. most emoji) take
// 2 (a surrogate pair). Invalid runes (utf16.RuneLen == -1) are
// treated as one unit, matching how text encoders fall back to U+FFFD.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if utf16.RuneLen(r) == 2 {
			n += 2
		} else {
			n++
		}
	}
	return n
}
