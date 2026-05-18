package native

import (
	voxgigstruct "github.com/voxgig/struct"
)

// The "pad" word is registered via the consolidated Natives slice in
// natives.go.
//
// padDefaultHandler calls voxgigstruct.Pad with default width.
func padDefaultHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Pad(data)
	return []Value{NewString(result)}, nil
}

// padWidthHandler calls voxgigstruct.Pad with a specified width.
func padWidthHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	width, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, err
	}
	data := valueToAny(args[1])
	result := voxgigstruct.Pad(data, int(width))
	return []Value{NewString(result)}, nil
}
