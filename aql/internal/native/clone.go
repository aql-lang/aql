package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// cloneHandler calls voxgigstruct.Clone to produce a deep copy.
// The "clone" word is registered via the consolidated Natives slice in
// natives.go.
func cloneHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])

	result := voxgigstruct.Clone(data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}
	return []engine.Value{val}, nil
}
