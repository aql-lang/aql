package engine

import (
	"fmt"
	"strings"
)

func registerRepeat(r *Registry) {
	// repeat: [string, integer] -> [string]
	repeatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsString()
		_as0, _ := args[1].AsInteger()
		return doRepeat(_as1, _as0, strOpts{})
	}

	// repeat: [string, integer, map] -> [string]
	repeatOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[2])
		_as3, _ := args[0].AsString()
		_as2, _ := args[1].AsInteger()
		return doRepeat(_as3, _as2, opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "repeat",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString, TInteger, TMap}, Handler: repeatOptsHandler},
			{Args: []Type{TString, TInteger}, Handler: repeatHandler},
		},
	})
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
