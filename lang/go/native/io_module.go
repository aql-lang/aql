package native

import "github.com/aql-lang/aql/eng/go"

// IOModuleNativeFuncs builds the input/output words that were moved out
// of the core registry into the loadable `aql:io` module (namespace
// `IO`). They are registered ONLY into that module's sub-registry by
// modules.BuildIOModule — deliberately absent from the global registry.
// Most handlers live in their feature files (native_print.go's
// eng.PrintstrHandler, native_trace.go's eng.TraceHandler,
// native_misc.go's read/write in this package).
//
// `streamKind` is the per-import StreamKind type (minted by
// MintStreamKind into the module's sub-registry). It tags the
// stdin/stdout/stderr handles and types the read/write Stream
// signatures, so a stream target is type-distinct from a file path —
// without StreamKind being a global builtin. The stdin/stdout/stderr
// handlers are closures over it so each module instance stamps its own
// StreamKind onto the handles it returns.
//
// `print` is NOT here: it stays a core word so basic output needs no import.
//
// At dispatch the module FnDef wrapper short-circuits to the inner native's
// handler, which runs against the LIVE engine registry (execMatch passes
// e.registry, not the sub-registry) — so IO.read / IO.write reach the host
// FileOps and IO.stdout / IO.trace / IO.printstr reach the host Output writer
// exactly as the former core words did.
//
//	printstr  write a value's formatted form without a trailing newline
//	read      read a file or stream (path/string/StreamKind; optional options map)
//	write     write a file or stream (path/string/StreamKind; value; optional options map)
//	stdin     the standard-input stream handle (a StreamKind atom)
//	stdout    the standard-output stream handle (a StreamKind atom)
//	stderr    the standard-error stream handle (a StreamKind atom)
//	trace     run a list as a sub-program with step-by-step tracing
//	folder    create / list a filesystem folder (Path; optional options)
func IOModuleNativeFuncs(streamKind *Type) []NativeFunc {
	streamHandle := func(name string) NativeFunc {
		return NativeFunc{
			Name: name,
			Signatures: []NativeSig{{
				Args: []*Type{},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{newStreamAtom(name, streamKind)}, nil
				},
				Returns: []*Type{streamKind}, BarrierPos: -1,
			}},
		}
	}
	return []NativeFunc{
		{
			Name: "printstr",
			Signatures: []NativeSig{{
				Args:    []*Type{TAny},
				Handler: eng.PrintstrHandler,
				Returns: []*Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "read",
			Signatures: []NativeSig{
				{Args: []*Type{TPath, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TPath}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{streamKind, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{streamKind}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TPath}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TString}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, streamKind}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			// write returns the target it wrote to, tagged with the target's
			// type (a file path, or the Stream handle for a standard stream),
			// so the result can be threaded straight into read.
			Name: "write",
			Signatures: []NativeSig{
				{Args: []*Type{TPath, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{TPath}, BarrierPos: -1},
				{Args: []*Type{TPath, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{TPath}, BarrierPos: -1},
				{Args: []*Type{TPath, TString}, Handler: writeHandler, Returns: []*Type{TPath}, BarrierPos: -1},
				{Args: []*Type{TPath, TAny}, Handler: writeAnyHandler, Returns: []*Type{TPath}, BarrierPos: -1},
				{Args: []*Type{TString, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Handler: writeHandler, Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TAny}, Handler: writeAnyHandler, Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{streamKind, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TString}, Handler: writeHandler, Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny}, Handler: writeAnyHandler, Returns: []*Type{streamKind}, BarrierPos: -1},
			},
		},
		streamHandle("stdin"),
		streamHandle("stdout"),
		streamHandle("stderr"),
		{
			Name: "trace",
			Signatures: []NativeSig{{
				Args:    []*Type{TList},
				Handler: eng.TraceHandler,
				Returns: []*Type{TAny}, BarrierPos: -1,
			}},
		},
		{
			Name: "folder",
			Signatures: []NativeSig{
				{Args: []*Type{TOptions, TPath}, Handler: folderOptsHandler, Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TPath}, Handler: folderHandler, Returns: []*Type{TList}, BarrierPos: -1},
			},
		},
	}
}
