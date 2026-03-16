package engine

import "strings"

func registerTypeof(r *Registry) {
	typeofHandler := func(args []Value) ([]Value, error) {
		v := args[0]
		// Return the full type path as a string, excluding any literal
		// value suffix (e.g. Scalar/Number/Integer/42 → Scalar/Number/Integer).
		parts := v.VType.Parts
		// Strip numeric literal suffix if present (e.g. the "42" in Number/Integer/42).
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
				parts = parts[:len(parts)-1]
			}
			// Also strip negative number suffixes like "-5".
			if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
				parts = parts[:len(parts)-1]
			}
		}
		return []Value{NewAtom(strings.Join(parts, "/"))}, nil
	}

	r.Register("typeof",
		Signature{Args: []Type{TAny}, Handler: typeofHandler},
	)
}
