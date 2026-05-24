package native

import "github.com/aql-lang/aql/eng/go"

// unifyNatives covers the `unify` word — the surface-level entry
// point to the engine's structural unification algorithm.
//
//	a b unify     → [unified, true]  on success
//	a b unify     → ["~unify-fail", false]  on failure
//
// The algorithm (Unify and friends) lives in eng/go/unify.go; this
// file owns the word name, dispatch wiring, and the Go-level adapter
// (unifyHandler) that converts an eng.Unify result into the AQL
// stack shape.
var unifyNatives = []NativeFunc{
	{
		Name:        "unify",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: unifyHandler,
			Returns: []*Type{TAny, TBoolean},
		}},
	},
}

func unifyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	unified, ok := eng.Unify(args[0], args[1])
	if ok {
		return []Value{unified, NewBoolean(true)}, nil
	}
	return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
}
