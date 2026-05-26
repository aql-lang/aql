package eng

import (
	"fmt"
)

// resolveRef looks up name in the registry and returns the value-form
// of its current binding without invoking. Resolution mirrors the
// priority in stepWord:
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
// caller decides how to report the failure — `ref` raises an
// undefined_word error, the stepWord short-circuit does the same.
func resolveRef(r *Registry, name string) (Value, bool) {
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

// RefHandler implements the `ref` word. The /q on its argument slot
// makes the parser capture the upcoming Word as an Atom rather than
// executing it, so `ref foo` arrives here with args[0] = Atom(foo).
// The handler resolves that name and returns its referent.
func RefHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ref: missing name")
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("ref: expected an atom name, got %s", args[0].Parent.String())
	}
	v, ok := resolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, reg.AqlError("undefined_word", "ref: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("ref: name %s is not bound", name)
	}
	return []Value{v}, nil
}

// registerCoreRef installs the kernel-level `ref` word. Called from
// NewRegistry so every registry — eng-only or lang-shimmed — has it
// available alongside `/r` on word names.
func registerCoreRef(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "ref",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:           []*Type{TAtom},
			QuoteArgs:      map[int]bool{0: true},
			Handler:        RefHandler,
			Returns:        []*Type{TAny},
			RunInCheckMode: true,
		}},
	})
}
