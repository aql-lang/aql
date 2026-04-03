package engine

import "strings"

func registerSplit(r *Registry) {
	// split: [string, string] -> [list]
	splitHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsString()
		_as0, _ := args[1].AsString()
		return doSplit(_as1, _as0, strOpts{cs: "sensitive", mode: "literal"})
	}

	// split: [string, string, map] -> [list]
	splitOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[2])
		_as3, _ := args[0].AsString()
		_as2, _ := args[1].AsString()
		return doSplit(_as3, _as2, opts)
	}

	r.Register("split",
		Signature{Args: []Type{TString, TString, TMap}, Handler: splitOptsHandler},
		Signature{Args: []Type{TString, TString}, Handler: splitHandler},
	)
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
