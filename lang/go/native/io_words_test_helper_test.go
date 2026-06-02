package native

// registerIOWords installs the aql:io module words (read/write/stdin/stdout/
// stderr/printstr/trace) under their bare names into a test registry. They
// moved out of core (see io_module.go); this helper lets the native-package
// behaviour tests keep exercising the unchanged handlers without an explicit
// import. Production code must `import "aql:io"` and use IO.read etc.
//
// Idempotent: a no-op if the words are already present, so it is safe to call
// from the shared runners (runAQL, …) on a registry that some test already
// seeded.
func registerIOWords(r *Registry) {
	if r == nil || r.Lookup("read") != nil {
		return
	}
	for _, n := range IOModuleNatives {
		r.RegisterNativeFunc(n)
	}
}
