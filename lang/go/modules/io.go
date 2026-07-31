package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildIOModule creates the "boru:io" native module. It registers the I/O
// words (formerly core) into an isolated sub-registry and returns a
// ModuleDesc with an "IO" export containing FnDef wrappers for each word.
//
// After import, the words are accessed via dot notation:
//
//	import "boru:io"
//	"data.csv" IO.read                 # read a file
//	"out.txt" "hello" IO.write          # write a file
//	IO.stdin                           # read all of stdin
//	[1 add 2 mul 3] IO.trace            # traced sub-program
//
// `print` is NOT in this module — it stays a core word so basic output needs
// no import. Everything else from the I/O family moved here (see
// native/io_module.go). The wrappers dispatch to the inner native's handler
// via the trivial-delegation short-circuit, which runs against the LIVE
// engine registry — so IO.read/IO.write reach the host FileOps and
// IO.stdout/IO.trace/IO.printstr reach the host Output writer, exactly as the
// former core words did.
func BuildIOModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// StreamKind escapes to the importer (the exported type literal, and
	// the Parent tag on the stdin/stdout/stderr handles), so it must draw
	// its ID from the importing tree's counter — see eng TypeTable.mintID.
	subReg.Types.AdoptSeqFrom(parent.Types)
	subReg.Types.MintOwner = "boru:io"
	// StreamKind — the type of the stdin/stdout/stderr handles — is owned
	// by this module: minted per import into the sub-registry, never a
	// global builtin. It tags the handles and types the read/write Stream
	// signatures, and is exported as a type literal so `x is IO.StreamKind`
	// works after import.
	streamKind := native.MintStreamKind(subReg)
	// FileType — the kind field of a stat record (file/dir/symlink/other) —
	// is owned by this module too: minted per import into the sub-registry
	// and exported as a type literal so `x is IO.FileType` works.
	fileType := native.MintFileType(subReg)
	// Watcher — the live filesystem-subscription resource IO.watch
	// returns — is minted per import too, and exported as a type literal
	// so `w is IO.Watcher` works.
	watcherType := native.MintWatcherType(subReg)
	// File — a stateful open-file handle IO.open returns — is minted per
	// import too, and exported so `f is IO.File` works. Distinct from
	// FileType (the stat-record atom enum).
	fileHandleType := native.MintFileHandleType(subReg)
	// Lock and Mmap — advisory-lock and memory-map resources IO.lock /
	// IO.mmap return — are minted per import too.
	lockType := native.MintLockType(subReg)
	mmapType := native.MintMmapType(subReg)
	ioNatives := native.IOModuleNativeFuncs(native.IOModuleTypes{
		StreamKind: streamKind,
		FileType:   fileType,
		Watcher:    watcherType,
		File:       fileHandleType,
		Lock:       lockType,
		Mmap:       mmapType,
	})
	for _, n := range ioNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := delegatingExports(ioNatives, subReg)
	// The script-argument word is exported as IO.args, but its inner
	// native is "script-args": the core fn-arguments word `args` lives in
	// the sub-registry too, and sharing its name would merge signature
	// sets (see native/io_module.go).
	if w, ok := exports.Get("script-args"); ok {
		exports.Set("args", w)
		exports.Delete("script-args")
	}
	exports.Set("StreamKind", native.NewTypeLiteral(streamKind))
	exports.Set("FileType", native.NewTypeLiteral(fileType))
	exports.Set("Watcher", native.NewTypeLiteral(watcherType))
	exports.Set("File", native.NewTypeLiteral(fileHandleType))
	exports.Set("Lock", native.NewTypeLiteral(lockType))
	exports.Set("Mmap", native.NewTypeLiteral(mmapType))

	// list / remove are exported as WORD EXTENSIONS, not namespaced FnDef
	// wrappers: import transplants a Pathon overload onto the importer's bare
	// `list` / `remove` (design/OPEN-WORDS.0.md). transplantWordExtensions
	// picks these out of the export map by IsWordExtension.
	for _, ext := range native.IOWordExtensions(fileType) {
		exports.Set(ext.Extends, native.NewFunction(ext))
	}

	return moduleDesc(parent, "IO", subReg, exports), nil
}
