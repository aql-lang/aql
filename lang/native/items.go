package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/lang/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "items" word is registered via the consolidated Natives slice in
// natives.go.
//
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
