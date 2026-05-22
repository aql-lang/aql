package eng

import (
	"fmt"
	"strings"
)

// InstallDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), InstallDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
func InstallDef(r *Registry, name string, body Value, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	_ = isStackOnly // used by InstallFnDef below

	// FnDefInfo body (from fn word): install typed signatures.
	// Only fn-based defs register functions; simple value defs just use DefStacks.
	if body.Parent.Equal(TFnDef) || body.Parent.Equal(TFunction) {
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
					top, ok := r.Defs.Top(name)
					if !ok {
						return nil, fmt.Errorf("undefined: %s", name)
					}
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
						return nil, r.AqlError("signature_error", "no matching signature for "+name, name)
					}
					if top.Parent.Equal(TFunction) {
						return nil, r.AqlError("signature_error", "no matching signature for "+name, name)
					}
					return nil, r.AqlError("signature_error", "no matching signature for "+name, name)
				},
			})
		}

		// Remove any previous DefStack entries whose signatures overlap
		// with the new definition. Without this, redefining a fn-based
		// word with the same signature leaves stale handlers that win
		// matching over the new ones (equal scores, first match wins).
		if stack := r.Defs.Stack(name); len(stack) > 0 {
			filtered := stack[:0:0]
			changed := false
			for _, entry := range stack {
				oldFn, ok := entry.Data.(FnDefInfo)
				if ok && FnDefsOverlap(oldFn, fnDef) {
					changed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			if changed {
				r.Defs.Set(name, filtered)
				// Rebuild: clear Signatures on the top FnDefInfo (keep fallback),
				// then re-register from remaining DefStack entries.
				if top := r.Lookup(name); top != nil {
					r.clearSigsKeepFallback(name)
				}
				for _, entry := range filtered {
					if fd, ok := entry.Data.(FnDefInfo); ok {
						InstallFnDef(r, name, fd, isStackOnly)
					}
				}
			}
		}

		// Carry forward existing compiled Signatures (from previous defs
		// of the same name) so overloading works across stacked defs.
		if prev := r.Lookup(name); prev != nil {
			fnDef.Signatures = append([]Signature(nil), prev.Signatures...)
		}
		// Push the FnDefInfo to DefStacks first, then InstallFnDef→Register→
		// upsertFnDef will update its Signatures in place.
		r.Defs.Push(name, NewFnDef(fnDef))
		InstallFnDef(r, name, fnDef, isStackOnly)
		return
	}

	// FnUndefInfo body (from fn word in pair mode): remove targeted signatures.
	if body.Parent.Equal(TFnUndef) {
		undefInfo, ok := body.Data.(FnUndefInfo)
		if !ok {
			return
		}
		UninstallFnSigs(r, name, undefInfo)
		return
	}

	// ObjectTypeInfo body: set the proper name in the type hierarchy.
	if IsObjectType(body) {
		info, _ := AsObjectType(body)
		if info.Parent != nil {
			// Child type: full name is Parent/Name (e.g. Ideal/Foo/Bar)
			info.Name = info.Parent.Name + "/" + name
		} else {
			// Direct child of the Object kind: Ideal/Name
			info.Name = "Ideal/" + name
		}
		// Register the name parts as known type parts.
		for _, p := range strings.Split(info.Name, "/") {
			r.RegisterPart(p)
		}
		// Preserve the body's *Type identity (set by the caller via
		// NewObjectType). InstallDef rewrites info.Name based on the
		// def name and parent, then re-wraps the value — but the def
		// itself stays the caller's choice. For builtin object types
		// (Resource, Entity) the caller passes the canonical builtin
		// *Type; for user-defined object types installed as defs the
		// caller is responsible for minting first.
		def := body.Parent
		if def == nil {
			def = TObject
		}
		body = NewObjectType(def, info)
		r.Defs.Push(name, body)
		return
	}

	r.Defs.Push(name, body)
}

// UninstallDef removes the most recent def for a word. If no definitions
// remain, the function entry is removed so the word falls through to
// normal resolution (unknown word → string).
func UninstallDef(r *Registry, name string) {
	top, ok := r.Defs.Top(name)
	if !ok {
		return
	}
	r.Defs.Pop(name)

	if !r.Defs.Has(name) {
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
	for _, entry := range r.Defs.Stack(name) {
		if fd, ok := entry.Data.(FnDefInfo); ok {
			InstallFnDef(r, name, fd)
		}
	}
}

// InstallFnDef registers typed signatures for a function definition.
// For each signature, it creates a handler that binds named parameters
// via InstallDef, returns body tokens, and appends undef cleanup.
func InstallFnDef(r *Registry, name string, fnDef FnDefInfo, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	// Expand optional parameters into additional signatures.
	fnDef.Sigs = ExpandOptionalSigs(name, fnDef.Sigs)
	for _, sig := range fnDef.Sigs {
		argTypes := make([]*Type, len(sig.Params))
		var patterns map[int]Value
		for i, p := range sig.Params {
			argTypes[i] = p.Type
			if p.Pattern != nil {
				if patterns == nil {
					patterns = make(map[int]Value)
				}
				patterns[i] = *p.Pattern
			}
		}
		s := sig // capture for closure
		handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			var result []Value
			var names []string
			// Wrap the entire expansion (unnamed args + body + undef
			// cleanup) in parens so it evaluates as a single
			// sub-expression. Without this, an outer forward can grab
			// intermediate values from the body before the body
			// finishes executing (e.g. recursive factorial: the outer
			// mul's forward grabs x=1 from the inner body instead of
			// waiting for the full result).
			result = append(result, NewOpenParen())

			// Push args list onto the args stack for access via the
			// "args" word (args.0, args.1, etc.).
			argsCopy := make([]Value, len(args))
			copy(argsCopy, args)
			argsList := NewList(argsCopy)
			if err := r.Args.Push(argsList); err != nil {
				return nil, err
			}

			unnamedCount := 0
			for i, p := range s.Params {
				if p.Name != "" {
					arg := args[i]
					// Quote list params so they're treated as data values
					// when referenced in the body, not expanded as code bodies.
					if arg.Parent.Equal(TList) && !arg.Quoted {
						arg.Quoted = true
					}
					InstallDef(r, p.Name, arg)
					names = append(names, p.Name)
				} else {
					// Unnamed parameter: push value back for the body to use
					result = append(result, args[i])
					unnamedCount++
				}
			}
			// Snapshot DefStacks lengths after installing named params
			// so we can clean up any defs created during body execution
			// (fixes def leakage from fn bodies — DX-REPORT Issue 2).
			defSnapshot := r.Defs.Snapshot()

			body := make([]Value, len(s.Body))
			copy(body, s.Body)
			result = append(result, body...)
			// Clean up defs created during body execution, then pop
			// the args stack to restore the previous args (for nesting).
			result = append(result, NewDefCleanup(DefCleanupInfo{
				Snapshot: defSnapshot,
				Registry: r,
			}))
			result = append(result, NewWord("__pa"))
			for i := len(names) - 1; i >= 0; i-- {
				// Force forward so undef takes the name word that follows,
				// not a same-typed value from the prefix stack (e.g. a
				// string return value when the param is also a string).
				result = append(result,
					NewWordModified("undef", -1, false, true),
					NewWord(names[i]),
				)
			}
			// Inject return-check if return types are declared.
			if len(s.Returns) > 0 {
				result = append(result, NewReturnCheck(ReturnCheckInfo{
					FuncName:     name,
					Returns:      s.Returns,
					UnnamedCount: unnamedCount,
				}))
			}
			result = append(result, NewCloseParen())
			return result, nil
		}
		// Static type-check: analyse the body once per arg-type
		// tuple via AnalyseFnBody. If declared return types are
		// present, use them verbatim (no analysis needed); otherwise
		// use the residual top-of-stack carrier(s).
		paramNames := make([]string, len(s.Params))
		paramPatterns := make([]*Value, len(s.Params))
		for i, p := range s.Params {
			paramNames[i] = p.Name
			paramPatterns[i] = p.Pattern
		}
		declaredReturns := append([]*Type(nil), s.Returns...)
		bodyCopy := append([]Value(nil), s.Body...)
		nameCopy := name
		returnsFn := func(args []Value, _ *Registry) []Value {
			// Pattern / record-shape check: for each declared
			// record-typed param, verify the arg map carries each
			// declared field key. Skip calls whose arg is empty or
			// whose key set doesn't overlap the pattern at all
			// (that pattern is typically the one used during fn
			// body analysis, not a real user call).
			for i, pat := range paramPatterns {
				if pat == nil || i >= len(args) {
					continue
				}
				val := args[i]
				if !pat.Parent.Equal(TMap) || !val.Parent.Equal(TMap) ||
					pat.Data == nil || val.Data == nil {
					continue
				}
				pMap, _ := AsMap(*pat)
				vMap, _ := AsMap(val)
				if pMap == nil || vMap == nil || vMap.Len() == 0 {
					continue
				}
				// Overlap gate: only emit if val's keys intersect
				// the pattern at all. This avoids false positives
				// when analysing with synthetic/default arg maps.
				overlap := 0
				for _, k := range pMap.Keys() {
					if _, ok := vMap.Get(k); ok {
						overlap++
					}
				}
				if overlap == 0 {
					continue
				}
				for _, key := range pMap.Keys() {
					pv, _ := pMap.Get(key)
					av, hasKey := vMap.Get(key)
					if !hasKey {
						r.Check.AddDiagnostic(CheckDiagnostic{
							Code:     "record_shape_mismatch",
							Detail:   "argument to " + nameCopy + " missing field: " + key,
							Word:     nameCopy,
							Severity: SeverityError,
						})
						continue
					}
					if pv.Data == nil && !av.Parent.Matches(pv.Parent) && !av.Parent.Equal(TAny) {
						r.Check.AddDiagnostic(CheckDiagnostic{
							Code:     "record_shape_mismatch",
							Detail:   "argument to " + nameCopy + ": field " + key + " expected " + pv.Parent.String() + ", got " + av.Parent.String(),
							Word:     nameCopy,
							Severity: SeverityError,
						})
					}
				}
			}
			// Always analyse the body so diagnostics emitted by stepWord
			// (undefined_word, no_signature, …) inside the body propagate
			// up to the parent registry. When the fn declares an explicit
			// return type, we use that for the carrier result and drop
			// the analyser's residual stack — the analyser is run purely
			// for its side-effecting diagnostic collection. Memoisation
			// inside AnalyseFnBody keeps recursive / repeated calls cheap.
			stk := AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, args)
			if len(declaredReturns) > 0 {
				out := make([]Value, len(declaredReturns))
				for i, t := range declaredReturns {
					out[i] = NewCarrier(t)
				}
				return out
			}
			if len(stk) == 0 {
				return []Value{NewCarrier(TAny)}
			}
			return stk
		}

		r.RegisterNativeFunc(NativeFunc{
			Name:        name,
			ForwardArgs: !isStackOnly,
			Signatures: []NativeSig{{
				Args:       argTypes,
				Handler:    handler,
				Patterns:   patterns,
				BarrierPos: s.BarrierPos,
				ReturnsFn:  returnsFn,
			}},
		})
	}
}

// UninstallFnSigs removes specific function signatures from a word's DefStack.
// For each spec in the FnUndefInfo, it finds and removes the most recent
// DefStack entry containing a matching signature, then rebuilds the
// Function.Signatures slice from the remaining entries.
func UninstallFnSigs(r *Registry, name string, specs FnUndefInfo) {
	stack := r.Defs.Stack(name)
	if len(stack) == 0 {
		return
	}
	stack = append([]Value(nil), stack...)

	// For each spec, find and remove the most recent matching DefStack entry.
	for _, spec := range specs.Sigs {
		for j := len(stack) - 1; j >= 0; j-- {
			fnDef, ok := stack[j].Data.(FnDefInfo)
			if !ok {
				continue
			}
			matched := false
			for _, sig := range fnDef.Sigs {
				if FnSigMatchesSpec(sig, spec) {
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

	r.Defs.Set(name, stack)

	// If no DefStack entries remain, clean up entirely.
	if len(stack) == 0 {
		return
	}

	// Rebuild: clear Signatures on the top entry (keep fallback),
	// then re-register from remaining DefStack entries.
	r.clearSigsKeepFallback(name)
	for _, entry := range stack {
		if fnDef, ok := entry.Data.(FnDefInfo); ok {
			InstallFnDef(r, name, fnDef)
		}
	}
}

// CoerceBoolean converts any value to a boolean using the same rules
// as `convert boolean`: booleans pass through, numbers are non-zero,
// none is false, lists/maps are non-empty, "true"/"false" parse
// literally, all other values are non-empty.
func CoerceBoolean(v Value) bool {
	switch {
	case v.Parent.Matches(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case v.Parent.Matches(TNumber):
		n, _ := AsNumber(v)
		return n != 0
	case v.Parent.Equal(TNone):
		return false
	case v.Parent.Equal(TList):
		if v.Data == nil {
			return false
		}
		if elems, err := AsMutableList(v); err == nil {
			return len(elems) > 0
		}
		// Non-[]Value list backings (table types, query builders) are truthy.
		return true
	case v.Parent.Equal(TMap):
		if v.Data == nil {
			return false
		}
		if om, err := AsMutableMap(v); err == nil {
			return om.Len() > 0
		}
		// Non-*OrderedMap map backings (record/options/child types) are truthy.
		return true
	}
	text := ValToString(v)
	switch text {
	case "true":
		return true
	case "false", "":
		return false
	default:
		return text != ""
	}
}

// CowSet performs a copy-on-write set on a Store. It creates a new Store
// layer whose prototype is the old Store, sets the key in the new layer,
// and propagates the update up through parent Stores to the ctxStack.
func CowSet(store *StoreInstanceInfo, key string, val Value, r *Registry) {
	// Create new COW layer: only the changed key, prototype = old store.
	newStore := &StoreInstanceInfo{
		TypeName:  store.TypeName,
		Data:      map[string]Value{key: val},
		Prototype: store,
		Parent:    store.Parent,
		ParentKey: store.ParentKey,
	}

	// Track parent for nested Store values.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = newStore
		childStore.ParentKey = key
	}

	// Propagate up the parent chain: each parent Store gets a new COW
	// layer with the updated child reference.
	current := newStore
	parent := store.Parent
	parentKey := store.ParentKey

	for parent != nil {
		newParent := &StoreInstanceInfo{
			TypeName:  parent.TypeName,
			Data:      map[string]Value{parentKey: NewStoreValue(nil, current)},
			Prototype: parent,
			Parent:    parent.Parent,
			ParentKey: parent.ParentKey,
		}
		current.Parent = newParent
		current.ParentKey = parentKey

		current = newParent
		parentKey = parent.ParentKey
		parent = parent.Parent
	}

	// current is the topmost COW'd Store. Update the ctxStack entry that
	// references the original store (either directly or via prototype chain).
	// The topmost COW'd store's prototype is the original root store.
	// Walk each ctxStack entry's prototype chain to see if it passes
	// through the original root, and if so, create a new ctxStack entry
	// that uses the COW'd store.
	origRoot := current.Prototype
	if origRoot == nil {
		origRoot = store
	}
	r.Contexts.UpdateChain(origRoot, current)
}

// IsHostTypeBody reports whether v is a constructed type produced by a
// host Ideal: an ExtensionPayload whose Body embeds eng.HostTypeBody.
// The kernel recognises such a value as a type without inspecting its
// concrete shape (the payload Body being opaque). See
// lang/doc/design/IDEAL.0.md §6.
func IsHostTypeBody(v Value) bool {
	ep, ok := v.Data.(ExtensionPayload)
	if !ok {
		return false
	}
	_, ok = ep.Body.(interface{ hostTypeBody() })
	return ok
}

// IsTypeBody reports whether a value is a valid type definition body
// in the strict, structural sense: it carries explicit type-shape
// information (a type literal, disjunct, record / table / object /
// options type, typed list/map, dependent scalar, fn-shape, or
// predicate function).
//
// AQL also lets every concrete value act as a type — `type Foo 1`
// defines Foo as the singleton type containing only 1 — but that
// "literals as types" admission is checked separately via
// IsLiteralTypeBody at the `type` install site, so paths that need
// to discriminate code-bodies / fn-bodies / data-defs from explicit
// type shapes (e.g. `inspect`) keep using IsTypeBody and stay sharp.
func IsTypeBody(v Value) bool {
	// Type literal (Data==nil): number, string, boolean, any, etc.
	// Excludes the value `none` (Data != nil sentinel).
	if v.Data == nil {
		return true
	}
	// Implicit-map record shape (`{x:Integer}`): a Map whose backing
	// OrderedMap is flagged Implicit. Used as a structural Node-type
	// declaration body.
	if IsImplicitMap(v) {
		return true
	}
	// Record type
	if IsRecordType(v) {
		return true
	}
	// Options type
	if IsOptionsType(v) {
		return true
	}
	// Table type
	if IsTableType(v) {
		return true
	}
	// Disjunct
	if IsDisjunct(v) {
		return true
	}
	// Typed list [:type]
	if IsTypedList(v) {
		return true
	}
	// Typed map {:type}
	if IsTypedMap(v) {
		return true
	}
	// Object type
	if IsObjectType(v) {
		return true
	}
	// Dependent scalar type (Integer gt 10, String lt "z", …)
	if v.IsDepScalar() {
		return true
	}
	// Function-signature type: a FnUndef carrying input + output sig
	// patterns and no body.
	if v.Parent.Equal(TFnUndef) {
		return true
	}
	// Predicate type: a FnDef / Function whose body returns a Boolean.
	if v.Parent.Equal(TFnDef) || v.Parent.Equal(TFunction) {
		return true
	}
	// Host-Ideal constructed type (ExtensionPayload + HostTypeBody).
	if IsHostTypeBody(v) {
		return true
	}
	return false
}

// PredicateInputType returns the concrete input type of a
// predicate-shaped fn body (a Function or FnDef whose first sig
// takes exactly one argument with a declared type other than Any).
// Returns nil if v isn't a predicate type or the input type is Any
// or unset — those bodies stay parented at TFnDef / TFunction, the
// pre-existing behavior.
//
// Used by InstallType to mint user-defined predicate types with the
// declared input type as their parent so values rewrapped by the
// typed-bind path participate in the LCA-walk dispatch alongside
// kernel scalars. Without this, `behave compare/q (fn [[Positive
// Positive] …])` would have no dispatch surface — no value's Parent
// is ever Positive.
func PredicateInputType(v Value) *Type {
	if v.Parent == nil {
		return nil
	}
	if !v.Parent.Equal(TFnDef) && !v.Parent.Equal(TFunction) {
		return nil
	}
	info, ok := v.Data.(FnDefInfo)
	if !ok || len(info.Sigs) == 0 {
		return nil
	}
	sig := info.Sigs[0]
	if len(sig.Params) != 1 {
		return nil
	}
	t := sig.Params[0].Type
	if t == nil || t.Equal(TAny) {
		return nil
	}
	return t
}

// IsLiteralTypeBody reports whether v can be installed as a "value-
// is-a-type" type body — the singleton-type interpretation. Scalar
// literals (Integer / Decimal / String / Boolean / Atom / Path / the
// `none` value), and concrete lists / maps qualify. Used by
// installType to relax the strict IsTypeBody check in a way that
// doesn't pollute the inspect / fn-shape paths.
func IsLiteralTypeBody(v Value) bool {
	if IsNone(v) {
		return true
	}
	switch {
	case v.Parent.Matches(TInteger),
		v.Parent.Matches(TDecimal),
		v.Parent.Matches(TNumber),
		v.Parent.Matches(TString),
		v.Parent.Matches(TBoolean),
		v.Parent.Equal(TAtom),
		v.Parent.Equal(TPath):
		return v.Data != nil
	}
	if v.Parent.Equal(TList) && v.Data != nil {
		return true
	}
	if v.Parent.Equal(TMap) && v.Data != nil {
		return true
	}
	return false
}

// ResolveWordValue converts a word value to its semantic value.
// Words named "true"/"false" become booleans, known type names become type
// literals, and other words become atoms (bare strings).
func ResolveWordValue(v Value) Value {
	if !IsWord(v) {
		return v
	}
	_as1, _ := AsWord(v)
	name := _as1.Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	default:
		if t, ok := typeNames[name]; ok {
			return NewTypeLiteral(t)
		}
		return NewAtom(name)
	}
}

// SimplifyDisjunctAlts filters Never, dedupes structurally identical
// alternatives, and applies subsumption: a strict subtype drops in
// favour of its supertype, and a concrete value drops if some other
// alternative is a covering type literal. Two concrete values of the
// same type are both kept — each one is a distinct piece of
// information that the type literal couldn't replace.
func SimplifyDisjunctAlts(alts []Value) []Value {
	// First pass: drop Never.
	live := make([]Value, 0, len(alts))
	for _, alt := range alts {
		if ValueType(alt).Equal(TNever) {
			continue
		}
		live = append(live, alt)
	}
	// Second pass: keep an alt only if no other live alt subsumes or
	// duplicates it. "Earlier-wins" for duplicates so source order is
	// preserved among survivors.
	out := make([]Value, 0, len(live))
outer:
	for i, cand := range live {
		candType := ValueType(cand)
		// Drop if structurally equal to an earlier kept alt.
		for j := 0; j < i; j++ {
			if ValueType(live[j]).Equal(candType) && ValuesEqual(live[j], cand) {
				continue outer
			}
		}
		// Drop if subsumed by some other alt:
		//   - cand is a type literal whose type is a strict subtype
		//     of another's (Integer subsumed by Number).
		//   - cand is a concrete value covered by another type literal
		//     (5 subsumed by Integer).
		// Strict subtype only: equal types are handled by dedup above.
		for j, other := range live {
			if i == j {
				continue
			}
			otherType := ValueType(other)
			if candType.Equal(otherType) {
				continue
			}
			if !candType.Matches(otherType) {
				continue
			}
			// cand's type is a strict subtype of other's.
			if cand.Data == nil && other.Data == nil {
				continue outer
			}
			if cand.Data != nil && other.Data == nil {
				continue outer
			}
		}
		out = append(out, cand)
	}
	return out
}

// FnDefsOverlap returns true if any signature in a has the same parameter
// types as any signature in b (ignoring param names, return types, and body).
func FnDefsOverlap(a, b FnDefInfo) bool {
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

// BaseValue returns the zero/default value for a given type, similar to Go's
// zero values. Used by both the "base" word and "make" with base:true option.
func BaseValue(t *Type) (Value, error) {
	switch {
	case t.Matches(TInteger):
		return NewInteger(0), nil
	case t.Matches(TDecimal):
		return NewDecimal(0), nil
	case t.Matches(TNumber):
		return NewInteger(0), nil
	case t.Matches(TString):
		return NewString(""), nil
	case t.Matches(TBoolean):
		return NewBoolean(false), nil
	case t.Matches(TList):
		return NewList([]Value{}), nil
	case t.Matches(TMap):
		return NewMap(NewOrderedMap()), nil
	case t.Matches(TNone):
		return NewTypeLiteral(TNone), nil
	case t.Matches(TAtom):
		return NewAtom(""), nil
	default:
		return Value{}, fmt.Errorf("base: unsupported type %s", t.String())
	}
}

// BaseValueForConstraint returns the base value for a field constraint.
// For type literals, returns the zero value directly.
// For disjunctions (e.g. string|none), returns the base of the first
// non-none alternative.
func BaseValueForConstraint(constraint Value) (Value, error) {
	if IsDisjunct(constraint) {
		di, _ := AsDisjunct(constraint)
		for _, alt := range di.Alternatives {
			if alt.Data == nil && !alt.Parent.Equal(TNone) {
				return BaseValue(alt.Parent)
			}
		}
		return NewTypeLiteral(TNone), nil
	}
	if constraint.Data == nil {
		return BaseValue(constraint.Parent)
	}
	return Value{}, fmt.Errorf("base: cannot determine base value for %s", constraint.String())
}

// omittedDefaultValue returns the value substituted for an omitted
// optional FnParam. Options-typed params get a Map populated with
// each Field's concrete default (fields whose value is a type body —
// type literals, disjuncts, nested type definitions — carry no
// default and are skipped). Non-Options params fall back to BaseValue
// of the param's declared Type.
func omittedDefaultValue(p FnParam) (Value, error) {
	if p.Pattern != nil && IsOptionsType(*p.Pattern) {
		oi, err := AsOptionsType(*p.Pattern)
		if err == nil && oi.Fields != nil {
			m := NewOrderedMap()
			for _, k := range oi.Fields.Keys() {
				fv, _ := oi.Fields.Get(k)
				if IsTypeBody(fv) {
					continue
				}
				m.Set(k, fv)
			}
			return NewMap(m), nil
		}
	}
	return BaseValue(p.Type)
}

// ExpandOptionalSigs takes a list of fn signatures and expands those with
// optional params into the full set of overloaded signatures. Each
// optional combination becomes its own signature whose body calls the
// original function with the omitted params filled by their type's
// base value.
func ExpandOptionalSigs(name string, sigs []FnSig) []FnSig {
	var expanded []FnSig
	for _, sig := range sigs {
		expanded = append(expanded, sig)

		var optIndices []int
		for i, p := range sig.Params {
			if p.Optional {
				optIndices = append(optIndices, i)
			}
		}
		if len(optIndices) == 0 {
			continue
		}

		numOpt := len(optIndices)
		for mask := 1; mask < (1 << numOpt); mask++ {
			omitted := make(map[int]bool)
			for bit := 0; bit < numOpt; bit++ {
				if mask&(1<<bit) != 0 {
					omitted[optIndices[bit]] = true
				}
			}

			var reducedParams []FnParam
			for i, p := range sig.Params {
				if !omitted[i] {
					reducedParams = append(reducedParams, FnParam{
						Name:    p.Name,
						Type:    p.Type,
						Pattern: p.Pattern,
					})
				}
			}

			var body []Value
			body = append(body, NewWord(name))
			presentIdx := 0
			for i, p := range sig.Params {
				if omitted[i] {
					bv, err := omittedDefaultValue(p)
					if err != nil {
						continue
					}
					body = append(body, bv)
				} else {
					if p.Name != "" {
						body = append(body, NewWord(p.Name))
					} else {
						body = append(body,
							NewOpenParen(),
							NewWord("args"),
							NewAtom(fmt.Sprintf("%d", presentIdx)),
							NewWord("get"),
							NewCloseParen(),
						)
					}
					presentIdx++
				}
			}

			expanded = append(expanded, FnSig{
				Params:  reducedParams,
				Returns: sig.Returns,
				Body:    body,
			})
		}
	}
	return expanded
}
