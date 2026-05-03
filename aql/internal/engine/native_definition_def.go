package engine

import (
	"fmt"
	"strings"
)

// defName extracts a word name from a Value that is either a word or a string.
func defName(v Value) string {
	if v.IsWord() {
		_as0, _ := v.AsWord()
		return _as0.Name
	}
	_as1, _ := v.AsString()
	return _as1
}

// defStackOnly returns true if the name word carries the /s modifier,
// indicating the defined word should be stack-only (not forward precedence).
func defStackOnly(v Value) bool {
	if v.IsWord() {
		_as2, _ := v.AsWord()
		return _as2.ForceStack
	}
	return false
}

// RegisterDef registers the "def" word for defining new words.
//
// def creates literal substitutions: the body replaces the word during
// evaluation. If the body is a list, its elements are spliced into the
// stack. Otherwise the single value is pushed.
//
// Three signatures, sharing a single handler each:
//
//	Args:[TString, TAny]       – def "name" body
//	Args:[TAtom/q, TAny]       – def name body  (word captured as atom via /q)
//	Args:[TMap, TAny]          – def name:Type body  (typed binding)
//
// The /q modifier on the Atom position causes Word values to be treated as
// Atoms for matching, and captured without evaluation during forward
// collection. Forward precedence rules handle all orderings (forward,
// infix, postfix) without separate infix signatures.
//
// The TMap form picks up the surface syntax `def name:Type body`. At the
// top level, jsonic parses `name:Type` as a single-pair map; the handler
// extracts the only key as the name, the only value as a type
// constraint, and unifies the body with the constraint before
// installing. Multi-key maps and non-type values are rejected.
func RegisterDef(r *Registry) {
	defHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		stackOnly := defStackOnly(args[0])
		body := args[1]
		if IsCapitalisedName(name) {
			return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
		}
		// Refuse a def whose name is already a registered TYPE — type
		// and def share the same Word namespace so a single name
		// must mean exactly one thing.
		if _, ok := r.Types[name]; ok {
			return nil, fmt.Errorf("def %s: name clash — already a type", name)
		}
		installDef(r, name, body, stackOnly)
		// Record installation for unused-def analysis. The arg's
		// Pos points at the name token.
		r.recordCheckDef(name, args[0].Pos)
		return nil, nil
	}

	defTypedHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		nameMap := args[0].AsMap()
		if nameMap == nil || nameMap.Len() == 0 {
			return nil, fmt.Errorf("def: typed-name map must have exactly one key, got empty/non-concrete map")
		}
		if nameMap.Len() != 1 {
			return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
		}
		name := nameMap.Keys()[0]
		if IsCapitalisedName(name) {
			return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
		}
		if _, ok := r.Types[name]; ok {
			return nil, fmt.Errorf("def %s: name clash — already a type", name)
		}
		constraint, _ := nameMap.Get(name)
		// NoEvalMapArgs suppresses the generic autoEvalMap pipeline for
		// this slot, so a Word at the type position arrives raw.
		// Resolve named user-defined types via r.Types (the dedicated
		// type registry) first; fall back to DefStacks so legacy
		// type-definition kinds that still pass through installDef
		// (records, ObjectType, DepScalar, …) keep working until the
		// full migration completes.
		if constraint.IsWord() {
			w, _ := constraint.AsWord()
			if tv, ok := r.Types[w.Name]; ok {
				constraint = tv
			} else if ds := r.DefStacks[w.Name]; len(ds) > 0 {
				constraint = ds[len(ds)-1]
			}
		}
		if !isTypeValue(constraint) {
			return nil, fmt.Errorf("def %s: type annotation must be a type value, got %s", name, constraint.String())
		}
		// When the constraint is a function-shape type and the body is
		// a quoted atom naming a defined function, resolve to the
		// function value before unifying. This lets the user write
		// `def m:Mapper (quote double)` to bind m to the function
		// double — quote's normal output is an Atom, which would
		// never unify with a FnUndef constraint otherwise.
		body := args[1]
		if constraint.VType.Equal(TFnUndef) && body.IsAtom() {
			atomName, _ := body.AsAtom()
			if ds := r.DefStacks[atomName]; len(ds) > 0 {
				top := ds[len(ds)-1]
				if top.VType.Equal(TFnDef) || top.VType.Equal(TFunction) {
					body = top
				}
			}
		}
		// Predicate type: the constraint is a fn whose body unifies the
		// candidate against the type. The fn returns None on failure
		// or the unified value on success — typically the candidate
		// itself, but a coercive predicate may return a transformed
		// value. The def installs with the *returned* value, not the
		// candidate, so a predicate like `[x upper]` actually rebinds
		// to the transformed shape.
		if constraint.VType.Equal(TFnDef) || constraint.VType.Equal(TFunction) {
			fnDef, ok := constraint.Data.(FnDefInfo)
			if !ok {
				return nil, fmt.Errorf("def %s: predicate type has invalid payload", name)
			}
			if len(fnDef.Sigs) == 0 || len(fnDef.Sigs[0].Params) != 1 {
				return nil, fmt.Errorf("def %s: predicate type must take exactly one argument", name)
			}
			result, err := r.CallAQL(&fnDef.Sigs[0], []Value{body})
			if err != nil {
				return nil, fmt.Errorf("def %s: predicate evaluation failed: %w", name, err)
			}
			if len(result) != 1 {
				return nil, fmt.Errorf("def %s: predicate type must return exactly one value, got %d", name, len(result))
			}
			out := result[0]
			if out.VType.Equal(TNone) {
				return nil, fmt.Errorf("def %s: value does not satisfy predicate type", name)
			}
			installDef(r, name, out)
			r.recordCheckDef(name, args[0].Pos)
			return nil, nil
		}
		unified, ok := Unify(body, constraint)
		if !ok {
			return nil, fmt.Errorf("def %s: value does not unify with declared type %s",
				name, constraint.String())
		}
		installDef(r, name, unified)
		r.recordCheckDef(name, args[0].Pos)
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "def",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				// Typed-name binding: def name:Type body. Sorts first
				// because TMap is more specific than TString / TAtom
				// at the same depth (higher inherent score).
				// NoEvalMapArgs[0]=true keeps the type-name map's value
				// raw so the handler can resolve it through DefStacks
				// itself — important for fn-as-type names that double
				// as registered callables.
				Args:           []Type{TMap, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				NoEvalMapArgs:  map[int]bool{0: true},
				Handler:        defTypedHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TString, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})
}

// installDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), installDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
func installDef(r *Registry, name string, body Value, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	_ = isStackOnly // used by installFnDef below

	// FnDefInfo body (from fn word): install typed signatures.
	// Only fn-based defs register functions; simple value defs just use DefStacks.
	if body.VType.Equal(TFnDef) || body.VType.Equal(TFunction) {
		fnDef, ok := body.Data.(FnDefInfo)
		if !ok {
			return
		}
		fnDef.Name = name

		// Add a fallback handler (0-arg catch-all) if none exists yet.
		// This handles 0-arg invocations of fn-defined words.
		hasFallback := false
		if prev := r.Lookup(name); prev != nil {
			for _, sig := range prev.Signatures {
				if sig.Fallback {
					hasFallback = true
					break
				}
			}
		}
		if !hasFallback {
			fnDef.Signatures = append(fnDef.Signatures, Signature{
				Fallback: true,
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					stack := r.DefStacks[name]
					if len(stack) == 0 {
						return nil, fmt.Errorf("undefined: %s", name)
					}
					top := stack[len(stack)-1]
					if _, ok := top.Data.(FnDefInfo); ok {
						if fn := r.Lookup(name); fn != nil {
							for i := range fn.Signatures {
								sig := &fn.Signatures[i]
								if len(sig.Args) == 0 && sig.Handler != nil && !sig.Fallback {
									result, err := sig.Handler(nil, nil, nil, r)
									if err != nil {
										return nil, err
									}
									return result, nil
								}
							}
						}
						return nil, makeAqlError("signature_error", "no matching signature for "+name, name, r.Source, "")
					}
					if top.VType.Equal(TFunction) {
						return nil, makeAqlError("signature_error", "no matching signature for "+name, name, r.Source, "")
					}
					return nil, makeAqlError("signature_error", "no matching signature for "+name, name, r.Source, "")
				},
			})
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
				// Rebuild: clear Signatures on the top FnDefInfo (keep fallback),
				// then re-register from remaining DefStack entries.
				if top := r.Lookup(name); top != nil {
					r.clearSigsKeepFallback(name)
				}
				for _, entry := range filtered {
					if fd, ok := entry.Data.(FnDefInfo); ok {
						installFnDef(r, name, fd, isStackOnly)
					}
				}
			}
		}

		// Carry forward existing compiled Signatures (from previous defs
		// of the same name) so overloading works across stacked defs.
		if prev := r.Lookup(name); prev != nil {
			fnDef.Signatures = append([]Signature(nil), prev.Signatures...)
			fnDef.ForwardPrecedence = prev.ForwardPrecedence
		}
		// Push the FnDefInfo to DefStacks first, then installFnDef→Register→
		// upsertFnDef will update its Signatures in place.
		r.DefStacks[name] = append(r.DefStacks[name], NewFnDef(fnDef))
		installFnDef(r, name, fnDef, isStackOnly)
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
		info, _ := body.AsObjectType()
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

	// If DefStacks is now empty, clean up entirely.
	if len(r.DefStacks[name]) == 0 {
		delete(r.DefStacks, name)
		return
	}

	// Count typed signatures to remove (function defs register N typed sigs).
	_, isFnDef := top.Data.(FnDefInfo)
	if !isFnDef {
		return
	}

	// Rebuild: clear Signatures on the (now-top) entry, keep fallback,
	// then re-register from remaining DefStack entries.
	r.clearSigsKeepFallback(name)
	for _, entry := range r.DefStacks[name] {
		if fd, ok := entry.Data.(FnDefInfo); ok {
			installFnDef(r, name, fd)
		}
	}
}
