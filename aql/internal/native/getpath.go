package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// getpathFunc returns the "getpath" native function definition.
// getpath has forward precedence and one signature:
//   - [any, string] — retrieves a value at a dot-separated path from the data
func getpathFunc() NativeFunc {
	return NativeFunc{
		Name:             "getpath",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TString},
				Handler: getpathHandler,
			},
		},
	}
}

// getpathHandler calls voxgigstruct.GetPath to retrieve a nested value.
func getpathHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	path, err := args[1].AsString()
	if err != nil {
		return nil, err
	}

	result := voxgigstruct.GetPath(path, data)

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("getpath: %w", err)
	}
	return []engine.Value{val}, nil
}
