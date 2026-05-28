package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "transform" word is registered via the consolidated Natives slice
// in natives.go. This file keeps the transform handler plus the shared
// Value↔any conversion helpers (valueToAny / anyToValue / valueToMap)
// used across the package.
//
// transformHandler calls voxgig struct Transform.
// args[0]=spec (Map, forward-collected), args[1]=data (Any, from stack).
func transformHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	spec := valueToAny(args[0])
	data := valueToAny(args[1])

	result := voxgigstruct.Transform(data, spec)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	return []Value{val}, nil
}

// ValueToAny is the exported entry point to the value→Go-any
// conversion. The unexported valueToAny is the in-package alias
// kept for existing handler call sites; new external callers (e.g.
// lang/go/modules) should use this name.
func ValueToAny(v Value) any { return valueToAny(v) }

// valueToAny converts an Value to a Go any for use with voxgig struct.
func valueToAny(v Value) any {
	// Type literals (e.g. bare Map, List, Integer) have Data==nil.
	// Return nil rather than panicking on accessor calls.
	if !IsConcrete(v) {
		return nil
	}
	switch {
	case v.Parent.Matches(TInteger):
		i, _ := AsInteger(v)
		return float64(i)
	case v.Parent.Matches(TString):
		s, _ := AsString(v)
		return s
	case v.Parent.Matches(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case v.Parent.Equal(TAtom):
		a, _ := AsAtom(v)
		return a
	case v.Parent.Equal(TNone):
		return nil
	case v.Parent.Matches(TMap):
		m, _ := AsMap(v)
		out := make(map[string]any, m.Len())
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			out[key] = valueToAny(val)
		}
		return out
	case v.Parent.Matches(TList):
		_lst, _ := AsList(v)
		elems := _lst.Slice()
		out := make([]any, len(elems))
		for i, elem := range elems {
			out[i] = valueToAny(elem)
		}
		return out
	default:
		return v.String()
	}
}

// anyToValue converts a Go any (as returned by voxgig struct) back to an Value.
func anyToValue(v any) (Value, error) {
	switch val := v.(type) {
	case nil:
		return NewTypeLiteral(TNone), nil
	case bool:
		return NewBoolean(val), nil
	case float64:
		return NewInteger(int64(val)), nil
	case int:
		return NewInteger(int64(val)), nil
	case int64:
		return NewInteger(val), nil
	case string:
		return NewString(val), nil
	case []any:
		elems := make([]Value, len(val))
		for i, item := range val {
			e, err := anyToValue(item)
			if err != nil {
				return Value{}, err
			}
			elems[i] = e
		}
		return NewList(elems), nil
	case map[string]any:
		om := NewOrderedMap()
		for _, key := range sortedAnyMapKeys(val) {
			child, err := anyToValue(val[key])
			if err != nil {
				return Value{}, err
			}
			om.Set(key, child)
		}
		return NewMap(om), nil
	default:
		return Value{}, fmt.Errorf("unsupported type from transform: %T", v)
	}
}

// valueToMap converts a map-typed Value to map[string]any for use with SDK calls.
func valueToMap(v Value) map[string]any {
	m, _ := AsMap(v)
	if m == nil {
		return nil
	}
	out := make(map[string]any, m.Len())
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		out[key] = valueToAny(val)
	}
	return out
}

// sortedAnyMapKeys returns map keys in sorted order for deterministic output.
func sortedAnyMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
