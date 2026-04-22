package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// cloneFunc returns the "clone" native function definition.
// clone is stack-only and has one signature:
//   - [any] — deep-clones the value
func RegisterClone(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "clone",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: cloneHandler,
			},
		},
	})
}

// cloneHandler calls voxgigstruct.Clone to produce a deep copy.
func cloneHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])

	result := voxgigstruct.Clone(data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}
	return []engine.Value{val}, nil
}
