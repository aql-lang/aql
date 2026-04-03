package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// selectorFunc returns the "selector" native function definition.
// selector has forward precedence and one signature:
//   - [any, map] — selects children matching the query map using voxgig struct Select
func selectorFunc() NativeFunc {
	return NativeFunc{
		Name:             "selector",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TMap},
				Handler: selectorHandler,
			},
		},
	}
}

// selectorHandler calls voxgig struct Select, converting between
// engine.Value and Go any types.
func selectorHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	children := valueToAny(args[0])
	query := valueToAny(args[1])

	result := voxgigstruct.Select(children, query)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}
	return []engine.Value{val}, nil
}
