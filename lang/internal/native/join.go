package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "join" word is registered via the consolidated Natives slice in
// natives.go.
//
// joinDefaultHandler calls voxgigstruct.Join with default separator (comma).
func joinDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("join: expected list, got %T", data)
	}
	result := voxgigstruct.Join(arr)
	return []engine.Value{engine.NewString(result)}, nil
}

// joinSepHandler calls voxgigstruct.Join with a specified separator.
func joinSepHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
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
	return []engine.Value{engine.NewString(result)}, nil
}
