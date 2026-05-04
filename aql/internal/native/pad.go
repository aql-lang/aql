package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// padFunc returns the "pad" native function definition.
// pad has forward precedence and two signatures:
//   - [any, integer] — pads the string representation to the given width
//   - [any]          — pads the string representation to the default width
func RegisterPad(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "pad",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TInteger},
				Handler: padWidthHandler,
			},
			{
				Args:    []engine.Type{engine.TAny},
				Handler: padDefaultHandler,
			},
		},
	})
}

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
