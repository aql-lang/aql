package engine

import "strings"

func registerMatch(r *Registry) {
	// match: [string, string] -> [map]
	matchHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsString()
		_as0, _ := args[1].AsString()
		return doMatch(_as1, _as0,
			strOpts{cs: "sensitive", mode: "literal", scope: "first"})
	}

	// match: [string, string, map] -> [map]
	matchOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[2])
		_as3, _ := args[0].AsString()
		_as2, _ := args[1].AsString()
		return doMatch(_as3, _as2, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "match",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: matchOptsHandler, Returns: []Type{TMap}},
			{Args: []Type{TString, TString}, Handler: matchHandler, Returns: []Type{TMap}},
		},
	})
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
