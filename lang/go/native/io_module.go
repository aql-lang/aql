package native

import "github.com/aql-lang/aql/eng/go"

// IOModuleNatives holds the input/output words that were moved out of the
// core registry into the loadable `aql:io` module (namespace `IO`). They are
// registered ONLY into that module's sub-registry by modules.BuildIOModule —
// deliberately absent from the global registry. The handlers themselves still
// live in their feature files (native_print.go's eng.PrintstrHandler,
// native_trace.go's eng.TraceHandler, native_misc.go's read/write/std* in
// this package).
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
//	read      read a file or stream (path/string/Stream; optional options map)
//	write     write a file or stream (path/string/Stream; value; optional options map)
//	stdin     the standard-input stream handle (a Stream atom)
//	stdout    the standard-output stream handle (a Stream atom)
//	stderr    the standard-error stream handle (a Stream atom)
//	trace     run a list as a sub-program with step-by-step tracing
//	folder    create / list a filesystem folder (Path; optional options)
//
// The stream handles are Atoms of the Stream type (lang/go/native/io_stream.go),
// a closed enumeration of exactly {stdin, stdout, stderr}; read/write carry
// dedicated Stream signatures so a stream target is distinct from a file path.
var IOModuleNatives = []NativeFunc{
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
			{Args: []*Type{TStream, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TStream}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TPath}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TString}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TStream}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
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
			{Args: []*Type{TStream, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{TStream}, BarrierPos: -1},
			{Args: []*Type{TStream, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{TStream}, BarrierPos: -1},
			{Args: []*Type{TStream, TString}, Handler: writeHandler, Returns: []*Type{TStream}, BarrierPos: -1},
			{Args: []*Type{TStream, TAny}, Handler: writeAnyHandler, Returns: []*Type{TStream}, BarrierPos: -1},
		},
	},
	{
		Name: "stdin",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: stdinHandler,
			Returns: []*Type{TStream}, BarrierPos: -1,
		}},
	},
	{
		Name: "stdout",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: stdoutHandler,
			Returns: []*Type{TStream}, BarrierPos: -1,
		}},
	},
	{
		Name: "stderr",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: stderrHandler,
			Returns: []*Type{TStream}, BarrierPos: -1,
		}},
	},
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
