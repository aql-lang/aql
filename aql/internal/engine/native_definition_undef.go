package engine

import "fmt"

// registerUndef registers the "undef" word for removing word definitions.
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
func registerUndef(r *Registry) {
	undefHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		uninstallDef(r, name)
		return nil, nil
	}

	// Targeted undef: undef foo fn [[number] [number]]
	undefFnHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		undefInfo, ok := args[1].Data.(FnUndefInfo)
		if !ok {
			return nil, fmt.Errorf("undef: expected fn undef spec, got %s", args[1].String())
		}
		uninstallFnSigs(r, name, undefInfo)
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

// fnSigMatchesSpec returns true if a FnSig matches a FnSigSpec
// (same param types and return types, ignoring param names).
func fnSigMatchesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		if !sig.Params[i].Type.Equal(spec.Params[i].Type) {
			return false
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		if !sig.Returns[i].Equal(spec.Returns[i]) {
			return false
		}
	}
	return true
}

// uninstallFnSigs removes specific function signatures from a word's DefStack.
// For each spec in the FnUndefInfo, it finds and removes the most recent
// DefStack entry containing a matching signature, then rebuilds the
// Function.Signatures slice from the remaining entries.
func uninstallFnSigs(r *Registry, name string, specs FnUndefInfo) {
	sym := Intern(name)
	stack := r.DefStacks[sym]
	if len(stack) == 0 {
		return
	}

	// For each spec, find and remove the most recent matching DefStack entry.
	for _, spec := range specs.Sigs {
		for j := len(stack) - 1; j >= 0; j-- {
			fnDef, ok := stack[j].Data.(FnDefInfo)
			if !ok {
				continue
			}
			matched := false
			for _, sig := range fnDef.Sigs {
				if fnSigMatchesSpec(sig, spec) {
					matched = true
					break
				}
			}
			if matched {
				stack = append(stack[:j], stack[j+1:]...)
				break
			}
		}
	}

	r.DefStacks[sym] = stack

	// If no DefStack entries remain, clean up entirely.
	if len(stack) == 0 {
		delete(r.DefStacks, sym)
		return
	}

	// Rebuild: clear Signatures on the top entry (keep fallback),
	// then re-register from remaining DefStack entries.
	r.clearSigsKeepFallback(name)
	for _, entry := range stack {
		if fnDef, ok := entry.Data.(FnDefInfo); ok {
			installFnDef(r, name, fnDef)
		}
	}
}
