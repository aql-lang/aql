package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// sliceFunc returns the "slice" native function definition.
// slice has forward precedence and three signatures.
// Signatures use [Integer, Integer, Any] ordering so that forward-first
// rearrangement (forward args at positions 0..F-1, stack data last) aligns
// with positional matching.
func sliceFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:             "slice",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TInteger, engine.TInteger, engine.TAny},
				Handler: sliceStartEndHandler,
			},
			{
				Args:    []engine.Type{engine.TInteger, engine.TAny},
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
// With forward-first matching: args[0]=start (forward), args[1]=data (stack).
func sliceStartHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	start, err := args[0].AsInteger()
	if err != nil {
		return nil, fmt.Errorf("slice: start: %w", err)
	}
	data := valueToAny(args[1])
	result := voxgigstruct.Slice(data, int(start))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []engine.Value{val}, nil
}

// sliceStartEndHandler calls voxgigstruct.Slice with start and end indices.
// With forward-first matching: args[0]=start (forward), args[1]=end (forward),
// args[2]=data (stack).
func sliceStartEndHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	start, err := args[0].AsInteger()
	if err != nil {
		return nil, fmt.Errorf("slice: start: %w", err)
	}
	end, err := args[1].AsInteger()
	if err != nil {
		return nil, fmt.Errorf("slice: end: %w", err)
	}
	data := valueToAny(args[2])
	result := voxgigstruct.Slice(data, int(start), int(end))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []engine.Value{val}, nil
}
