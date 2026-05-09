package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// The "istype" word is registered via the consolidated Natives slice in
// natives.go. It reports whether the input is a type literal/Options/Node
// containing a type leaf.
func istypeHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewBoolean(engine.IsTypeValue(args[0]))}, nil
}
