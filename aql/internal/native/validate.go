package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// validateFunc returns the "validate" native function definition.
// validate has forward precedence and one signature:
//   - [any, map] — validates data against a spec using voxgig struct Validate
func validateFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:             "validate",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TMap},
				Handler: validateHandler,
			},
		},
	}
}

// validateHandler calls voxgigstruct.Validate on data with the given spec.
// Returns the validated data, or an error if validation fails.
func validateHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	spec := valueToAny(args[1])

	result, err := voxgigstruct.Validate(data, spec)
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	val, convErr := anyToValue(result)
	if convErr != nil {
		return nil, fmt.Errorf("validate: %w", convErr)
	}
	return []engine.Value{val}, nil
}
