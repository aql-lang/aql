package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildIOModule creates the "aql:io" native module. It registers the I/O
// words (formerly core) into an isolated sub-registry and returns a
// ModuleDesc with an "IO" export containing FnDef wrappers for each word.
//
// After import, the words are accessed via dot notation:
//
//	import "aql:io"
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
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// StreamKind escapes to the importer (the exported type literal, and
	// the Parent tag on the stdin/stdout/stderr handles), so it must draw
	// its ID from the importing tree's counter — see eng TypeTable.mintID.
	subReg.Types.AdoptSeqFrom(parent.Types)
	// StreamKind — the type of the stdin/stdout/stderr handles — is owned
	// by this module: minted per import into the sub-registry, never a
	// global builtin. It tags the handles and types the read/write Stream
	// signatures, and is exported as a type literal so `x is IO.StreamKind`
	// works after import.
	streamKind := native.MintStreamKind(subReg)
	ioNatives := native.IOModuleNativeFuncs(streamKind)
	for _, n := range ioNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range ioNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}
	exports.Set("StreamKind", native.NewTypeLiteral(streamKind))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"IO": exports},
	}
	return desc, nil
}
