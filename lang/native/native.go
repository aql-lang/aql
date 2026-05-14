package native

import (
	"github.com/aql-lang/aql/lang/engine"
)

// Register installs every entry in the consolidated Natives slice into
// the given registry and arranges for module sub-registries (created via
// ModuleInitFunc) to receive the same words automatically. This is the
// public entry point for the native package; per-word RegisterFoo
// functions have been removed in favour of the slice-driven model.
func Register(r *engine.Registry) {
	for _, n := range Natives {
		r.RegisterNativeFunc(n)
	}
	r.Modules.InitFunc = func(child *engine.Registry) {
		for _, n := range Natives {
			child.RegisterNativeFunc(n)
		}
	}
}
