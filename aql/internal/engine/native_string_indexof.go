package engine

import "strings"

func registerIndexOf(r *Registry) {
	// indexof: [string, string] -> [integer]
	indexOfHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return doIndexOf(args[0].AsString(), args[1].AsString(), strOpts{cs: "sensitive", mode: "literal", occ: "first"})
	}

	// indexof: [string, string, map] -> [integer]
	indexOfOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[2])
		return doIndexOf(args[0].AsString(), args[1].AsString(), opts)
	}

	r.Register("indexof",
		Signature{Args: []Type{TString, TString, TMap}, Handler: indexOfOptsHandler},
		Signature{Args: []Type{TString, TString}, Handler: indexOfHandler},
	)
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
