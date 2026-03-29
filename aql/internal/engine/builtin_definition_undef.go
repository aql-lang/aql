package engine

// registerUndef registers the "undef" word for removing word definitions.
// undef removes the most recent definition, potentially revealing a
// shadowed one. Signature: [word|string] -> [].
func registerUndef(r *Registry) {
	undefHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		uninstallDef(r, name)
		return nil, nil
	}

	r.Register("undef",
		Signature{
			Args:    []Type{TWord},
			Handler: undefHandler,
		},
		Signature{
			Args:    []Type{TString},
			Handler: undefHandler,
		},
	)

	// Targeted undef: undef foo fn [[number] [number]]
	// All-forward: args=[foo(name), fnUndefInfo]
	undefFnForwardHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		undefInfo := args[1].Data.(FnUndefInfo)
		uninstallFnSigs(r, name, undefInfo)
		return nil, nil
	}
	// Infix: args=[fnUndefInfo, foo(name)]
	undefFnInfixHandler := func(args []Value) ([]Value, error) {
		undefInfo := args[0].Data.(FnUndefInfo)
		name := defName(args[1])
		uninstallFnSigs(r, name, undefInfo)
		return nil, nil
	}
	r.Register("undef",
		Signature{
			Args:    []Type{TWord, TFnUndef},
			Handler: undefFnForwardHandler,
		},
		Signature{
			Args:    []Type{TString, TFnUndef},
			Handler: undefFnForwardHandler,
		},
		Signature{
			Args:    []Type{TFnUndef, TWord},
			Handler: undefFnInfixHandler,
		},
		Signature{
			Args:    []Type{TFnUndef, TString},
			Handler: undefFnInfixHandler,
		},
	)
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

	fn := r.funcs[name]
	if fn == nil {
		return
	}

	// If no DefStack entries remain, clean up entirely.
	if len(stack) == 0 {
		delete(r.funcs, name)
		delete(r.DefStacks, name)
		return
	}

	// Rebuild: keep the generic fallback, remove all typed sigs,
	// then re-register from remaining DefStack entries.
	if len(fn.Signatures) > 0 {
		fn.Signatures = KeepFallback(fn.Signatures)
	}
	for _, entry := range stack {
		if fnDef, ok := entry.Data.(FnDefInfo); ok {
			installFnDef(r, name, fnDef)
		}
	}
}
