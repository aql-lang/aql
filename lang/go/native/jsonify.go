package native

import (
	"fmt"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "jsonify" word is registered via the consolidated Natives slice in
// natives.go.
//
// Custom-type projection hook: before serialising, the handlers call
// eng.NodifyValue, which walks the value's type chain looking for a
// Nodifier behavior installed via `behave nodify/q (fn [[T] [Any]
// [body]])`. The body produces a Node or Scalar (data-shape, not a
// JSON string); the serialiser then encodes that. With no custom
// behavior the value passes through unchanged, preserving the
// pre-existing semantics for plain Maps / Lists / scalars.
//
// jsonifyDefaultHandler calls voxgigstruct.Jsonify with default settings.
func jsonifyDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	projected, err := eng.NodifyValue(args[0])
	if err != nil {
		return nil, err
	}
	data := valueToAny(projected)
	result := voxgigstruct.Jsonify(data)
	return []engine.Value{engine.NewString(result)}, nil
}

// jsonifyFlagsHandler calls voxgigstruct.Jsonify with a flags map.
func jsonifyFlagsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	flags, ok := valueToAny(args[0]).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("jsonify: expected map for flags, got %T", valueToAny(args[0]))
	}
	projected, err := eng.NodifyValue(args[1])
	if err != nil {
		return nil, err
	}
	data := valueToAny(projected)
	result := voxgigstruct.Jsonify(data, flags)
	return []engine.Value{engine.NewString(result)}, nil
}
