package engine

import "strings"

func registerReplace(r *Registry) {
	// replace: [string, string, string] -> [string]
	replaceHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsString()
		_as1, _ := args[1].AsString()
		_as0, _ := args[2].AsString()
		return doReplace(_as2, _as1, _as0,
			strOpts{cs: "sensitive", mode: "literal", scope: "first"})
	}

	// replace: [string, string, string, map] -> [string]
	replaceOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[3])
		_as5, _ := args[0].AsString()
		_as4, _ := args[1].AsString()
		_as3, _ := args[2].AsString()
		return doReplace(_as5, _as4, _as3, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "replace",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TString, TString, TMap}, Handler: replaceOptsHandler},
			{Args: []Type{TString, TString, TString}, Handler: replaceHandler},
		},
	})
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
