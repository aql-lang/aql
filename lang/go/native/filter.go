package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "filter" word is registered via the consolidated Natives slice in
// natives.go.
//
// filterHandler calls voxgigstruct.Filter with an AQL callback as predicate.
// The callback receives a map with "key" and "value" fields and should return
// a boolean indicating whether to keep the item.
func filterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	cb := args[0]
	data := valueToAny(args[1])

	var callErr error
	result := voxgigstruct.Filter(data, func(pair [2]any) bool {
		if callErr != nil {
			return false
		}

		item := NewOrderedMap()

		keyVal, err := anyToValue(pair[0])
		if err != nil {
			keyVal = NewString(fmt.Sprintf("%v", pair[0]))
		}
		item.Set("key", keyVal)

		valVal, err := anyToValue(pair[1])
		if err != nil {
			valVal = NewString(fmt.Sprintf("%v", pair[1]))
		}
		item.Set("value", valVal)

		cbArgs := []Value{NewMap(item)}
		cbSig := MatchFnSig(cb, cbArgs)
		if cbSig == nil {
			callErr = fmt.Errorf("filter: no matching callback signature")
			return false
		}
		cbResult, err := r.CallAQL(cbSig, cbArgs)
		if err != nil {
			callErr = err
			return false
		}
		if len(cbResult) > 0 && cbResult[0].VType.Matches(TBoolean) {
			b, _ := AsBoolean(cbResult[0])
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
	return []Value{val}, nil
}
