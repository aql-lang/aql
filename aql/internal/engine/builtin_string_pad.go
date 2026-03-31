package engine

import (
	"fmt"
	"strings"
)

func registerPad(r *Registry) {
	// pad: [string, integer] -> [string]
	padHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return doPad(args[0].AsString(), args[1].AsInteger(), strOpts{side: "right", fill: " "})
	}

	// pad: [string, integer, map] -> [string]
	padOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[2].Data == nil {
			return nil, fmt.Errorf("pad: options must be a concrete map, got type literal")
		}
		opts := parseStrOpts(args[2])
		if opts.fill == "" {
			opts.fill = " "
		}
		// Default side for pad is "right", not "both" from parseStrOpts.
		if m := args[2].AsMap(); m != nil {
			if _, ok := m.Get("side"); !ok {
				opts.side = "right"
			}
		}
		return doPad(args[0].AsString(), args[1].AsInteger(), opts)
	}

	r.Register("pad",
		Signature{Args: []Type{TString, TInteger, TMap}, Handler: padOptsHandler},
		Signature{Args: []Type{TString, TInteger}, Handler: padHandler},
	)
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
