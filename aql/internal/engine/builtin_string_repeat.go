package engine

import (
	"fmt"
	"strings"
)

func registerRepeat(r *Registry) {
	// repeat: [string, integer] -> [string]
	repeatHandler := func(args []Value) ([]Value, error) {
		return doRepeat(args[0].AsString(), args[1].AsInteger(), strOpts{})
	}

	// repeat: [string, integer, map] -> [string]
	repeatOptsHandler := func(args []Value) ([]Value, error) {
		opts := parseStrOpts(args[2])
		return doRepeat(args[0].AsString(), args[1].AsInteger(), opts)
	}

	r.Register("repeat",
		Signature{Args: []Type{TString, TInteger, TMap}, Handler: repeatOptsHandler},
		Signature{Args: []Type{TString, TInteger}, Handler: repeatHandler},
	)
}

func doRepeat(input string, count int64, o strOpts) ([]Value, error) {
	if count < 0 {
		return nil, fmt.Errorf("repeat: count must be non-negative, got %d", count)
	}

	if !o.hasSep || o.sep == "" {
		return []Value{NewString(strings.Repeat(input, int(count)))}, nil
	}

	// With separator: join count copies with sep
	if count == 0 {
		return []Value{NewString("")}, nil
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = input
	}
	return []Value{NewString(strings.Join(parts, o.sep))}, nil
}
