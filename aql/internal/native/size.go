package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// sizeFunc returns the "size" native function definition.
// size has forward precedence and one signature:
//   - [any] — returns the size/length of the value
func sizeFunc() NativeFunc {
	return NativeFunc{
		Name:             "size",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: sizeHandler,
			},
		},
	}
}

// sizeHandler calls voxgigstruct.Size to get the size of a value.
func sizeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Size(data)
	return []engine.Value{engine.NewInteger(int64(result))}, nil
}
