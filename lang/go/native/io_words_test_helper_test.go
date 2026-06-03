package native

// registerIOWords installs the words that were moved OUT of core into loadable
// modules (aql:io, aql:struct, aql:net, aql:bin bitwise, aql:type-util's
// tpartial, …) under their bare names into a test registry. The handlers are
// unchanged by the move; this helper lets the native-package behaviour tests
// keep exercising them without an explicit import. Production code must
// `import "aql:<mod>"` and use the namespace (IO.read, Struct.merge, Bin.band,
// …) — proved by the module-*.tsv specs.
//
// Idempotent: a no-op if the words are already present, so it is safe to call
// from the shared runners (runAQL, …) on a registry that some test seeded.
func registerIOWords(r *Registry) {
	if r == nil || r.Lookup("read") != nil {
		return
	}
	moved := [][]NativeFunc{
		IOModuleNatives,
		StructModuleNatives,
		NetModuleNatives,
		BitwiseModuleNatives,
		TPartialModuleNatives,
		TimeAsyncModuleNatives,
	}
	for _, slice := range moved {
		for _, n := range slice {
			r.RegisterNativeFunc(n)
		}
	}
}
