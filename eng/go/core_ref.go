package eng

// ResolveRef looks up name in the registry and returns the value-form
// of its current binding without invoking. Resolution mirrors the
// priority used by stepWord:
//
//  1. A type binding (capitalised def, refine-prefab) returns its
//     stored body — typically a type literal.
//  2. A value binding returns the bound value: an FnDef binding is
//     wrapped as a Function value; every other binding is returned
//     as-is.
//
// The returned Function value is UNQUOTED. Under the dispatch rules
// of this engine, an unquoted Function on the stack is a live call
// site — full signature matching (forward + stack) applies the next
// time the engine processes it. To capture as inert data, wrap with
// `quote` at the call site.
//
// The second return is false when the name is not bound at all. The
// caller decides how to report the failure — the `ref` word raises
// an undefined_word error, the /r short-circuit in stepWord does the
// same.
//
// Lives in eng because stepWord's /r path needs it during the run
// loop; the `ref` word itself is registered in the language layer.
func ResolveRef(r *Registry, name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if tv, ok := r.TopTypeBody(name); ok {
		return tv, true
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return Value{}, false
	}
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		return NewFunction(fnDef), true
	}
	return top, true
}

// IsFunctionRef reports whether a value resolved by ResolveRef is a
// function word — the only binding kind `/r` and `ref` are permitted to
// reference. The reference surfaces exist to break the asymmetry between
// value bindings (a bare name already pushes the value) and fn bindings
// (a bare name invokes); for a non-fn binding there is no asymmetry to
// break, so referencing it is meaningless and rejected. ResolveRef wraps
// every FnDef binding as a Function value (Parent == TFunction), so the
// predicate is a single Parent check; plain values and type bodies come
// back with their own Parent and are illegal ref targets.
func IsFunctionRef(v Value) bool {
	return v.Parent.Equal(TFunction)
}
