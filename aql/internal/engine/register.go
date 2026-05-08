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
	RegisterComparison(r)

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

	// I/O
	RegisterFileIO(r)
	RegisterPrint(r)
	RegisterTrace(r)

	// Unify
	RegisterUnify(r)

	// Module
	RegisterModule(r)

	// Array (core + higher-order)
	for _, n := range arrayNatives {
		r.RegisterNativeFunc(n)
	}

	// Temporal
	RegisterTimeout(r)
	RegisterAwait(r)

	// Help
	RegisterHelp(r)
}
