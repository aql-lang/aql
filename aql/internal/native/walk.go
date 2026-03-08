package native

import (
	"fmt"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// walkFunc returns the "walk" native function definition.
// walk is prefix-only and has one signature:
//   - [any] — depth-first walks the value and collects all leaf nodes
//     as a list of {path, value} maps
func walkFunc() NativeFunc {
	return NativeFunc{
		Name:             "walk",
		SuffixPrecedence: false,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: walkHandler,
			},
		},
	}
}

// walkHandler uses voxgigstruct.Walk to traverse the value depth-first,
// collecting each leaf node into a list of maps with "path" and "value" keys.
func walkHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value) ([]engine.Value, error) {
	data := valueToAny(args[0])

	var leaves []engine.Value

	voxgigstruct.Walk(data, func(key *string, val any, parent any, path []string) any {
		// Only collect leaf nodes (non-map, non-list values).
		if !voxgigstruct.IsNode(val) {
			leaf := engine.NewOrderedMap()

			pathStr := strings.Join(path, ".")
			leaf.Set("path", engine.NewString(pathStr))

			v, err := anyToValue(val)
			if err != nil {
				v = engine.NewString(fmt.Sprintf("%v", val))
			}
			leaf.Set("value", v)

			leaves = append(leaves, engine.NewMap(leaf))
		}
		return val
	})

	if leaves == nil {
		leaves = []engine.Value{}
	}
	return []engine.Value{engine.NewList(leaves)}, nil
}
