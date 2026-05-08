package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "pad" word is registered via the consolidated Natives slice in
// natives.go.
//
// padDefaultHandler calls voxgigstruct.Pad with default width.
func padDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Pad(data)
	return []engine.Value{engine.NewString(result)}, nil
}

// padWidthHandler calls voxgigstruct.Pad with a specified width.
func padWidthHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	width, err := args[1].AsConcreteInteger()
	if err != nil {
		return nil, err
	}
	result := voxgigstruct.Pad(data, int(width))
	return []engine.Value{engine.NewString(result)}, nil
}
