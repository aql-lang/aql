package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct/go"
)

// The "inject" word is registered via the consolidated Natives slice in
// natives.go.
//
// injectHandler calls voxgigstruct.Inject to resolve path references.
func injectHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	val := valueToAny(args[0])
	store := valueToAny(args[1])

	result := voxgigstruct.Inject(val, store)

	out, err := structConvert(result)
	if err != nil {
		return nil, fmt.Errorf("inject: %w", err)
	}
	// #2 (round 3): inject rebuilds a plain node (valueToAny/structConvert),
	// dropping the tag — re-validate + re-tag against the data template's {:T}
	// constraint so a resolved value violating the element type is rejected and
	// downstream writes stay enforced (mirrors merge).
	tagged, terr := d2ReTagContainer(r, args[0], out, "inject")
	if terr != nil {
		return nil, terr
	}
	return []Value{tagged}, nil
}
