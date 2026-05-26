package eng

// ResolveRef looks up name in the registry and returns the value-form
// of its current binding without invoking. Resolution mirrors the
// priority used by stepWord:
//
//  1. A type binding (capitalised def, refine-prefab) returns its
//     stored body — typically a type literal.
//  2. A value binding returns the bound value: an FnDef binding is
//     wrapped as a Function value and marked Quoted so it sits on the
//     stack as data rather than auto-executing; everything else is
//     returned as-is (also Quoted when its parent is TFnDef/TFunction,
//     so type bindings whose body is an fn value behave the same).
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
		out := tv
		if out.Parent.Equal(TFnDef) || out.Parent.Equal(TFunction) {
			out.Quoted = true
		}
		return out, true
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return Value{}, false
	}
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		v := NewFunction(fnDef)
		v.Quoted = true
		return v, true
	}
	out := top
	if out.Parent.Equal(TFnDef) || out.Parent.Equal(TFunction) {
		out.Quoted = true
	}
	return out, true
}
