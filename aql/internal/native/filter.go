package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// filterFunc returns the "filter" native function definition.
// filter has forward precedence and one signature:
//   - [any, function] — filters the value using the callback as predicate
func filterFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:             "filter",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TFunction},
				Handler: filterHandler,
			},
		},
	}
}

// filterHandler calls voxgigstruct.Filter with an AQL callback as predicate.
// The callback receives a map with "key" and "value" fields and should return
// a boolean indicating whether to keep the item.
func filterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	cb := args[1]

	var callErr error
	result := voxgigstruct.Filter(data, func(pair [2]any) bool {
		if callErr != nil {
			return false
		}

		item := engine.NewOrderedMap()

		keyVal, err := anyToValue(pair[0])
		if err != nil {
			keyVal = engine.NewString(fmt.Sprintf("%v", pair[0]))
		}
		item.Set("key", keyVal)

		valVal, err := anyToValue(pair[1])
		if err != nil {
			valVal = engine.NewString(fmt.Sprintf("%v", pair[1]))
		}
		item.Set("value", valVal)

		cbArgs := []engine.Value{engine.NewMap(item)}
		cbSig := engine.MatchFnSig(cb, cbArgs)
		if cbSig == nil {
			callErr = fmt.Errorf("filter: no matching callback signature")
			return false
		}
		cbResult, err := r.CallAQL(cbSig, cbArgs)
		if err != nil {
			callErr = err
			return false
		}
		if len(cbResult) > 0 && cbResult[0].VType.Matches(engine.TBoolean) {
			b, _ := cbResult[0].AsBoolean()
			return b
		}
		return false
	})

	if callErr != nil {
		return nil, fmt.Errorf("filter: callback error: %w", callErr)
	}

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	return []engine.Value{val}, nil
}
