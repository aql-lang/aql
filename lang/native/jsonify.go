package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "jsonify" word is registered via the consolidated Natives slice in
// natives.go.
//
// jsonifyDefaultHandler calls voxgigstruct.Jsonify with default settings.
func jsonifyDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Jsonify(data)
	return []engine.Value{engine.NewString(result)}, nil
}

// jsonifyFlagsHandler calls voxgigstruct.Jsonify with a flags map.
func jsonifyFlagsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	flags, ok := valueToAny(args[0]).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("jsonify: expected map for flags, got %T", valueToAny(args[0]))
	}
	data := valueToAny(args[1])
	result := voxgigstruct.Jsonify(data, flags)
	return []engine.Value{engine.NewString(result)}, nil
}
