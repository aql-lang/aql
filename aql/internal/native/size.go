package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "size" word is registered via the consolidated Natives slice in
// natives.go.
//
// sizeHandler calls voxgigstruct.Size to get the size of a value.
func sizeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Size(data)
	return []engine.Value{engine.NewInteger(int64(result))}, nil
}
