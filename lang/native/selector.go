package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "selector" word is registered via the consolidated Natives slice in
// natives.go.
//
// selectorHandler calls voxgig struct Select, converting between
// engine.Value and Go any types.
func selectorHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	query := valueToAny(args[0])
	children := valueToAny(args[1])

	result := voxgigstruct.Select(children, query)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}
	return []engine.Value{val}, nil
}
