package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "getpath" word is registered via the consolidated Natives slice in
// natives.go.
//
// getpathHandler calls voxgigstruct.GetPath to retrieve a nested value.
func getpathHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	path, err := args[1].AsConcreteString()
	if err != nil {
		return nil, err
	}

	result := voxgigstruct.GetPath(path, data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("getpath: %w", err)
	}
	return []engine.Value{val}, nil
}
