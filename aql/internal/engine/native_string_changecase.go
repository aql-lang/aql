package engine

import (
	"strings"
	"unicode"
)

func registerChangeCase(r *Registry) {
	// changecase: [string] -> [string] (default: lower)
	changeCaseHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsString()
		return doChangeCase(_as0, strOpts{style: "lower"})
	}

	// changecase: [string, map] -> [string]
	changeCaseOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		if opts.style == "" {
			opts.style = "lower"
		}
		_as1, _ := args[0].AsString()
		return doChangeCase(_as1, opts)
	}

	r.Register("changecase",
		Signature{Args: []Type{TString, TMap}, Handler: changeCaseOptsHandler},
		Signature{Args: []Type{TString}, Handler: changeCaseHandler},
		Signature{Args: []Type{TAtom, TMap}, Handler: changeCaseOptsHandler},
		Signature{Args: []Type{TAtom}, Handler: changeCaseHandler},
	)
}

func doChangeCase(input string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	var result string
	switch o.style {
	case "upper":
		result = strings.ToUpper(input)
	case "capitalize":
		result = capitalize(input)
	case "title":
		result = titleCase(input)
	case "sentence":
		result = sentenceCase(input)
	case "fold":
		result = strings.ToLower(input) // fold approximation using toLower
	default: // "lower"
		result = strings.ToLower(input)
	}

	return []Value{NewString(result)}, nil
}

// capitalize uppercases the first character only.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// titleCase uppercases the first letter of each word.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(prev) || unicode.IsPunct(prev) {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return unicode.ToLower(r)
	}, s)
}

// sentenceCase lowercases everything, then uppercases the first letter.
func sentenceCase(s string) string {
	lower := strings.ToLower(s)
	return capitalize(lower)
}
