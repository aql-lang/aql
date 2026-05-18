package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "join" word is registered via the consolidated Natives slice in
// natives.go.
//
// joinDefaultHandler calls voxgigstruct.Join with default separator (comma).
func joinDefaultHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("join: expected list, got %T", data)
	}
	result := voxgigstruct.Join(arr)
	return []Value{NewString(result)}, nil
}

// joinSepHandler calls voxgigstruct.Join with a specified separator.
func joinSepHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	sep, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("join: separator: %w", err)
	}
	data := valueToAny(args[1])
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("join: expected list, got %T", data)
	}
	result := voxgigstruct.Join(arr, sep)
	return []Value{NewString(result)}, nil
}
