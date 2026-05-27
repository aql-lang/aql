// Hover: word-at-position → help.FormatDynamic / help.Format.

package lsp

import (
	"strings"

	"github.com/aql-lang/aql/lang/go/native"
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

// buildHover returns the Hover response for the word under pos in
// src, or nil if no word is at that position. The lookup mirrors
// `aql help <word>`: prefer dynamic registry info, fall back to
// static help, give up if neither has the word.
func (s *server) buildHover(src string, pos Position) *Hover {
	word, wordRange, ok := wordAt(src, pos)
	if !ok {
		return nil
	}

	var body string
	if reg := s.ensureRegistry(); reg != nil {
		if info := native.BuildFuncInfo(reg, word); info != nil {
			body = helppkg.FormatDynamic(*info)
		}
	}
	if body == "" {
		if entry := helppkg.Lookup(word); entry != nil {
			body = helppkg.Format(entry)
		}
	}
	if body == "" {
		return nil
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  "plaintext",
			Value: body,
		},
		Range: &wordRange,
	}
}

// wordAt locates the AQL "word" (run of identifier chars) covering
// the given LSP Position in src. Returns the word, its range, and
// true on a hit. AQL identifiers can contain letters, digits, and
// the characters typical of AQL words (e.g. `.`, `_`, `-`); we
// adopt a permissive isWordChar so dotted names like "Color.hex2rgb"
// hover as one unit.
func wordAt(src string, pos Position) (string, Range, bool) {
	// Find line start by counting newlines.
	lineStart := 0
	curLine := 0
	for i := 0; i < len(src) && curLine < pos.Line; i++ {
		if src[i] == '\n' {
			curLine++
			lineStart = i + 1
		}
	}
	if curLine != pos.Line {
		return "", Range{}, false
	}

	// Extract the line.
	lineEnd := lineStart
	for lineEnd < len(src) && src[lineEnd] != '\n' {
		lineEnd++
	}
	line := src[lineStart:lineEnd]

	if pos.Character < 0 || pos.Character > len(line) {
		return "", Range{}, false
	}

	// Expand left and right from the cursor over word characters.
	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	end := pos.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	if start == end {
		return "", Range{}, false
	}
	word := strings.TrimSpace(line[start:end])
	if word == "" {
		return "", Range{}, false
	}
	return word, Range{
		Start: Position{Line: pos.Line, Character: start},
		End:   Position{Line: pos.Line, Character: end},
	}, true
}

// isWordChar reports whether c is part of an AQL word. We accept
// the union of identifier chars across the languages: ASCII
// alphanumerics, '_', '-', and '.' (for namespaced words like
// "Color.hex2rgb"). The set is deliberately a superset — the
// downstream help lookup will return nothing if the resulting
// string isn't actually a registered word, so over-grabbing is
// harmless.
func isWordChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '-' || c == '.':
		return true
	}
	return false
}
