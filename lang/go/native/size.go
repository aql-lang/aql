package native

import (
	voxgigstruct "github.com/voxgig/struct"
)

// The "size" word is registered via the consolidated Natives slice in
// natives.go.
//
// sizeHandler calls voxgigstruct.Size to get the size of a value.
func sizeHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])
	result := voxgigstruct.Size(data)
	return []Value{NewInteger(int64(result))}, nil
}
