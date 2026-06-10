package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// cloneHandler produces a deep copy. A class / Object instance clones
// type-preservingly to a fresh instance of the same type — deq-equal
// to the original but eq-distinct (the field map is new), so a clone
// can diverge without aliasing the source. Plain data routes through
// voxgigstruct.Clone as before.
// The "clone" word is registered via the consolidated Natives slice in
// natives.go.
func cloneHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	if IsObjectInstance(args[0]) {
		return []Value{cloneValueTyped(args[0])}, nil
	}
	data := valueToAny(args[0])

	result := voxgigstruct.Clone(data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}
	return []Value{val}, nil
}

// cloneValueTyped deep-copies v preserving instance types: a class /
// Object instance becomes a fresh instance of the same type with a
// new field map (field values cloned recursively), maps and lists
// clone element-wise, and immutable scalars pass through as-is.
func cloneValueTyped(v Value) Value {
	switch {
	case IsObjectInstance(v):
		info, _ := AsObjectInstance(v)
		src := ObjectFields(&info)
		fields := NewOrderedMap()
		for _, k := range src.Keys() {
			fv, _ := src.Get(k)
			fields.Set(k, cloneValueTyped(fv))
		}
		return NewObjectInstance(v.Parent, ObjectInstanceInfo{
			TypeRef: info.TypeRef,
			Fields:  fields,
		})
	case v.Parent.ConformsTo(TMap) && IsConcrete(v):
		src, _ := AsMap(v)
		out := NewOrderedMap()
		for _, k := range src.Keys() {
			mv, _ := src.Get(k)
			out.Set(k, cloneValueTyped(mv))
		}
		return NewMap(out)
	case v.Parent.ConformsTo(TList) && IsConcrete(v):
		lst, _ := AsList(v)
		cp := make([]Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			cp[i] = cloneValueTyped(lst.Get(i))
		}
		return NewList(cp)
	}
	return v
}
