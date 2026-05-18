package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "inject" word is registered via the consolidated Natives slice in
// natives.go.
//
// injectHandler calls voxgigstruct.Inject to resolve path references.
func injectHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	val := valueToAny(args[0])
	store := valueToAny(args[1])

	result := voxgigstruct.Inject(val, store)

	out, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("inject: %w", err)
	}
	return []Value{out}, nil
}
