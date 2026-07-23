package native

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// strOpts holds common option fields parsed from an AQL options map.
type strOpts struct {
	u         bool   // Unicode-aware behavior
	normForm  string // "", "NFC", "NFD", "NFKC", "NFKD"
	cs        string // "sensitive" or "insensitive"
	mode      string // "literal" or "shell"
	side      string // "left", "right", "both"
	sep       string // separator
	hasSep    bool   // whether sep was explicitly set
	fill      string // fill character for pad
	style     string // case style
	loc       string // locale hint
	unit      string // "code-unit", "code-point", "grapheme"
	tgt       string // escape target
	quote     string // escape quote style
	scope     string // "first" or "all"
	from      int64  // start index
	hasFrom   bool
	count     int64 // max replacements
	hasCount  bool
	lim       int64 // max parts for split
	hasLim    bool
	occ       string // "first" or "last"
	groups    string // "named", "numbered", "both", or "" for bool
	groupBool bool   // if groups was just true/false

	// boolean flags
	skipEmpty   bool
	skipNullish bool
	keepEmpty   bool
	trimParts   bool
	wholeWord   bool
	anchored    string // "", "start", "end", "both" or "true"
	fromEnd     bool
	trunc       bool
	litRepl     bool

	// normalize-specific
	trim       bool
	collapseWs bool
	eol        string // "preserve", "lf", "crlf"

	// form for normalize function
	form string
}

// parseStrOpts extracts common string options from an AQL map value.
func parseStrOpts(v Value) strOpts {
	var o strOpts
	o.cs = "sensitive"
	o.mode = "literal"
	o.side = "both"
	o.unit = "code-unit"
	o.scope = "first"
	o.occ = "first"
	o.eol = "preserve"
	o.form = "NFC"
	o.quote = "none"

	if !v.Parent.Equal(TMap) || !IsConcrete(v) {
		return o
	}
	m, _ := AsMap(v)

	if b, ok := MapFieldBoolean(m, "u"); ok {
		o.u = b
	}
	if val, ok := m.Get("norm"); ok {
		if val.Parent.ConformsTo(TBoolean) {
			_as1, _ := AsBoolean(val)
			if _as1 {
				o.normForm = "NFC"
			}
		} else if val.Parent.ConformsTo(TString) || IsAtom(val) {
			o.normForm = strings.ToUpper(ValToString(val))
		}
	}
	if val, ok := m.Get("cs"); ok {
		o.cs = ValToString(val)
	}
	if val, ok := m.Get("mode"); ok {
		o.mode = ValToString(val)
	}
	if val, ok := m.Get("side"); ok {
		o.side = ValToString(val)
	}
	if val, ok := m.Get("sep"); ok {
		o.sep = ValToString(val)
		o.hasSep = true
	}
	if val, ok := m.Get("fill"); ok {
		o.fill = ValToString(val)
	}
	if val, ok := m.Get("style"); ok {
		o.style = ValToString(val)
	}
	if val, ok := m.Get("loc"); ok {
		o.loc = ValToString(val)
	}
	if val, ok := m.Get("unit"); ok {
		o.unit = ValToString(val)
	}
	if val, ok := m.Get("tgt"); ok {
		o.tgt = ValToString(val)
	}
	if val, ok := m.Get("quote"); ok {
		o.quote = ValToString(val)
	}
	if val, ok := m.Get("scope"); ok {
		o.scope = ValToString(val)
	}
	if val, ok := m.Get("occ"); ok {
		o.occ = ValToString(val)
	}
	if n, ok := MapFieldInteger(m, "from"); ok {
		o.from = n
		o.hasFrom = true
	}
	if n, ok := MapFieldInteger(m, "count"); ok {
		o.count = n
		o.hasCount = true
	}
	if n, ok := MapFieldInteger(m, "lim"); ok {
		o.lim = n
		o.hasLim = true
	}

	// boolean flags
	if b, ok := MapFieldBoolean(m, "skipEmpty"); ok {
		o.skipEmpty = b
	}
	if b, ok := MapFieldBoolean(m, "skipNullish"); ok {
		o.skipNullish = b
	}
	if b, ok := MapFieldBoolean(m, "keepEmpty"); ok {
		o.keepEmpty = b
	}
	if b, ok := MapFieldBoolean(m, "trimParts"); ok {
		o.trimParts = b
	}
	if b, ok := MapFieldBoolean(m, "wholeWord"); ok {
		o.wholeWord = b
	}
	if val, ok := m.Get("anchored"); ok {
		if val.Parent.ConformsTo(TBoolean) {
			_as10, _ := AsBoolean(val)
			if _as10 {
				o.anchored = "both"
			}
		} else {
			o.anchored = ValToString(val)
		}
	}
	if b, ok := MapFieldBoolean(m, "fromEnd"); ok {
		o.fromEnd = b
	}
	if b, ok := MapFieldBoolean(m, "trunc"); ok {
		o.trunc = b
	}
	if b, ok := MapFieldBoolean(m, "litRepl"); ok {
		o.litRepl = b
	}

	// normalize-specific
	if b, ok := MapFieldBoolean(m, "trim"); ok {
		o.trim = b
	}
	if b, ok := MapFieldBoolean(m, "collapseWs"); ok {
		o.collapseWs = b
	}
	if val, ok := m.Get("eol"); ok {
		o.eol = ValToString(val)
	}
	if val, ok := m.Get("form"); ok {
		o.form = strings.ToUpper(ValToString(val))
	}

	// groups for match
	if val, ok := m.Get("groups"); ok {
		if val.Parent.ConformsTo(TBoolean) {
			_as16, _ := AsBoolean(val)
			o.groupBool = _as16
		} else {
			o.groups = ValToString(val)
		}
	}

	// chars for trim
	if val, ok := m.Get("chars"); ok {
		o.fill = ValToString(val) // reuse fill field for trim chars
	}

	return o
}

// applyNorm normalizes input string if normForm is set.
func applyNorm(s string, normForm string) string {
	switch normForm {
	case "NFC":
		return norm.NFC.String(s)
	case "NFD":
		return norm.NFD.String(s)
	case "NFKC":
		return norm.NFKC.String(s)
	case "NFKD":
		return norm.NFKD.String(s)
	default:
		return s
	}
}

// shellMatch performs shell-style glob matching using filepath.Match.
// The pattern uses *, ?, and [...] wildcards.
func shellMatch(pattern, s string, caseInsensitive bool) bool {
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		s = strings.ToLower(s)
	}
	matched, _ := filepath.Match(pattern, s)
	return matched
}

// runeBounds returns every rune-boundary byte offset of s, including
// the trailing len(s). Shell candidates are cut at these boundaries so
// every reported offset is a valid position in the ORIGINAL string.
func runeBounds(s string) []int {
	b := make([]int, 0, len(s)+1)
	for i := range s {
		b = append(b, i)
	}
	return append(b, len(s))
}

// foldShell lowercases per-rune (simple fold) for shell matching. It is
// applied to the PATTERN and to each candidate SUBSTRING COPY — never
// to the searched string itself — so reported offsets always index the
// original string. Lowering a whole copy and searching it returned
// offsets into the COPY, which drift whenever folding changes a rune's
// byte width (İ U+0130 is 2 bytes, its lowercase i is 1 — flagged in
// PR #306 review).
func foldShell(s string) string {
	return strings.Map(unicode.ToLower, s)
}

// shellFind finds the first occurrence of a shell pattern in a string
// by trying the pattern against every rune-boundary substring.
// Returns (byte index, byte length) into the ORIGINAL string, or
// (-1, 0) if not found.
func shellFind(s, pattern string, caseInsensitive bool) (int, int) {
	pat := pattern
	if caseInsensitive {
		pat = foldShell(pat)
	}
	bounds := runeBounds(s)
	for bi := 0; bi < len(bounds)-1; bi++ {
		for bj := bi + 1; bj < len(bounds); bj++ {
			sub := s[bounds[bi]:bounds[bj]]
			if caseInsensitive {
				sub = foldShell(sub)
			}
			if matched, _ := filepath.Match(pat, sub); matched {
				return bounds[bi], bounds[bj] - bounds[bi]
			}
		}
	}
	return -1, 0
}

// shellFindAll finds all non-overlapping occurrences of a shell
// pattern, as (start, end) byte-offset pairs into the ORIGINAL string.
func shellFindAll(s, pattern string, caseInsensitive bool) [][2]int {
	pat := pattern
	if caseInsensitive {
		pat = foldShell(pat)
	}
	bounds := runeBounds(s)
	var results [][2]int
	pos := 0
	for pos < len(bounds)-1 {
		found := false
		for i := pos; i < len(bounds)-1 && !found; i++ {
			for j := i + 1; j < len(bounds); j++ {
				sub := s[bounds[i]:bounds[j]]
				if caseInsensitive {
					sub = foldShell(sub)
				}
				if matched, _ := filepath.Match(pat, sub); matched {
					results = append(results, [2]int{bounds[i], bounds[j]})
					pos = j
					found = true
					break
				}
			}
			if !found {
				pos = i + 1
			}
		}
		if !found {
			break
		}
	}
	return results
}

// isWordBoundary checks if position i in string s is at a word boundary.
func isWordBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	next, _ := utf8.DecodeRuneInString(s[i:])
	prevWord := unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_'
	nextWord := unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_'
	return prevWord != nextWord
}
