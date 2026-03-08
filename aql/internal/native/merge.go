package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// mergeFunc returns the "merge" native function definition.
// merge has suffix precedence and one signature:
//   - [any, any] — deep-merges the second value into the first using voxgig struct Merge
func mergeFunc() NativeFunc {
	return NativeFunc{
		Name:             "merge",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TAny},
				Handler: mergeHandler,
			},
		},
	}
}

// mergeHandler calls voxgigstruct.Merge on two values, returning the merged result.
func mergeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	a := valueToAny(args[0])
	b := valueToAny(args[1])

	result := voxgigstruct.Merge([]any{a, b})

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}
	return []engine.Value{val}, nil
}
