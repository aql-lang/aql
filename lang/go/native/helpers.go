package native

import (
	"fmt"
)

// valueToSliceArg converts a Value into the interface{} expected by
// voxgigstruct.Slice: strings flow through unchanged, lists become
// []interface{} with element-wise conversion, anything else falls back
// to its String() form. Used by stringSliceNative in natives.go.
func valueToSliceArg(v Value) interface{} {
	if v.VType.Matches(TString) {
		_as3, _ := AsString(v)
		return _as3
	}
	if v.VType.Matches(TList) {
		list, _ := AsList(v)
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
func sliceResult(result interface{}) ([]Value, error) {
	switch r := result.(type) {
	case string:
		return []Value{NewString(r)}, nil
	case []interface{}:
		vals := make([]Value, len(r))
		for i, item := range r {
			v, err := sliceResultItem(item)
			if err != nil {
				return nil, err
			}
			vals[i] = v
		}
		return []Value{NewList(vals)}, nil
	case nil:
		return []Value{NewTypeLiteral(TNone)}, nil
	default:
		return nil, fmt.Errorf("slice: unexpected result type %T", result)
	}
}

// sliceResultItem converts a single item produced by voxgigstruct.Slice
// into an Value, recursing into nested []interface{} slices.
func sliceResultItem(item interface{}) (Value, error) {
	switch v := item.(type) {
	case string:
		return NewString(v), nil
	case int:
		return NewInteger(int64(v)), nil
	case int64:
		return NewInteger(v), nil
	case float64:
		return NewDecimal(v), nil
	case bool:
		return NewBoolean(v), nil
	case nil:
		return NewTypeLiteral(TNone), nil
	case []interface{}:
		vals := make([]Value, len(v))
		for i, elem := range v {
			val, err := sliceResultItem(elem)
			if err != nil {
				return Value{}, err
			}
			vals[i] = val
		}
		return NewList(vals), nil
	default:
		return Value{}, fmt.Errorf("slice: unsupported element type %T", item)
	}
}
