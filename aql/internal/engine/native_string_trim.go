package engine

import (
	"strings"
	"unicode"
)

func RegisterTrim(r *Registry) {
	// trim: [string] -> [string]
	trimHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsConcreteString()
		return []Value{NewString(strings.TrimSpace(_as0))}, nil
	}

	// trim: [string, map] -> [string]
	trimOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		_as1, _ := args[0].AsConcreteString()
		return doTrim(_as1, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "trim",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TMap}, Handler: trimOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TString}, Handler: trimHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom, TMap}, Handler: trimOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TAtom}, Handler: trimHandler, Returns: []Type{TString}},
		},
	})
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
