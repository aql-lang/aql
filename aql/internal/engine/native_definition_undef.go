package engine

import "fmt"

// RegisterUndef registers the "undef" word for removing word definitions.
// undef removes the most recent definition, potentially revealing a
// shadowed one.
//
// Two simple signatures plus two targeted-undef signatures:
//
//	[TString]             – undef "name"
//	[TAtom/q]             – undef name  (word captured as atom via /q)
//	[TString, TFnUndef]   – undef "name" fn [spec]
//	[TAtom/q, TFnUndef]   – undef name fn [spec]
//
// Forward precedence handles all orderings without infix signatures.
func RegisterUndef(r *Registry) {
	undefHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		UninstallDef(r, name)
		return nil, nil
	}

	// Targeted undef: undef foo fn [[number] [number]]
	undefFnHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		undefInfo, ok := args[1].Data.(FnUndefInfo)
		if !ok {
			return nil, fmt.Errorf("undef: expected fn undef spec, got %s", args[1].String())
		}
		UninstallFnSigs(r, name, undefInfo)
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "undef",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString},
				Handler:        undefHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        undefHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TString, TFnUndef},
				Handler:        undefFnHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TFnUndef},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        undefFnHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})
}

// FnSigMatchesSpec and FnSigSatisfiesSpec live in fnsig.go alongside
// the other FnSig comparison helpers (FnUndefMatchesFnDef, FnDefHasSig).

// UninstallFnSigs: re-exported from aqleng via aliases.go
