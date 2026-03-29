package engine

import (
	"fmt"
	"strings"
)

// defName extracts a word name from a Value that is either a word or a string.
func defName(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
	return v.AsString()
}

// defPrefixOnly returns true if the name word carries the /p modifier,
// indicating the defined word should be prefix-only (not forward precedence).
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
// Flexible matching handles reordering: in "body def name", forward collects
// name(TWord), pushes it, then prefix sees [body, name] and flexible match
// reorders to [name, body] matching [TWord, TAny].
func registerDef(r *Registry) {
	// All-forward handler: "def foo 42 end" → args=[foo(name), 42(body)]
	defForwardHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		prefixOnly := defPrefixOnly(args[0])
		body := args[1]
		installDef(r, name, body, prefixOnly)
		return nil, nil
	}

	// Infix handler: "42 def foo" → args=[42(body), foo(name)]
	defInfixHandler := func(args []Value) ([]Value, error) {
		body := args[0]
		name := defName(args[1])
		prefixOnly := defPrefixOnly(args[1])
		installDef(r, name, body, prefixOnly)
		return nil, nil
	}

	r.Register("def",
		// All-forward: name first, body second
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: defForwardHandler,
		},
		Signature{
			Args:    []Type{TString, TAny},
			Handler: defForwardHandler,
		},
		// Infix: body first (prefix), name second (forward)
		Signature{
			Args:    []Type{TAny, TWord},
			Handler: defInfixHandler,
		},
		Signature{
			Args:    []Type{TAny, TString},
			Handler: defInfixHandler,
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
				// However, if a 0-arg typed signature exists (e.g. from
				// optional param expansion), execute it instead of erroring.
				if _, ok := top.Data.(FnDefInfo); ok {
					if fn := r.Lookup(name); fn != nil {
						for i := range fn.Signatures {
							sig := &fn.Signatures[i]
							if len(sig.Args) == 0 && sig.Handler != nil && i > 0 {
								result, err := sig.Handler(nil)
								if err != nil {
									return nil, err
								}
								return result, nil
							}
						}
					}
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
		fnDef, ok := body.Data.(FnDefInfo)
		if !ok {
			return
		}

		// Remove any previous DefStack entries whose signatures overlap
		// with the new definition. Without this, redefining a fn-based
		// word with the same signature leaves stale handlers that win
		// matching over the new ones (equal scores, first match wins).
		if stack := r.DefStacks[name]; len(stack) > 0 {
			filtered := stack[:0:0]
			changed := false
			for _, entry := range stack {
				oldFn, ok := entry.Data.(FnDefInfo)
				if ok && fnDefsOverlap(oldFn, fnDef) {
					changed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			if changed {
				r.DefStacks[name] = filtered
				// Rebuild typed signatures from remaining DefStack entries.
				fn := r.funcs[name]
				if fn != nil && len(fn.Signatures) > 0 {
					fn.Signatures = fn.Signatures[:1] // keep generic fallback
				}
				for _, entry := range filtered {
					if fd, ok := entry.Data.(FnDefInfo); ok {
						installFnDef(r, name, fd, isPrefixOnly)
					}
				}
			}
		}

		installFnDef(r, name, fnDef, isPrefixOnly)
		// Store as TFnDef on the stack so uninstallDef handles it uniformly.
		r.DefStacks[name] = append(r.DefStacks[name], NewFnDef(fnDef))
		return
	}

	// FnUndefInfo body (from fn word in pair mode): remove targeted signatures.
	if body.VType.Equal(TFnUndef) {
		undefInfo, ok := body.Data.(FnUndefInfo)
		if !ok {
			return
		}
		uninstallFnSigs(r, name, undefInfo)
		return
	}

	// ObjectTypeInfo body: set the proper name in the type hierarchy.
	if body.IsObjectType() {
		info := body.AsObjectType()
		if info.Parent != nil {
			// Child type: full name is Parent/Name (e.g. Object/Foo/Bar)
			info.Name = info.Parent.Name + "/" + name
		} else {
			// Direct child of Object root: Object/Name
			info.Name = "Object/" + name
		}
		// Register the name parts as known type parts.
		for _, p := range strings.Split(info.Name, "/") {
			r.KnownTypeParts[p] = true
		}
		body = NewObjectType(info)
		r.DefStacks[name] = append(r.DefStacks[name], body)
		return
	}

	r.DefStacks[name] = append(r.DefStacks[name], body)
}

// fnDefsOverlap returns true if any signature in a has the same parameter
// types as any signature in b (ignoring param names, return types, and body).
func fnDefsOverlap(a, b FnDefInfo) bool {
	for _, sa := range a.Sigs {
		for _, sb := range b.Sigs {
			if len(sa.Params) != len(sb.Params) {
				continue
			}
			match := true
			for i := range sa.Params {
				if !sa.Params[i].Type.Equal(sb.Params[i].Type) {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
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
