package native

import (
	"fmt"
	"math"

	voxgigstruct "github.com/voxgig/struct"
)

// The "flatten" word is registered via the consolidated Natives slice in
// natives.go.
//
// flattenDefaultHandler calls voxgigstruct.Flatten with default depth (1).
func flattenDefaultHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Flatten(data)
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []Value{val}, nil
}

// fullFlattenDepth is the depth used for `flatten -1` (deep flatten): a
// value larger than any realistic nesting depth.
const fullFlattenDepth = math.MaxInt32

// flattenDepthHandler calls voxgigstruct.Flatten with an explicit depth.
// A negative depth (canonically -1) means "fully flatten": every level of
// nesting is removed. This is the deep-flatten operation — there is no
// separate array-module word for it.
func flattenDepthHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	depth, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, fmt.Errorf("flatten: depth: %w", err)
	}
	data := valueToAny(args[1])
	d := int(depth)
	if d < 0 {
		// _flattenDepth recurses only into real sublists (bounded by the
		// actual nesting), so a depth larger than any nesting fully
		// flattens without unbounded recursion.
		d = fullFlattenDepth
	}
	result := voxgigstruct.Flatten(data, d)
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []Value{val}, nil
}
