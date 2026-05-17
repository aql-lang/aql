package engine

import "github.com/aql-lang/aql/eng"

// unifyNatives covers the `unify` word — the surface-level entry
// point to the engine's structural unification algorithm.
//
//	a b unify     → [unified, true]  on success
//	a b unify     → ["~unify-fail", false]  on failure
//
// The algorithm (Unify and friends) lives in eng/go/unify.go; this
// file owns the word name and dispatch wiring.
var unifyNatives = []NativeFunc{
	{
		Name:        "unify",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.UnifyHandler,
			Returns: []*Type{TAny, TBoolean},
		}},
	},
}
