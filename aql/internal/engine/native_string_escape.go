package engine

import "strings"

func RegisterEscape(r *Registry) {
	// escape: [string] -> [string]
	escapeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsConcreteString()
		return doEscape(_as0, strOpts{tgt: "sh", quote: "none"})
	}

	// escape: [string, map] -> [string]
	escapeOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		_as1, _ := args[0].AsConcreteString()
		return doEscape(_as1, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "escape",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: escapeOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: escapeHandler, Returns: []Type{TString}},
		},
	})
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
