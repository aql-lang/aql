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
// exactly: same arity, same param types pairwise, same return types
// pairwise. Used by `undef name fn [spec]` to identify the precise
// previously-installed signature to remove. Variance is intentionally
// NOT applied here — the user is naming a specific shape to discard.
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

// fnSigSatisfiesSpec returns true if a candidate FnSig satisfies a
// FnSigSpec under standard structural function subtyping:
//
//   - **Inputs are contravariant.** Each spec param type must be a
//     subtype of the candidate's param type at the same position.
//     Example: spec=[Integer], sig=[Number] — sig accepts Integer
//     (because Integer ⊂ Number), so it satisfies the spec.
//   - **Returns are covariant.** Each candidate return type must be
//     a subtype of the spec's return type at the same position.
//     Example: spec=[Number], sig=[Integer] — sig produces Integer
//     which is also a Number, so it satisfies the spec.
//   - Arities must match exactly. Optional/BarrierPos differences
//     and pattern (FnParam.Pattern) constraints are not yet checked.
//
// Used by `fnDefHasSig` for `type Foo fn [...]` constraint matching.
// Strictly widens the previous exact-match rule.
func fnSigSatisfiesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		// Contravariant: spec_input must be a subtype of sig_input.
		// `t.Matches(pattern)` is true iff t ⊆ pattern in the type
		// lattice, so spec.Type.Matches(sig.Type) checks spec ⊆ sig.
		if !spec.Params[i].Type.Matches(sig.Params[i].Type) {
			return false
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		// Covariant: sig_return must be a subtype of spec_return.
		if !sig.Returns[i].Matches(spec.Returns[i]) {
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
	stack := r.DefStacks[name]
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

	r.DefStacks[name] = stack

	// If no DefStack entries remain, clean up entirely.
	if len(stack) == 0 {
		delete(r.DefStacks, name)
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
