package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// Register installs all built-in native functions into the given registry.
// It also sets ModuleInitFunc so that module sub-registries automatically
// get the same native words.
func Register(r *engine.Registry) {
	for _, fn := range All() {
		r.RegisterNativeFunc(fn)
	}
	r.ModuleInitFunc = func(child *engine.Registry) {
		for _, fn := range All() {
			child.RegisterNativeFunc(fn)
		}
	}
}

// All returns all built-in native functions.
func All() []engine.NativeFunc {
	return []engine.NativeFunc{
		listFunc(),
		createFunc(),
		loadFunc(),
		updateFunc(),
		removeFunc(),
		transformFunc(),
		mergeFunc(),
		validateFunc(),
		getpathFunc(),
		setpathFunc(),
		injectFunc(),
		cloneFunc(),
		walkFunc(),
		selectorFunc(),
		sizeFunc(),
		sliceFunc(),
		padFunc(),
		itemsFunc(),
		fetchFunc(),
		prepareFunc(),
		directFunc(),
		flattenFunc(),
		filterFunc(),
		joinFunc(),
		jsonifyFunc(),
		pushFunc(),
		popFunc(),
		unshiftFunc(),
		shiftFunc(),
		istypeFunc(),
	}
}
