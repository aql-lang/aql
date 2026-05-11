package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "validate" word is registered via the consolidated Natives slice in
// natives.go.
//
// validateHandler calls voxgigstruct.Validate on data with the given spec.
// Returns the validated data, or an error if validation fails.
func validateHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	spec := valueToAny(args[0])
	data := valueToAny(args[1])

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
