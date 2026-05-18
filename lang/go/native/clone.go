package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// cloneHandler calls voxgigstruct.Clone to produce a deep copy.
// The "clone" word is registered via the consolidated Natives slice in
// natives.go.
func cloneHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])

	result := voxgigstruct.Clone(data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}
	return []Value{val}, nil
}
