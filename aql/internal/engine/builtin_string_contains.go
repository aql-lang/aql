package engine

import "strings"

func registerContains(r *Registry) {
	// contains: [string, string] -> [boolean]
	containsHandler := func(args []Value) ([]Value, error) {
		return doContains(args[0].AsString(), args[1].AsString(), strOpts{cs: "sensitive", mode: "literal"})
	}

	// contains: [string, string, map] -> [boolean]
	containsOptsHandler := func(args []Value) ([]Value, error) {
		opts := parseStrOpts(args[2])
		return doContains(args[0].AsString(), args[1].AsString(), opts)
	}

	r.Register("contains",
		Signature{Args: []Type{TString, TString, TMap}, Handler: containsOptsHandler},
		Signature{Args: []Type{TString, TString}, Handler: containsHandler},
	)
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
