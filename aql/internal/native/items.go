package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// itemsFunc returns the "items" native function definition.
// items has suffix precedence and one signature:
//   - [any] — returns key-value pairs as a list of [key, value] lists
func itemsFunc() NativeFunc {
	return NativeFunc{
		Name:             "items",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: itemsHandler,
			},
		},
	}
}

// itemsHandler calls voxgigstruct.Items and converts the result to a list of
// two-element lists [key, value].
func itemsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	pairs := voxgigstruct.Items(data)

	result := make([]engine.Value, len(pairs))
	for i, pair := range pairs {
		keyStr, ok := pair[0].(string)
		if !ok {
			keyStr = fmt.Sprintf("%v", pair[0])
		}
		keyVal := engine.NewString(keyStr)
		valVal, err := anyToValue(pair[1])
		if err != nil {
			valVal = engine.NewString("")
		}
		result[i] = engine.NewList([]engine.Value{keyVal, valVal})
	}

	return []engine.Value{engine.NewList(result)}, nil
}
