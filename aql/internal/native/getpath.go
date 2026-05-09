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
	path, err := args[0].AsConcreteString()
	if err != nil {
		return nil, err
	}
	data := valueToAny(args[1])

	result := voxgigstruct.GetPath(path, data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("getpath: %w", err)
	}
	return []engine.Value{val}, nil
}
