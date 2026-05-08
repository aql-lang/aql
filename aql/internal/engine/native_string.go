package engine

import (
	"fmt"
	"strings"
	"unicode"
)

// stringNatives covers the string-manipulation words. Each entry uses
// the standard NativeFunc shape: forward-precedence words whose
// signatures fan out across [TString], [TString, TMap], etc.
//
// Most handlers are package-level named functions defined below; the
// trivial upper/lower handlers are produced by the unaryStringNative
// builder which returns a NativeFunc carrying both [TString] and
// [TAtom] signatures.
var stringNatives = []NativeFunc{
	unaryStringNative("upper", strings.ToUpper),
	unaryStringNative("lower", strings.ToLower),
	{
		Name:              "concat",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList, TMap}, Handler: concatOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TList}, Handler: concatHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "split",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: splitOptsHandler, Returns: []Type{TList}},
			{Args: []Type{TString, TString}, Handler: splitHandler, Returns: []Type{TList}},
		},
	},
	{
		Name:              "trim",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: trimOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: trimHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom, TMap}, Handler: trimOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom}, Handler: trimHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "contains",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: containsOptsHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TString, TString}, Handler: containsHandler, Returns: []Type{TBoolean}},
		},
	},
	{
		Name:              "indexof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: indexOfOptsHandler, Returns: []Type{TInteger}},
			{Args: []Type{TString, TString}, Handler: indexOfHandler, Returns: []Type{TInteger}},
		},
	},
	{
		Name:              "replace",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TString, TMap}, Handler: replaceOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString, TString, TString}, Handler: replaceHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "changecase",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: changeCaseOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: changeCaseHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom, TMap}, Handler: changeCaseOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom}, Handler: changeCaseHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "normalize",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: normalizeOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: normalizeHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "repeat",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TInteger, TMap}, Handler: repeatOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString, TInteger}, Handler: repeatHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "pad",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TInteger, TMap, TString}, Handler: padOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TInteger, TString}, Handler: padHandler, Returns: []Type{TString}},
		},
	},
	{
		Name:              "match",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: matchOptsHandler, Returns: []Type{TMap}},
			{Args: []Type{TString, TString}, Handler: matchHandler, Returns: []Type{TMap}},
		},
	},
	{
		Name:              "escape",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: escapeOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: escapeHandler, Returns: []Type{TString}},
		},
	},
}

// unaryStringNative builds a NativeFunc with two signatures —
// [TString] and [TAtom] — both routed through a handler that applies
// fn to the input string. Used for upper, lower, and similar
// transforms that share the same shape.
func unaryStringNative(name string, fn func(string) string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s, ok := args[0].Data.(string)
		if !ok {
			return nil, fmt.Errorf("%s: expected string, got %s", name, args[0].String())
		}
		return []Value{NewString(fn(s))}, nil
	}
	return NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString}, Handler: handler, Returns: []Type{TString}},
			{Args: []Type{TAtom}, Handler: handler, Returns: []Type{TString}},
		},
	}
}

// ---- concat ----

func concatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return doConcat(args[0], strOpts{})
}

func concatOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[1])
	return doConcat(args[0], opts)
}

func doConcat(listVal Value, o strOpts) ([]Value, error) {
	if listVal.Data == nil {
		return nil, fmt.Errorf("concat: argument must be a concrete list, got type literal")
	}
	elems := listVal.AsList()
	var parts []string
	for _, e := range elems.Slice() {
		if e.VType.Equal(TNone) {
			if o.skipNullish {
				continue
			}
			parts = append(parts, "")
			continue
		}
		s := ValToString(e)
		if s == "" && o.skipEmpty {
			continue
		}
		parts = append(parts, s)
	}
	result := strings.Join(parts, o.sep)
	return []Value{NewString(result)}, nil
}

// ---- split ----

func splitHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[0].AsConcreteString()
	_as0, _ := args[1].AsConcreteString()
	return doSplit(_as1, _as0, strOpts{cs: "sensitive", mode: "literal"})
}

func splitOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[2])
	_as3, _ := args[0].AsConcreteString()
	_as2, _ := args[1].AsConcreteString()
	return doSplit(_as3, _as2, opts)
}

func doSplit(input, sep string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	var parts []string

	if o.mode == "shell" {
		parts = splitByShell(input, sep, o.cs == "insensitive")
	} else {
		// literal split
		if o.cs == "insensitive" {
			parts = splitInsensitive(input, sep)
		} else {
			lim := -1
			if o.hasLim {
				lim = int(o.lim)
			}
			if lim > 0 {
				parts = strings.SplitN(input, sep, lim)
			} else {
				parts = strings.Split(input, sep)
			}
		}
	}

	// Apply lim for non-literal modes (literal already handled above)
	if o.hasLim && o.mode != "literal" {
		lim := int(o.lim)
		if lim > 0 && len(parts) > lim {
			// Join the excess parts back with the separator
			last := strings.Join(parts[lim-1:], sep)
			parts = append(parts[:lim-1], last)
		}
	}

	// Apply keepEmpty and trimParts
	var result []Value
	for _, p := range parts {
		if o.trimParts {
			p = strings.TrimSpace(p)
		}
		if !o.keepEmpty && p == "" {
			continue
		}
		result = append(result, NewString(p))
	}

	// If keepEmpty is true, include all parts (even empty ones)
	if o.keepEmpty {
		result = nil
		for _, p := range parts {
			if o.trimParts {
				p = strings.TrimSpace(p)
			}
			result = append(result, NewString(p))
		}
	}

	if result == nil {
		result = []Value{}
	}
	return []Value{NewList(result)}, nil
}

// splitInsensitive splits a string by separator case-insensitively.
func splitInsensitive(input, sep string) []string {
	if sep == "" {
		return strings.Split(input, "")
	}
	lower := strings.ToLower(input)
	lowerSep := strings.ToLower(sep)
	var parts []string
	start := 0
	for {
		idx := strings.Index(lower[start:], lowerSep)
		if idx < 0 {
			parts = append(parts, input[start:])
			break
		}
		parts = append(parts, input[start:start+idx])
		start += idx + len(sep)
	}
	return parts
}

// splitByShell splits using shell pattern matching to find separators.
func splitByShell(input, pattern string, caseInsensitive bool) []string {
	var parts []string
	remaining := input
	for {
		idx, length := shellFind(remaining, pattern, caseInsensitive)
		if idx < 0 || length == 0 {
			parts = append(parts, remaining)
			break
		}
		parts = append(parts, remaining[:idx])
		remaining = remaining[idx+length:]
	}
	return parts
}

// ---- trim ----

func trimHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return []Value{NewString(strings.TrimSpace(_as0))}, nil
}

func trimOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doTrim(_as1, opts)
}

func doTrim(input string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	chars := o.fill // chars field reused from fill via parseStrOpts
	if chars != "" {
		cutset := chars
		if o.cs == "insensitive" {
			// For case-insensitive char matching, include both cases in cutset
			cutset = strings.ToLower(chars) + strings.ToUpper(chars)
		}
		switch o.side {
		case "left":
			input = strings.TrimLeft(input, cutset)
		case "right":
			input = strings.TrimRight(input, cutset)
		default: // "both"
			input = strings.Trim(input, cutset)
		}
	} else {
		switch o.side {
		case "left":
			input = strings.TrimLeftFunc(input, unicode.IsSpace)
		case "right":
			input = strings.TrimRightFunc(input, unicode.IsSpace)
		default: // "both"
			input = strings.TrimSpace(input)
		}
	}

	return []Value{NewString(input)}, nil
}

// ---- contains ----

func containsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[0].AsConcreteString()
	_as0, _ := args[1].AsConcreteString()
	return doContains(_as1, _as0, strOpts{cs: "sensitive", mode: "literal"})
}

func containsOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[2])
	_as3, _ := args[0].AsConcreteString()
	_as2, _ := args[1].AsConcreteString()
	return doContains(_as3, _as2, opts)
}

func doContains(input, search string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"

	if o.mode == "shell" {
		// Shell pattern matching
		switch o.anchored {
		case "both", "true":
			return []Value{NewBoolean(shellMatch(search, input, ci))}, nil
		case "start":
			// Check if any prefix matches the pattern
			for i := 1; i <= len(input); i++ {
				if shellMatch(search, input[:i], ci) {
					return []Value{NewBoolean(true)}, nil
				}
			}
			return []Value{NewBoolean(false)}, nil
		case "end":
			// Check if any suffix matches the pattern
			for i := 0; i < len(input); i++ {
				if shellMatch(search, input[i:], ci) {
					return []Value{NewBoolean(true)}, nil
				}
			}
			return []Value{NewBoolean(false)}, nil
		default:
			idx, _ := shellFind(input, search, ci)
			return []Value{NewBoolean(idx >= 0)}, nil
		}
	}

	// Literal matching
	haystack := input
	needle := search
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	switch o.anchored {
	case "start":
		found := strings.HasPrefix(haystack, needle)
		if found && o.wholeWord {
			found = isWordBoundary(input, len(search))
		}
		return []Value{NewBoolean(found)}, nil
	case "end":
		found := strings.HasSuffix(haystack, needle)
		if found && o.wholeWord {
			found = isWordBoundary(input, len(input)-len(search))
		}
		return []Value{NewBoolean(found)}, nil
	case "both", "true":
		return []Value{NewBoolean(haystack == needle)}, nil
	default:
		if o.wholeWord {
			return []Value{NewBoolean(containsWholeWord(input, search, ci))}, nil
		}
		return []Value{NewBoolean(strings.Contains(haystack, needle))}, nil
	}
}

// containsWholeWord checks if search appears as a whole word in input.
func containsWholeWord(input, search string, ci bool) bool {
	haystack := input
	needle := search
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(needle)
		if isWordBoundary(input, absPos) && isWordBoundary(input, endPos) {
			return true
		}
		idx = absPos + 1
	}
}

// ---- indexof ----

func indexOfHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[0].AsConcreteString()
	_as0, _ := args[1].AsConcreteString()
	return doIndexOf(_as1, _as0, strOpts{cs: "sensitive", mode: "literal", occ: "first"})
}

func indexOfOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[2])
	_as3, _ := args[0].AsConcreteString()
	_as2, _ := args[1].AsConcreteString()
	return doIndexOf(_as3, _as2, opts)
}

func doIndexOf(input, search string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"
	from := 0
	if o.hasFrom {
		from = int(o.from)
		if from < 0 {
			from = 0
		}
		if from > len(input) {
			return []Value{NewInteger(-1)}, nil
		}
	}

	if o.mode == "shell" {
		if o.occ == "last" {
			return []Value{NewInteger(int64(shellFindLast(input, search, ci)))}, nil
		}
		idx, _ := shellFind(input[from:], search, ci)
		if idx >= 0 {
			idx += from
		}
		return []Value{NewInteger(int64(idx))}, nil
	}

	// Literal matching
	haystack := input
	needle := search
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	if o.occ == "last" {
		idx := strings.LastIndex(haystack, needle)
		return []Value{NewInteger(int64(idx))}, nil
	}

	idx := strings.Index(haystack[from:], needle)
	if idx >= 0 {
		idx += from
	}
	return []Value{NewInteger(int64(idx))}, nil
}

// shellFindLast finds the last occurrence of a shell pattern.
func shellFindLast(s, pattern string, caseInsensitive bool) int {
	matches := shellFindAll(s, pattern, caseInsensitive)
	if len(matches) == 0 {
		return -1
	}
	return matches[len(matches)-1][0]
}

// ---- replace ----

func replaceHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as2, _ := args[0].AsConcreteString()
	_as1, _ := args[1].AsConcreteString()
	_as0, _ := args[2].AsConcreteString()
	return doReplace(_as2, _as1, _as0,
		strOpts{cs: "sensitive", mode: "literal", scope: "first"})
}

func replaceOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[3])
	_as5, _ := args[0].AsConcreteString()
	_as4, _ := args[1].AsConcreteString()
	_as3, _ := args[2].AsConcreteString()
	return doReplace(_as5, _as4, _as3, opts)
}

func doReplace(input, search, repl string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"
	from := 0
	if o.hasFrom {
		from = int(o.from)
		if from < 0 {
			from = 0
		}
	}

	maxCount := -1
	if o.hasCount {
		maxCount = int(o.count)
	}

	if o.mode == "shell" {
		return []Value{NewString(replaceShell(input, search, repl, ci, o.scope, from, maxCount))}, nil
	}

	// Literal replacement
	if o.scope == "all" {
		if ci {
			return []Value{NewString(replaceAllInsensitive(input, search, repl, from, maxCount))}, nil
		}
		if maxCount >= 0 {
			return []Value{NewString(replaceN(input, search, repl, from, maxCount))}, nil
		}
		if from > 0 {
			prefix := input[:from]
			rest := strings.ReplaceAll(input[from:], search, repl)
			return []Value{NewString(prefix + rest)}, nil
		}
		return []Value{NewString(strings.ReplaceAll(input, search, repl))}, nil
	}

	// scope == "first" (default)
	if ci {
		return []Value{NewString(replaceFirstInsensitive(input, search, repl, from))}, nil
	}
	if from > 0 {
		idx := strings.Index(input[from:], search)
		if idx < 0 {
			return []Value{NewString(input)}, nil
		}
		absIdx := from + idx
		return []Value{NewString(input[:absIdx] + repl + input[absIdx+len(search):])}, nil
	}
	return []Value{NewString(strings.Replace(input, search, repl, 1))}, nil
}

func replaceFirstInsensitive(input, search, repl string, from int) string {
	lower := strings.ToLower(input)
	lowerSearch := strings.ToLower(search)
	idx := strings.Index(lower[from:], lowerSearch)
	if idx < 0 {
		return input
	}
	absIdx := from + idx
	return input[:absIdx] + repl + input[absIdx+len(search):]
}

func replaceAllInsensitive(input, search, repl string, from, maxCount int) string {
	lower := strings.ToLower(input)
	lowerSearch := strings.ToLower(search)
	var b strings.Builder
	b.WriteString(input[:from])
	remaining := input[from:]
	lowerRemaining := lower[from:]
	count := 0
	for {
		if maxCount >= 0 && count >= maxCount {
			b.WriteString(remaining)
			break
		}
		idx := strings.Index(lowerRemaining, lowerSearch)
		if idx < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:idx])
		b.WriteString(repl)
		remaining = remaining[idx+len(search):]
		lowerRemaining = lowerRemaining[idx+len(lowerSearch):]
		count++
	}
	return b.String()
}

func replaceN(input, search, repl string, from, maxCount int) string {
	var b strings.Builder
	b.WriteString(input[:from])
	remaining := input[from:]
	count := 0
	for {
		if maxCount >= 0 && count >= maxCount {
			b.WriteString(remaining)
			break
		}
		idx := strings.Index(remaining, search)
		if idx < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:idx])
		b.WriteString(repl)
		remaining = remaining[idx+len(search):]
		count++
	}
	return b.String()
}

func replaceShell(input, pattern, repl string, ci bool, scope string, from, maxCount int) string {
	matches := shellFindAll(input[from:], pattern, ci)
	if len(matches) == 0 {
		return input
	}

	// Adjust indices for the from offset
	for i := range matches {
		matches[i][0] += from
		matches[i][1] += from
	}

	if scope == "first" {
		matches = matches[:1]
	}
	if maxCount >= 0 && len(matches) > maxCount {
		matches = matches[:maxCount]
	}

	var b strings.Builder
	prev := 0
	for _, m := range matches {
		b.WriteString(input[prev:m[0]])
		b.WriteString(repl)
		prev = m[1]
	}
	b.WriteString(input[prev:])
	return b.String()
}

// ---- changecase ----

func changeCaseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doChangeCase(_as0, strOpts{style: "lower"})
}

func changeCaseOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[1])
	if opts.style == "" {
		opts.style = "lower"
	}
	_as1, _ := args[0].AsConcreteString()
	return doChangeCase(_as1, opts)
}

func doChangeCase(input string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	var result string
	switch o.style {
	case "upper":
		result = strings.ToUpper(input)
	case "capitalize":
		result = capitalize(input)
	case "title":
		result = titleCase(input)
	case "sentence":
		result = sentenceCase(input)
	case "fold":
		result = strings.ToLower(input) // fold approximation using toLower
	default: // "lower"
		result = strings.ToLower(input)
	}

	return []Value{NewString(result)}, nil
}

// capitalize uppercases the first character only.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// titleCase uppercases the first letter of each word.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(prev) || unicode.IsPunct(prev) {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return unicode.ToLower(r)
	}, s)
}

// sentenceCase lowercases everything, then uppercases the first letter.
func sentenceCase(s string) string {
	lower := strings.ToLower(s)
	return capitalize(lower)
}

// ---- normalize ----

func normalizeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doNormalize(_as0, strOpts{form: "NFC", eol: "preserve"})
}

func normalizeOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doNormalize(_as1, opts)
}

func doNormalize(input string, o strOpts) ([]Value, error) {
	// Apply Unicode normalization
	result := applyNorm(input, o.form)

	// Normalize line endings
	switch o.eol {
	case "lf":
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\r", "\n")
	case "crlf":
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\r", "\n")
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}

	// Collapse whitespace
	if o.collapseWs {
		var b strings.Builder
		prevWs := false
		for _, r := range result {
			if unicode.IsSpace(r) && r != '\n' && r != '\r' {
				if !prevWs {
					b.WriteRune(' ')
					prevWs = true
				}
			} else {
				b.WriteRune(r)
				prevWs = false
			}
		}
		result = b.String()
	}

	// Trim surrounding whitespace
	if o.trim {
		result = strings.TrimSpace(result)
	}

	return []Value{NewString(result)}, nil
}

// ---- repeat ----

func repeatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[0].AsConcreteString()
	_as0, _ := args[1].AsConcreteInteger()
	return doRepeat(_as1, _as0, strOpts{})
}

func repeatOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[2])
	_as3, _ := args[0].AsConcreteString()
	_as2, _ := args[1].AsConcreteInteger()
	return doRepeat(_as3, _as2, opts)
}

func doRepeat(input string, count int64, o strOpts) ([]Value, error) {
	if count < 0 {
		return nil, fmt.Errorf("repeat: count must be non-negative, got %d", count)
	}

	if !o.hasSep || o.sep == "" {
		return []Value{NewString(strings.Repeat(input, int(count)))}, nil
	}

	// With separator: join count copies with sep
	if count == 0 {
		return []Value{NewString("")}, nil
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = input
	}
	return []Value{NewString(strings.Join(parts, o.sep))}, nil
}

// ---- pad ----

// pad: forward-first: args[0]=width (forward), args[1]=string (stack).
// Usage: "ab" pad 5 → "ab   "
func padHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[1].AsConcreteString()
	_as0, _ := args[0].AsConcreteInteger()
	return doPad(_as1, _as0, strOpts{side: "right", fill: " "})
}

// pad: forward-first: args[0]=width (forward), args[1]=opts (forward), args[2]=string (stack).
// Usage: "ab" pad 5 {side:"left" fill:"0"} → "000ab"
func padOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("pad: options must be a concrete map, got type literal")
	}
	opts := parseStrOpts(args[1])
	if opts.fill == "" {
		opts.fill = " "
	}
	// Default side for pad is "right", not "both" from parseStrOpts.
	if m := args[1].AsMap(); m != nil {
		if _, ok := m.Get("side"); !ok {
			opts.side = "right"
		}
	}
	_as3, _ := args[2].AsConcreteString()
	_as2, _ := args[0].AsConcreteInteger()
	return doPad(_as3, _as2, opts)
}

func doPad(input string, targetLen int64, o strOpts) ([]Value, error) {
	current := len(input)
	target := int(targetLen)

	if current >= target {
		if o.trunc {
			return []Value{NewString(input[:target])}, nil
		}
		return []Value{NewString(input)}, nil
	}

	needed := target - current
	fill := o.fill
	if fill == "" {
		fill = " "
	}

	// Generate enough fill characters
	padding := strings.Repeat(fill, (needed/len(fill))+1)

	switch o.side {
	case "left":
		result := padding[:needed] + input
		return []Value{NewString(result)}, nil
	case "both":
		left := needed / 2
		right := needed - left
		result := padding[:left] + input + padding[:right]
		return []Value{NewString(result)}, nil
	default: // "right"
		result := input + padding[:needed]
		return []Value{NewString(result)}, nil
	}
}

// ---- match ----

func matchHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[0].AsConcreteString()
	_as0, _ := args[1].AsConcreteString()
	return doMatch(_as1, _as0,
		strOpts{cs: "sensitive", mode: "literal", scope: "first"})
}

func matchOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[2])
	_as3, _ := args[0].AsConcreteString()
	_as2, _ := args[1].AsConcreteString()
	return doMatch(_as3, _as2, opts)
}

func doMatch(input, pattern string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		pattern = applyNorm(pattern, o.normForm)
	}

	ci := o.cs == "insensitive"

	var matches []matchEntry

	if o.mode == "shell" {
		matches = findShellMatches(input, pattern, ci, o.scope == "all")
	} else {
		matches = findLiteralMatches(input, pattern, ci, o.scope == "all")
	}

	return []Value{buildMatchResult(matches)}, nil
}

type matchEntry struct {
	m string
	i int
	e int
}

func findLiteralMatches(input, pattern string, ci bool, all bool) []matchEntry {
	haystack := input
	needle := pattern
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	var matches []matchEntry
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			break
		}
		absIdx := start + idx
		end := absIdx + len(pattern)
		matches = append(matches, matchEntry{
			m: input[absIdx:end],
			i: absIdx,
			e: end,
		})
		if !all {
			break
		}
		start = absIdx + 1
		if start >= len(haystack) {
			break
		}
	}
	return matches
}

func findShellMatches(input, pattern string, ci bool, all bool) []matchEntry {
	results := shellFindAll(input, pattern, ci)
	var matches []matchEntry
	for _, r := range results {
		matches = append(matches, matchEntry{
			m: input[r[0]:r[1]],
			i: r[0],
			e: r[1],
		})
		if !all {
			break
		}
	}
	return matches
}

func buildMatchResult(matches []matchEntry) Value {
	result := NewOrderedMap()

	ok := len(matches) > 0
	result.Set("ok", NewBoolean(ok))

	// Build ms array
	msElems := make([]Value, len(matches))
	for i, m := range matches {
		entry := NewOrderedMap()
		entry.Set("m", NewString(m.m))
		entry.Set("i", NewInteger(int64(m.i)))
		entry.Set("e", NewInteger(int64(m.e)))
		msElems[i] = NewMap(entry)
	}
	result.Set("ms", NewList(msElems))

	// Set fst and lst convenience views
	if len(matches) > 0 {
		fst := NewOrderedMap()
		fst.Set("m", NewString(matches[0].m))
		fst.Set("i", NewInteger(int64(matches[0].i)))
		fst.Set("e", NewInteger(int64(matches[0].e)))
		result.Set("fst", NewMap(fst))

		last := matches[len(matches)-1]
		lst := NewOrderedMap()
		lst.Set("m", NewString(last.m))
		lst.Set("i", NewInteger(int64(last.i)))
		lst.Set("e", NewInteger(int64(last.e)))
		result.Set("lst", NewMap(lst))
	}

	result.Set("n", NewInteger(int64(len(matches))))

	return NewMap(result)
}

// ---- escape ----

func escapeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doEscape(_as0, strOpts{tgt: "sh", quote: "none"})
}

func escapeOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doEscape(_as1, opts)
}

func doEscape(input string, o strOpts) ([]Value, error) {
	var result string

	switch o.tgt {
	case "bash":
		result = escapeBash(input)
	case "sed":
		result = escapeSed(input)
	case "awk":
		result = escapeAwk(input)
	case "grep":
		result = escapeGrep(input)
	default: // "sh"
		result = escapeSh(input)
	}

	// Apply quoting
	switch o.quote {
	case "single":
		result = "'" + result + "'"
	case "double":
		result = "\"" + result + "\""
	}

	return []Value{NewString(result)}, nil
}

// escapeSh escapes for POSIX sh: backslash-escape shell metacharacters.
func escapeSh(s string) string {
	meta := `\'"` + "`$!#&|;(){}[]<>?*~ \t\n"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeBash escapes for bash: similar to sh but includes additional chars.
func escapeBash(s string) string {
	return escapeSh(s) // same treatment for simplicity
}

// escapeSed escapes for sed regex: escape BRE metacharacters.
func escapeSed(s string) string {
	meta := `\/.^$*[]`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeAwk escapes for awk regex: escape ERE metacharacters.
func escapeAwk(s string) string {
	meta := `\/.^$*+?()[]{}|`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeGrep escapes for grep BRE: escape BRE metacharacters.
func escapeGrep(s string) string {
	meta := `\/.^$*[]`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
