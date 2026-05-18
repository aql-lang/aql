package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "items" word is registered via the consolidated Natives slice in
// natives.go.
//
// itemsHandler calls voxgigstruct.Items and converts the result to a list of
// two-element lists [key, value].
func itemsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])
	pairs := voxgigstruct.Items(data)

	result := make([]Value, len(pairs))
	for i, pair := range pairs {
		keyStr, ok := pair[0].(string)
		if !ok {
			keyStr = fmt.Sprintf("%v", pair[0])
		}
		keyVal := NewString(keyStr)
		valVal, err := anyToValue(pair[1])
		if err != nil {
			valVal = NewString("")
		}
		result[i] = NewList([]Value{keyVal, valVal})
	}

	return []Value{NewList(result)}, nil
}
