package engine

import (
	"fmt"
	"strings"
)

func registerConcat(r *Registry) {
	// concat: [list] -> [string]
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return doConcat(args[0], strOpts{})
	}

	// concat: [list, map] -> [string]
	concatOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		opts := parseStrOpts(args[1])
		return doConcat(args[0], opts)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "concat",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList, TMap}, Handler: concatOptsHandler},
			{Args: []Type{TList}, Handler: concatHandler},
		},
	})
}

func doConcat(listVal Value, o strOpts) ([]Value, error) {
	if listVal.Data == nil {
		return nil, fmt.Errorf("concat: argument must be a concrete list, got type literal")
	}
	elems := listVal.AsList()
	var parts []string
	for _, e := range elems.Slice() {
		if e.VType.Equal(TNone) {
			if o.skipNullish {
				continue
			}
			parts = append(parts, "")
			continue
		}
		s := valToString(e)
		if s == "" && o.skipEmpty {
			continue
		}
		parts = append(parts, s)
	}
	result := strings.Join(parts, o.sep)
	return []Value{NewString(result)}, nil
}
