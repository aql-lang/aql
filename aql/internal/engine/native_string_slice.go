package engine

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// registerSlice registers the "slice" word for substring/sublist extraction.
//
// Signature: [(Integer Integer Node/String) | (Integer Node/String) | (Node/String)]
//
// Usage:
//
//	"hello" slice 1 3   => "el"   (start=1, end=3)
//	"hello" slice 2     => "llo"  (start=2, to end)
//	[10 20 30] slice 0 2 => [10 20]
func registerSlice(r *Registry) {
	// 3-arg: slice start end data
	// Forward-first: args[0]=start, args[1]=end, args[2]=data (stack).
	sliceStartEndHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsInteger()
		start := int(_as0)
		_as1, _ := args[1].AsInteger()
		end := int(_as1)
		data := valueToSliceArg(args[2])
		result := voxgigstruct.Slice(data, start, end)
		return sliceResult(result)
	}

	// 2-arg: slice start data
	// Forward-first: args[0]=start, args[1]=data (stack).
	sliceStartHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		start := int(_as2)
		data := valueToSliceArg(args[1])
		result := voxgigstruct.Slice(data, start)
		return sliceResult(result)
	}

	// 1-arg: slice data (identity/copy)
	sliceAllHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		data := valueToSliceArg(args[0])
		result := voxgigstruct.Slice(data)
		return sliceResult(result)
	}

	r.Register("slice",
		Signature{Args: []Type{TInteger, TInteger, TString}, Handler: sliceStartEndHandler},
		Signature{Args: []Type{TInteger, TInteger, TList}, Handler: sliceStartEndHandler},
		Signature{Args: []Type{TInteger, TString}, Handler: sliceStartHandler},
		Signature{Args: []Type{TInteger, TList}, Handler: sliceStartHandler},
		Signature{Args: []Type{TString}, Handler: sliceAllHandler},
		Signature{Args: []Type{TList}, Handler: sliceAllHandler},
	)
}

// valueToSliceArg converts a Value to the interface{} expected by voxgigstruct.Slice.
func valueToSliceArg(v Value) interface{} {
	if v.VType.Matches(TString) {
		_as3, _ := v.AsString()
		return _as3
	}
	if v.VType.Matches(TList) {
		list := v.AsList()
		result := make([]interface{}, list.Len())
		for i, elem := range list.Slice() {
			result[i] = valueToSliceArg(elem)
		}
		return result
	}
	return v.String()
}

// sliceResult converts a voxgigstruct.Slice result back to engine Values.
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

// sliceResultItem converts a single item from a sliced list.
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
