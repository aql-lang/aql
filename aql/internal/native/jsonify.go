package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// jsonifyFunc returns the "jsonify" native function definition.
// jsonify has forward precedence and two signatures:
//   - [any, map] — converts the value to a JSON string with flags (indent, offset)
//   - [any]      — converts the value to a JSON string with defaults
func jsonifyFunc() NativeFunc {
	return NativeFunc{
		Name:             "jsonify",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TMap},
				Handler: jsonifyFlagsHandler,
			},
			{
				Args:    []engine.Type{engine.TAny},
				Handler: jsonifyDefaultHandler,
			},
		},
	}
}

// jsonifyDefaultHandler calls voxgigstruct.Jsonify with default settings.
func jsonifyDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Jsonify(data)
	return []engine.Value{engine.NewString(result)}, nil
}

// jsonifyFlagsHandler calls voxgigstruct.Jsonify with a flags map.
func jsonifyFlagsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	flags, ok := valueToAny(args[1]).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("jsonify: expected map for flags, got %T", valueToAny(args[1]))
	}
	result := voxgigstruct.Jsonify(data, flags)
	return []engine.Value{engine.NewString(result)}, nil
}
