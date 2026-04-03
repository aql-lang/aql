package engine

import (
	"strings"
	"unicode"
)

func registerTrim(r *Registry) {
	// trim: [string] -> [string]
	trimHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsString()
		return []Value{NewString(strings.TrimSpace(_as0))}, nil
	}

	// trim: [string, map] -> [string]
	trimOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		_as1, _ := args[0].AsString()
		return doTrim(_as1, opts)
	}

	r.Register("trim",
		Signature{Args: []Type{TString, TMap}, Handler: trimOptsHandler},
		Signature{Args: []Type{TString}, Handler: trimHandler},
		Signature{Args: []Type{TAtom, TMap}, Handler: trimOptsHandler},
		Signature{Args: []Type{TAtom}, Handler: trimHandler},
	)
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
