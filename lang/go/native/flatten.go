package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "flatten" word is registered via the consolidated Natives slice in
// natives.go.
//
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
	depth, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, fmt.Errorf("flatten: depth: %w", err)
	}
	data := valueToAny(args[1])
	result := voxgigstruct.Flatten(data, int(depth))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []engine.Value{val}, nil
}
