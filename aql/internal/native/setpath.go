package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// setpathFunc returns the "setpath" native function definition.
// setpath has suffix precedence and one signature:
//   - [any, string, any] — sets a value at a dot-separated path in the data
func setpathFunc() NativeFunc {
	return NativeFunc{
		Name:             "setpath",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TString, engine.TAny},
				Handler: setpathHandler,
			},
		},
	}
}

// setpathHandler calls voxgigstruct.SetPath to set a nested value.
func setpathHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	path := args[1].AsString()
	newVal := valueToAny(args[2])

	result := voxgigstruct.SetPath(data, path, newVal)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("setpath: %w", err)
	}
	return []engine.Value{val}, nil
}
