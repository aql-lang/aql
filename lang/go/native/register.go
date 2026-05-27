package native

// Register installs every built-in word on the given registry. Called
// from DefaultRegistry. Word definitions live in the various
// native_*.go and feature files alongside their handlers.
//
// Lang owns every word name. The eng kernel exposes algorithm
// primitives (CoerceBoolean, TandValues, TorHandler, ...) that the
// registrations below wire into the dispatch; eng does not register
// any word of its own.
//
// Post-consolidation (engine → native), this is the single entry
// point. The Natives slice (in natives.go) covers the
// data-manipulation words formerly registered by native.Register;
// the per-category slices below cover the language-layer primitives
// formerly registered by engine.Register.
func Register(r *Registry) {
	for _, n := range makeNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range inspectNatives {
		r.RegisterNativeFunc(n)
	}
	// break / continue are owned by lang (see native_control.go); the
	// kernel only provides the FlowCtrl type and the Run-loop dispatch.

	// String
	for _, n := range stringNatives {
		r.RegisterNativeFunc(n)
	}

	// Stack ops
	for _, n := range stackNatives {
		r.RegisterNativeFunc(n)
	}

	// Math: basic arithmetic.
	for _, n := range mathNatives {
		r.RegisterNativeFunc(n)
	}

	// Boolean.
	for _, n := range booleanNatives {
		r.RegisterNativeFunc(n)
	}

	// Comparison
	for _, n := range comparisonNatives {
		r.RegisterNativeFunc(n)
	}

	// Storage
	for _, n := range storageNatives {
		r.RegisterNativeFunc(n)
	}

	// Definition
	for _, n := range definitionNatives {
		r.RegisterNativeFunc(n)
	}

	// Ref / apply — first-class function value pipeline.
	for _, n := range refNatives {
		r.RegisterNativeFunc(n)
	}

	// *Type
	for _, n := range typeNatives {
		r.RegisterNativeFunc(n)
	}
	r.RegisterNativeFunc(behaveNative)
	r.RegisterNativeFunc(nodifyNative)
	r.RegisterNativeFunc(sortNative)
	installResourceTypes(r)
	installIdeals(r)

	// Control flow
	for _, n := range controlNatives {
		r.RegisterNativeFunc(n)
	}

	// Accessors
	for _, n := range accessorNatives {
		r.RegisterNativeFunc(n)
	}

	// I/O, help, module, temporal (consolidated)
	for _, n := range miscNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range printNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range traceNatives {
		r.RegisterNativeFunc(n)
	}

	// Unify
	for _, n := range unifyNatives {
		r.RegisterNativeFunc(n)
	}

	// Array (core + higher-order)
	for _, n := range arrayNatives {
		r.RegisterNativeFunc(n)
	}

	// Data-manipulation words (former native.Register body).
	for _, n := range Natives {
		r.RegisterNativeFunc(n)
	}
	r.Modules.InitFunc = func(child *Registry) {
		for _, n := range Natives {
			child.RegisterNativeFunc(n)
		}
	}
}
