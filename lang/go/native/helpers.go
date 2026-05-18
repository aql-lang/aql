package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/engine"
)

// valueToSliceArg converts a Value into the interface{} expected by
// voxgigstruct.Slice: strings flow through unchanged, lists become
// []interface{} with element-wise conversion, anything else falls back
// to its String() form. Used by stringSliceNative in natives.go.
func valueToSliceArg(v engine.Value) interface{} {
	if v.VType.Matches(engine.TString) {
		_as3, _ := engine.AsString(v)
		return _as3
	}
	if v.VType.Matches(engine.TList) {
		list, _ := engine.AsList(v)
		result := make([]interface{}, list.Len())
		for i, elem := range list.Slice() {
			result[i] = valueToSliceArg(elem)
		}
		return result
	}
	return v.String()
}

// sliceResult converts a voxgigstruct.Slice result back into engine
// values. Strings stay strings; []interface{} becomes a List of converted
// items via sliceResultItem; nil becomes a None type literal.
func sliceResult(result interface{}) ([]engine.Value, error) {
	switch r := result.(type) {
	case string:
		return []engine.Value{engine.NewString(r)}, nil
	case []interface{}:
		vals := make([]engine.Value, len(r))
		for i, item := range r {
			v, err := sliceResultItem(item)
			if err != nil {
				return nil, err
			}
			vals[i] = v
		}
		return []engine.Value{engine.NewList(vals)}, nil
	case nil:
		return []engine.Value{engine.NewTypeLiteral(engine.TNone)}, nil
	default:
		return nil, fmt.Errorf("slice: unexpected result type %T", result)
	}
}

// sliceResultItem converts a single item produced by voxgigstruct.Slice
// into an engine.Value, recursing into nested []interface{} slices.
func sliceResultItem(item interface{}) (engine.Value, error) {
	switch v := item.(type) {
	case string:
		return engine.NewString(v), nil
	case int:
		return engine.NewInteger(int64(v)), nil
	case int64:
		return engine.NewInteger(v), nil
	case float64:
		return engine.NewDecimal(v), nil
	case bool:
		return engine.NewBoolean(v), nil
	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil
	case []interface{}:
		vals := make([]engine.Value, len(v))
		for i, elem := range v {
			val, err := sliceResultItem(elem)
			if err != nil {
				return engine.Value{}, err
			}
			vals[i] = val
		}
		return engine.NewList(vals), nil
	default:
		return engine.Value{}, fmt.Errorf("slice: unsupported element type %T", item)
	}
}
