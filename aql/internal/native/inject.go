package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// injectFunc returns the "inject" native function definition.
// inject has forward precedence and one signature:
//   - [any, any] — resolves backtick-escaped path references in the first value
//     using the second value as the store
func RegisterInject(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "inject",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TAny},
				Handler: injectHandler,
			},
		},
	})
}

// injectHandler calls voxgigstruct.Inject to resolve path references.
func injectHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	val := valueToAny(args[0])
	store := valueToAny(args[1])

	result := voxgigstruct.Inject(val, store)

	out, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("inject: %w", err)
	}
	return []engine.Value{out}, nil
}
