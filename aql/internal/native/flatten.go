package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// flattenFunc returns the "flatten" native function definition.
// flatten has suffix precedence and two signatures:
//   - [list, integer] — flattens the list to the given depth
//   - [list]          — flattens the list by one level
func flattenFunc() NativeFunc {
	return NativeFunc{
		Name:             "flatten",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TInteger},
				Handler: flattenDepthHandler,
			},
			{
				Args:    []engine.Type{engine.TList},
				Handler: flattenDefaultHandler,
			},
		},
	}
}

// flattenDefaultHandler calls voxgigstruct.Flatten with default depth (1).
func flattenDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Flatten(data)
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []engine.Value{val}, nil
}

// flattenDepthHandler calls voxgigstruct.Flatten with a specified depth.
func flattenDepthHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	depth := args[1].AsInteger()
	result := voxgigstruct.Flatten(data, int(depth))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []engine.Value{val}, nil
}
