package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// istypeFunc returns the "istype" native function definition.
// istype checks whether a value is a type literal, an Options instance,
// or a Node (List/Map) containing a leaf that is a type.
func istypeFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "istype",
		ForwardPrecedence: true,
		SkipSafetyCheck:   true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: istypeHandler,
			},
		},
	}
}

func istypeHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewBoolean(engine.IsTypeValue(args[0]))}, nil
}
