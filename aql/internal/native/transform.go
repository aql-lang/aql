package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// transformFunc returns the "transform" native function definition.
// transform has forward precedence with sig [Map, Any]:
//
//	data transform {spec}   — spec (Map) forward-collected → args[0], data from stack → args[1]
//
// The spec (transform template) is always the Map at sig[0].
func RegisterTransform(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "transform",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TMap, engine.TAny},
				Handler: transformHandler,
			},
		},
	})
}

// transformHandler calls voxgig struct Transform.
// args[0]=spec (Map, forward-collected), args[1]=data (Any, from stack).
func transformHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	spec := valueToAny(args[0])
	data := valueToAny(args[1])

	result := voxgigstruct.Transform(data, spec)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	return []engine.Value{val}, nil
}

// valueToAny converts an engine.Value to a Go any for use with voxgig struct.
func valueToAny(v engine.Value) any {
	// Type literals (e.g. bare Map, List, Integer) have Data==nil.
	// Return nil rather than panicking on accessor calls.
	if v.Data == nil {
		return nil
	}
	switch {
	case v.VType.Matches(engine.TInteger):
		i, _ := v.AsInteger()
		return float64(i)
	case v.VType.Matches(engine.TString):
		s, _ := v.AsString()
		return s
	case v.VType.Matches(engine.TBoolean):
		b, _ := v.AsBoolean()
		return b
	case v.VType.Equal(engine.TAtom):
		a, _ := v.AsAtom()
		return a
	case v.VType.Equal(engine.TNone):
		return nil
	case v.VType.Matches(engine.TMap):
		m := v.AsMap()
		out := make(map[string]any, m.Len())
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			out[key] = valueToAny(val)
		}
		return out
	case v.VType.Matches(engine.TList):
		elems := v.AsList().Slice()
		out := make([]any, len(elems))
		for i, elem := range elems {
			out[i] = valueToAny(elem)
		}
		return out
	default:
		return v.String()
	}
}

// anyToValue converts a Go any (as returned by voxgig struct) back to an engine.Value.
func anyToValue(v any) (engine.Value, error) {
	switch val := v.(type) {
	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil
	case bool:
		return engine.NewBoolean(val), nil
	case float64:
		return engine.NewInteger(int64(val)), nil
	case int:
		return engine.NewInteger(int64(val)), nil
	case int64:
		return engine.NewInteger(val), nil
	case string:
		return engine.NewString(val), nil
	case []any:
		elems := make([]engine.Value, len(val))
		for i, item := range val {
			e, err := anyToValue(item)
			if err != nil {
				return engine.Value{}, err
			}
			elems[i] = e
		}
		return engine.NewList(elems), nil
	case map[string]any:
		om := engine.NewOrderedMap()
		for _, key := range sortedAnyMapKeys(val) {
			child, err := anyToValue(val[key])
			if err != nil {
				return engine.Value{}, err
			}
			om.Set(key, child)
		}
		return engine.NewMap(om), nil
	default:
		return engine.Value{}, fmt.Errorf("unsupported type from transform: %T", v)
	}
}

// valueToMap converts a map-typed Value to map[string]any for use with SDK calls.
func valueToMap(v engine.Value) map[string]any {
	m := v.AsMap()
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
