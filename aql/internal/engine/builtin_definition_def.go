package engine

import "fmt"

// defName extracts a word name from a Value that is either a word or a string.
func defName(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
	return v.AsString()
}

// defPrefixOnly returns true if the name word carries the /p modifier,
// indicating the defined word should be prefix-only (not suffix precedence).
func defPrefixOnly(v Value) bool {
	if v.IsWord() {
		return v.AsWord().ForcePrefix
	}
	return false
}

// registerDef registers the "def" word for defining new words.
//
// def creates literal substitutions: the body replaces the word during
// evaluation. If the body is a list, its elements are spliced into the
// stack. Otherwise the single value is pushed.
//
// Single handler, two signatures:
//
//	Args:[TWord, TAny]   – def name body  or  body def name
//	Args:[TString, TAny] – def "name" body  or  body def "name"
//
// Flexible matching handles reordering: in "body def name", suffix collects
// name(TWord), pushes it, then prefix sees [body, name] and flexible match
// reorders to [name, body] matching [TWord, TAny].
func registerDef(r *Registry) {
	defHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		prefixOnly := defPrefixOnly(args[0])
		body := args[1]
		installDef(r, name, body, prefixOnly)
		return nil, nil
	}

	r.Register("def",
		// Args:[TWord, TAny] — word name
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: defHandler,
		},
		// Args:[TString, TAny] — string name
		Signature{
			Args:    []Type{TString, TAny},
			Handler: defHandler,
		},
	)
}

// installDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), installDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
func installDef(r *Registry, name string, body Value, prefixOnly ...bool) {
	isPrefixOnly := len(prefixOnly) > 0 && prefixOnly[0]
	registerFn := r.Register
	if isPrefixOnly {
		registerFn = r.RegisterPrefixOnly
	}
	if len(r.DefStacks[name]) == 0 {
		// First definition: register one generic fallback handler
		// that reads the top of the definition stack.
		registerFn(name, Signature{
			Handler: func(_ []Value) ([]Value, error) {
				stack := r.DefStacks[name]
				if len(stack) == 0 {
					return nil, fmt.Errorf("undefined: %s", name)
				}
				top := stack[len(stack)-1]
				// Guard: function definitions have typed signatures;
				// the generic handler should not expand them as literals.
				if _, ok := top.Data.(FnDefInfo); ok {
					return nil, fmt.Errorf("signature error: no matching signature for %s", name)
				}
				if top.VType.Equal(TFunction) {
					return nil, fmt.Errorf("signature error: no matching signature for %s", name)
				}
				if top.VType.Equal(TList) && !top.IsTypedList() && !top.IsTableType() {
					elems := top.AsList()
					result := make([]Value, len(elems))
					copy(result, elems)
					return result, nil
				}
				return []Value{top}, nil
			},
		})
	}

	// FnDefInfo body (from fn word): install typed signatures.
	if body.VType.Equal(TFnDef) || body.VType.Equal(TFunction) {
		fnDef := body.Data.(FnDefInfo)
		installFnDef(r, name, fnDef, isPrefixOnly)
		// Store as TFnDef on the stack so uninstallDef handles it uniformly.
		r.DefStacks[name] = append(r.DefStacks[name], NewFnDef(fnDef))
		return
	}

	// FnUndefInfo body (from fn word in pair mode): remove targeted signatures.
	if body.VType.Equal(TFnUndef) {
		undefInfo := body.Data.(FnUndefInfo)
		uninstallFnSigs(r, name, undefInfo)
		return
	}

	r.DefStacks[name] = append(r.DefStacks[name], body)
}

// uninstallDef removes the most recent def for a word. If no definitions
// remain, the function entry is removed so the word falls through to
// normal resolution (unknown word → string).
func uninstallDef(r *Registry, name string) {
	stack := r.DefStacks[name]
	if len(stack) == 0 {
		return
	}

	top := stack[len(stack)-1]
	r.DefStacks[name] = stack[:len(stack)-1]

	// Count typed signatures to remove (function defs register N typed sigs).
	sigsToRemove := 0
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		sigsToRemove = len(fnDef.Sigs)
	}

	fn := r.funcs[name]
	if fn == nil {
		return
	}

	// Remove typed signatures from the end.
	if sigsToRemove > 0 && len(fn.Signatures) >= sigsToRemove {
		fn.Signatures = fn.Signatures[:len(fn.Signatures)-sigsToRemove]
	}

	// If DefStacks is now empty, also remove the generic fallback handler.
	if len(r.DefStacks[name]) == 0 {
		if len(fn.Signatures) > 0 {
			fn.Signatures = fn.Signatures[:len(fn.Signatures)-1]
		}
		if len(fn.Signatures) == 0 {
			delete(r.funcs, name)
		}
		delete(r.DefStacks, name)
	}
}
