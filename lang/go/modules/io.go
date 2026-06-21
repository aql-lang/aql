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
//	"aql:io" import
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
	for _, n := range native.IOModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.IOModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	// StreamKind — the type of the stdin/stdout/stderr handles — is a
	// module-scoped type, not a global builtin. Export it as a type
	// literal so `x is IO.StreamKind` works after import.
	exports.Set("StreamKind", native.NewTypeLiteral(native.StreamKindType))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"IO": exports},
	}
	return desc, nil
}
