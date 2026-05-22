package eng

import (
	"strconv"
	"strings"
)

// AqlError is the structured error type for AQL engine errors.
// It mirrors the jsonic JsonicError format, providing error codes,
// source location, source extracts, and detailed descriptions.
//
// Format:
//
//	[aql/<code>]: <detail>
//	  --> <row>:<col>
//	  <line> | <source>
//	           ^^^^ <detail>
type AqlError struct {
	Code   string // Error code: "signature_error", "type_error", "syntax_error", etc.
	Detail string // Human-readable detail message
	Row    int    // 1-based line number (0 = unknown)
	Col    int    // 1-based column number (0 = unknown)
	Src    string // Source fragment at the error (the token/word text)
	Hint   string // Additional explanatory text
	// fullSource is the complete source text for generating context extracts.
	fullSource string
}

// TODO: this should use jsonic error formatting

// Error implements the error interface with jsonic-style formatting.
func (e *AqlError) Error() string {
	var b strings.Builder

	// Line 1: [aql/<code>]: <detail>
	b.WriteString("[aql/")
	b.WriteString(e.Code)
	b.WriteString("]: ")
	b.WriteString(e.Detail)

	// Line 2: --> <row>:<col>
	if e.Row > 0 {
		b.WriteString("\n  --> ")
		b.WriteString(strconv.Itoa(e.Row))
		b.WriteString(":")
		if e.Col > 0 {
			b.WriteString(strconv.Itoa(e.Col))
		} else {
			b.WriteString("1")
		}
	}

	// Source site extract
	if e.fullSource != "" && e.Row > 0 {
		site := aqlErrSite(e.fullSource, e.Src, e.Detail, e.Row, e.Col)
		if site != "" {
			b.WriteString("\n")
			b.WriteString(site)
		}
	}

	// Hint
	if e.Hint != "" {
		b.WriteString("\n  = ")
		b.WriteString(e.Hint)
	}

	return b.String()
}

// aqlErrSite generates a source code extract showing the error location,
// matching the jsonic errsite() output format.
func aqlErrSite(src, sub, msg string, row, col int) string {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}

	lines := strings.Split(src, "\n")

	// row is 1-based, convert to 0-based index
	lineIdx := row - 1
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}

	// Determine padding width based on largest line number shown
	maxLineNum := row + 2
	pad := len(strconv.Itoa(maxLineNum)) + 2

	// Build context lines: 2 before, error line, caret line, 2 after
	var result []string

	ln := func(num int, text string) string {
		numStr := strconv.Itoa(num)
		return strings.Repeat(" ", pad-len(numStr)) + numStr + " | " + text
	}

	// 2 lines before
	if lineIdx-2 >= 0 {
		result = append(result, ln(row-2, lines[lineIdx-2]))
	}
	if lineIdx-1 >= 0 {
		result = append(result, ln(row-1, lines[lineIdx-1]))
	}

	// Error line
	if lineIdx >= 0 && lineIdx < len(lines) {
		result = append(result, ln(row, lines[lineIdx]))
	}

	// Caret line
	caretCount := len(sub)
	if caretCount < 1 {
		caretCount = 1
	}
	indent := strings.Repeat(" ", pad) + "   " + strings.Repeat(" ", col-1)
	result = append(result, indent+strings.Repeat("^", caretCount)+" "+msg)

	// 2 lines after
	if lineIdx+1 < len(lines) {
		result = append(result, ln(row+1, lines[lineIdx+1]))
	}
	if lineIdx+2 < len(lines) {
		result = append(result, ln(row+2, lines[lineIdx+2]))
	}

	return strings.Join(result, "\n")
}

// SrcPos holds source position information for a value.
// Embedded in Value to enable error messages with source extracts.
type SrcPos struct {
	Row int    // 1-based line number (0 = unknown)
	Col int    // 1-based column number (0 = unknown)
	Src string // source text of the token
}

// FindWordInSource locates the last occurrence of a word in the source
// text and returns 1-based row and col. Searching from the end is more
// likely to find the call site rather than the definition site (e.g.
// "def f ..." vs "f"). If the word is not found, returns (0, 0).
//
// Exported for use by the static type-checker when filling in Row/Col
// on diagnostics whose Pos is unknown.
func FindWordInSource(src, word string) (row int, col int) {
	return findWordInSource(src, word)
}

func findWordInSource(src, word string) (row int, col int) {
	if src == "" || word == "" {
		return 0, 0
	}
	lines := strings.Split(src, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		idx := strings.Index(lines[i], word)
		if idx >= 0 {
			return i + 1, idx + 1
		}
	}
	return 0, 0
}

// MakeAqlError creates an AqlError with source location from the given
// source text and word, searching for the word in the source to determine
// the error position.
func MakeAqlError(code, detail, word, fullSource, hint string) *AqlError {
	return makeAqlError(code, detail, word, fullSource, hint)
}

// makeAqlError creates an AqlError with source location from the given
// source text and word.
func makeAqlError(code, detail, word, fullSource, hint string) *AqlError {
	row, col := findWordInSource(fullSource, word)
	return &AqlError{
		Code:       code,
		Detail:     detail,
		Row:        row,
		Col:        col,
		Src:        word,
		Hint:       hint,
		fullSource: fullSource,
	}
}

// describeStackTypes returns a human-readable description of the types
// on the stack around a given position, for inclusion in error messages.
func describeStackTypes(stack []Value, pointer int) string {
	if len(stack) == 0 {
		return "stack is empty"
	}
	// Show types of up to 3 values before and after the pointer.
	var parts []string
	start := pointer - 3
	if start < 0 {
		start = 0
	}
	end := pointer + 4
	if end > len(stack) {
		end = len(stack)
	}
	for i := start; i < end; i++ {
		v := stack[i]
		label := v.Parent.String()
		if IsWord(v) {
			w, _ := AsWord(v)
			label = "word(" + w.Name + ")"
		} else if IsAtom(v) {
			a, _ := AsAtom(v)
			label = "atom(" + a + ")"
		} else if s := renderDepScalar(v); s != "" {
			// Render the constraint payload rather than falling
			// into a Matches(TString)/AsString path that would
			// silently produce an empty label.
			label = s
		} else if v.Parent.Matches(TString) {
			s, _ := AsString(v)
			if len(s) > 20 {
				s = s[:20] + "..."
			}
			label = "'" + s + "'"
		} else if v.Parent.Matches(TInteger) {
			n, _ := AsInteger(v)
			label = strconv.FormatInt(n, 10)
		} else if v.Parent.Matches(TDecimal) {
			f, _ := AsDecimal(v)
			label = formatDecimal(f)
		}
		if i == pointer {
			label = ">>>" + label + "<<<"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

// describeSigArgs returns a human-readable description of a signature's
// expected argument types.
func describeSigArgs(sig *Signature) string {
	if sig == nil || len(sig.Args) == 0 {
		return "(no args)"
	}
	parts := make([]string, len(sig.Args))
	for i, t := range sig.Args {
		parts[i] = t.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// describeAllSigs returns a summary of all available signatures for a word.
func describeAllSigs(fn *FnDefInfo) string {
	if fn == nil || len(fn.Signatures) == 0 {
		return ""
	}
	var parts []string
	for _, sig := range fn.Signatures {
		parts = append(parts, describeSigArgs(&sig))
	}
	return strings.Join(parts, " or ")
}
