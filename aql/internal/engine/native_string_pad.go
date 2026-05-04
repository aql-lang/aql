package engine

import (
	"fmt"
	"strings"
)

func RegisterPad(r *Registry) {
	// pad: [integer, string] -> [string]
	// Forward-first: args[0]=width (forward), args[1]=string (stack).
	// Usage: "ab" pad 5 → "ab   "
	padHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[1].AsConcreteString()
		_as0, _ := args[0].AsConcreteInteger()
		return doPad(_as1, _as0, strOpts{side: "right", fill: " "})
	}

	// pad: [integer, map, string] -> [string]
	// Forward-first: args[0]=width (forward), args[1]=opts (forward), args[2]=string (stack).
	// Usage: "ab" pad 5 {side:"left" fill:"0"} → "000ab"
	padOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if !IsConcrete(args[1]) {
			return nil, fmt.Errorf("pad: options must be a concrete map, got type literal")
		}
		opts := parseStrOpts(args[1])
		if opts.fill == "" {
			opts.fill = " "
		}
		// Default side for pad is "right", not "both" from parseStrOpts.
		if m := args[1].AsMap(); m != nil {
			if _, ok := m.Get("side"); !ok {
				opts.side = "right"
			}
		}
		_as3, _ := args[2].AsConcreteString()
		_as2, _ := args[0].AsConcreteInteger()
		return doPad(_as3, _as2, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "pad",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TInteger, TMap, TString}, Handler: padOptsHandler, Returns: []Type{TString}},
			{Args: []Type{TInteger, TString}, Handler: padHandler, Returns: []Type{TString}},
		},
	})
}

func doPad(input string, targetLen int64, o strOpts) ([]Value, error) {
	current := len(input)
	target := int(targetLen)

	if current >= target {
		if o.trunc {
			return []Value{NewString(input[:target])}, nil
		}
		return []Value{NewString(input)}, nil
	}

	needed := target - current
	fill := o.fill
	if fill == "" {
		fill = " "
	}

	// Generate enough fill characters
	padding := strings.Repeat(fill, (needed/len(fill))+1)

	switch o.side {
	case "left":
		result := padding[:needed] + input
		return []Value{NewString(result)}, nil
	case "both":
		left := needed / 2
		right := needed - left
		result := padding[:left] + input + padding[:right]
		return []Value{NewString(result)}, nil
	default: // "right"
		result := input + padding[:needed]
		return []Value{NewString(result)}, nil
	}
}
