package native

// registerIOWords installs the words that were moved OUT of core into loadable
// modules (boru:io, boru:struct, boru:net, boru:bin bitwise, boru:type-util's
// tpartial, …) under their bare names into a test registry. The handlers are
// unchanged by the move; this helper lets the native-package behaviour tests
// keep exercising them without an explicit import. Production code must
// `import "boru:<mod>"` and use the namespace (IO.read, StructUtil.merge, BinUtil.band,
// …) — proved by the module-*.tsv specs.
//
// Idempotent: a no-op if the words are already present, so it is safe to call
// from the shared runners (runBORU, …) on a registry that some test seeded.
func registerIOWords(r *Registry) {
	if r == nil || r.Lookup("read") != nil {
		return
	}
	fileType := MintFileType(r)
	moved := [][]NativeFunc{
		IOModuleNativeFuncs(IOModuleTypes{StreamKind: MintStreamKind(r), FileType: fileType, Watcher: MintWatcherType(r), File: MintFileHandleType(r), Lock: MintLockType(r), Mmap: MintMmapType(r)}),
		StructModuleNatives,
		NetModuleNatives(MintFetchTypes(r)),
		BitwiseModuleNatives,
		TPartialModuleNatives,
		TimeAsyncModuleNatives(MintTemporalModuleTypes(r)),
		LogicModuleNatives,
		StringModuleNatives,
	}
	for _, slice := range moved {
		for _, n := range slice {
			r.RegisterNativeFunc(n)
		}
	}
	// list / remove are exported as word extensions, not namespaced natives:
	// transplant their Pathon overloads onto the core list / remove words so
	// the bare-word behaviour tests can call `list`/`remove` on a Pathon.
	for _, ext := range IOWordExtensions(fileType) {
		if err := TransplantExtension(r, ext, "boru:io", "boru:io"); err != nil {
			panic(err)
		}
	}
}
