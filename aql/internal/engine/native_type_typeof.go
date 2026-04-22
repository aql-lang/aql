package engine

import "strings"

func RegisterTypeof(r *Registry) {
	typeofHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		parts := v.VType.Parts
		// Type literals: report metatype instead of represented type.
		if v.Data == nil && !v.VType.Matches(TWord) {
			parts = MetatypeFor(v.VType).Parts
		}
		// Strip numeric/negative literal suffix if present
		// (e.g. the "42" in Scalar/Number/Integer/42).
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
				parts = parts[:len(parts)-1]
			}
			if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
				parts = parts[:len(parts)-1]
			}
		}
		// Return second part if it exists, otherwise first.
		// e.g. Scalar/String/Proper → "String", Word → "Word", None → "None"
		result := parts[0]
		if len(parts) > 1 {
			result = parts[1]
		}
		return []Value{NewAtom(result)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "typeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: typeofHandler,
			Returns: []Type{TAtom},
		}},
	})
}

func RegisterFullTypeof(r *Registry) {
	fulltypeofHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		parts := v.VType.Parts
		// Type literals: report metatype instead of represented type.
		if v.Data == nil && !v.VType.Matches(TWord) {
			parts = MetatypeFor(v.VType).Parts
		}
		// Strip numeric/negative literal suffix if present.
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
				parts = parts[:len(parts)-1]
			}
			if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
				parts = parts[:len(parts)-1]
			}
		}
		return []Value{NewAtom(strings.Join(parts, "/"))}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "fulltypeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: fulltypeofHandler,
			Returns: []Type{TAtom},
		}},
	})
}
