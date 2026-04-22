package engine

import "strings"

func RegisterContains(r *Registry) {
	// contains: [string, string] -> [boolean]
	containsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsString()
		_as0, _ := args[1].AsString()
		return doContains(_as1, _as0, strOpts{cs: "sensitive", mode: "literal"})
	}

	// contains: [string, string, map] -> [boolean]
	containsOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[2])
		_as3, _ := args[0].AsString()
		_as2, _ := args[1].AsString()
		return doContains(_as3, _as2, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "contains",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TMap}, Handler: containsOptsHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TString, TString}, Handler: containsHandler, Returns: []Type{TBoolean}},
		},
	})
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
