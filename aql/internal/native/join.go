package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// joinFunc returns the "join" native function definition.
// join has forward precedence and two signatures:
//   - [list, string] — joins the list elements with the given separator
//   - [list]         — joins the list elements with a comma
func RegisterJoin(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "join",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TString},
				Handler: joinSepHandler,
			},
			{
				Args:    []engine.Type{engine.TList},
				Handler: joinDefaultHandler,
			},
		},
	})
}

// joinDefaultHandler calls voxgigstruct.Join with default separator (comma).
func joinDefaultHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("join: expected list, got %T", data)
	}
	result := voxgigstruct.Join(arr)
	return []engine.Value{engine.NewString(result)}, nil
}

// joinSepHandler calls voxgigstruct.Join with a specified separator.
func joinSepHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("join: expected list, got %T", data)
	}
	sep, err := args[1].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("join: separator: %w", err)
	}
	result := voxgigstruct.Join(arr, sep)
	return []engine.Value{engine.NewString(result)}, nil
}
