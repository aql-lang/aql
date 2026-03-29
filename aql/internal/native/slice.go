package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// sliceFunc returns the "slice" native function definition.
// slice has forward precedence and three signatures:
//   - [any, integer, integer] — slices the value from start to end
//   - [any, integer]          — slices the value from start
//   - [any]                   — returns the value unchanged
func sliceFunc() NativeFunc {
	return NativeFunc{
		Name:             "slice",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TInteger, engine.TInteger},
				Handler: sliceStartEndHandler,
			},
			{
				Args:    []engine.Type{engine.TAny, engine.TInteger},
				Handler: sliceStartHandler,
			},
			{
				Args:    []engine.Type{engine.TAny},
				Handler: sliceAllHandler,
			},
		},
	}
}

// sliceAllHandler calls voxgigstruct.Slice with no start/end arguments.
func sliceAllHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Slice(data)
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []engine.Value{val}, nil
}

// sliceStartHandler calls voxgigstruct.Slice with a start index.
func sliceStartHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	start := args[1].AsInteger()
	result := voxgigstruct.Slice(data, int(start))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []engine.Value{val}, nil
}

// sliceStartEndHandler calls voxgigstruct.Slice with start and end indices.
func sliceStartEndHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	start := args[1].AsInteger()
	end := args[2].AsInteger()
	result := voxgigstruct.Slice(data, int(start), int(end))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []engine.Value{val}, nil
}
