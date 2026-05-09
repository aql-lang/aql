package engine

// Register installs the engine's built-in word set on the given
// registry. This is invoked from DefaultRegistry. Word definitions
// themselves live in the various native_*.go and feature files
// alongside their handlers.
func Register(r *Registry) {
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
	for _, n := range ComparisonNatives {
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

	// Type
	for _, n := range typeNatives {
		r.RegisterNativeFunc(n)
	}
	installResourceTypes(r)

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
	for _, n := range PrintNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range TraceNatives {
		r.RegisterNativeFunc(n)
	}

	// Unify
	for _, n := range UnifyNatives {
		r.RegisterNativeFunc(n)
	}

	// Array (core + higher-order)
	for _, n := range arrayNatives {
		r.RegisterNativeFunc(n)
	}

	// Query DSL (select/from/where/order/...) is intentionally not
	// installed here: prior to this refactor RegisterQuery was a
	// dead function (no caller in register.go). Keeping that
	// behaviour preserves the baseline test failure count and
	// avoids registering the 'where' / 'group' query overloads on
	// top of the array words of the same name. Hosts that want the
	// query DSL can install queryNatives explicitly via a provider
	// passed to DefaultRegistry.
}
