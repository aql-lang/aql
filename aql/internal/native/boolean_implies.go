package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)
func RegisterImplies(r *engine.Registry) {
	// Signature [Boolean, Boolean]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b implies` → args=[b,a] → !a||b.
	handler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
		left, _ := args[1].AsBoolean()
		right, _ := args[0].AsBoolean()
		return []engine.Value{engine.NewBoolean(!left || right)}, nil
	}

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "implies",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args:    []engine.Type{engine.TBoolean, engine.TBoolean},
			Handler: handler,
			Returns: []engine.Type{engine.TBoolean},
		}},
	})
}
