package native
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// RegisterStringSlice registers the "slice" word for substring/sublist extraction.
//
// Signature: [(Integer Integer Node/String) | (Integer Node/String) | (Node/String)]
//
// Usage:
//
//	"hello" slice 1 3   => "el"   (start=1, end=3)
//	"hello" slice 2     => "llo"  (start=2, to end)
//	[10 20 30] slice 0 2 => [10 20]
func RegisterStringSlice(r *engine.Registry) {
	// 3-arg: slice start end data
	// Forward-first: args[0]=start, args[1]=end, args[2]=data (stack).
	sliceStartEndHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
	sliceStartHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
		_as2, _ := args[0].AsInteger()
		start := int(_as2)
		data := valueToSliceArg(args[1])
		result := voxgigstruct.Slice(data, start)
		return sliceResult(result)
	}

	// 1-arg: slice data (identity/copy)
	sliceAllHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
		data := valueToSliceArg(args[0])
		result := voxgigstruct.Slice(data)
		return sliceResult(result)
	}

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "slice",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TString}, Handler: sliceStartEndHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TList}, Handler: sliceStartEndHandler, Returns: []engine.Type{engine.TList}},
			{Args: []engine.Type{engine.TInteger, engine.TString}, Handler: sliceStartHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TInteger, engine.TList}, Handler: sliceStartHandler, Returns: []engine.Type{engine.TList}},
			{Args: []engine.Type{engine.TString}, Handler: sliceAllHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TList}, Handler: sliceAllHandler, Returns: []engine.Type{engine.TList}},
		},
	})
}

// valueToSliceArg converts a Value to the interface{} expected by voxgigstruct.Slice.
func valueToSliceArg(v engine.Value) interface{} {
	if v.VType.Matches(engine.TString) {
		_as3, _ := v.AsString()
		return _as3
	}
	if v.VType.Matches(engine.TList) {
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

// sliceResultItem converts a single item from a sliced list.
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
