package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "selector" word is registered via the consolidated Natives slice in
// natives.go.
//
// selectorHandler calls voxgig struct Select, converting between
// Value and Go any types.
func selectorHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	query := valueToAny(args[0])
	children := valueToAny(args[1])

	result := voxgigstruct.Select(children, query)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}
	return []Value{val}, nil
}
