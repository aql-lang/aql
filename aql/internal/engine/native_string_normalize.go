package engine

import (
	"strings"
	"unicode"
)

func RegisterNormalize(r *Registry) {
	// normalize: [string] -> [string]
	normalizeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsConcreteString()
		return doNormalize(_as0, strOpts{form: "NFC", eol: "preserve"})
	}

	// normalize: [string, map] -> [string]
	normalizeOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		_as1, _ := args[0].AsConcreteString()
		return doNormalize(_as1, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "normalize",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: normalizeOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: normalizeHandler, Returns: []Type{TString}},
		},
	})
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
