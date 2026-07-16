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
func IOModuleNativeFuncs(streamKind, fileType *Type) []NativeFunc {
	streamHandle := func(name string) NativeFunc {
		return NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Args: []*Type{},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{newStreamAtom(name, streamKind)}, nil
				}),
				Returns: []*Type{streamKind}, BarrierPos: -1,
			}},
		}
	}
	// statImpl / listImpl close over fileType so the filesystem-structure
	// handlers can tag stat records with the module's FileType atoms.
	statImpl := func(hasOpts bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			return statHandler(a, r, fileType, hasOpts)
		}
	}
	listImpl := func(hasOpts bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			return listHandler(a, r, fileType, hasOpts)
		}
	}
	return []NativeFunc{
		{
			Name: "printstr",
			Signatures: []Signature{{
				Args:    []*Type{TAny},
				Impl:    Go(eng.PrintstrHandler),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "read",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(readOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(readHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Impl: Go(readOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(readHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{streamKind, TMap}, Impl: Go(readOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{streamKind}, Impl: Go(readHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TPathon}, Impl: Go(readOptsRevHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TString}, Impl: Go(readOptsRevHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, streamKind}, Impl: Go(readOptsRevHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			// write returns the target it wrote to, tagged with the target's
			// type (a file path, or the Stream handle for a standard stream),
			// so the result can be threaded straight into read.
			Name: "write",
			Signatures: []Signature{
				// Binary writes: a Bytes payload is written verbatim (more
				// specific than the TAny/TString sigs, so it wins dispatch).
				{Args: []*Type{TPathon, TBytes, TMap}, Impl: Go(writeBytesOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TBytes}, Impl: Go(writeBytesHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TBytes, TMap}, Impl: Go(writeBytesOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TBytes}, Impl: Go(writeBytesHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{streamKind, TBytes, TMap}, Impl: Go(writeBytesOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TBytes}, Impl: Go(writeBytesHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString, TMap}, Impl: Go(writeOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TAny, TMap}, Impl: Go(writeAnyOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString}, Impl: Go(writeHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TAny}, Impl: Go(writeAnyHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TString, TMap}, Impl: Go(writeOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TAny, TMap}, Impl: Go(writeAnyOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Impl: Go(writeHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TAny}, Impl: Go(writeAnyHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{streamKind, TString, TMap}, Impl: Go(writeOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny, TMap}, Impl: Go(writeAnyOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TString}, Impl: Go(writeHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny}, Impl: Go(writeAnyHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
			},
		},
		streamHandle("stdin"),
		streamHandle("stdout"),
		streamHandle("stderr"),
		{
			Name: "trace",
			Signatures: []Signature{{
				Args:    []*Type{TList},
				Impl:    Go(eng.TraceHandler),
				Returns: []*Type{TAny}, BarrierPos: -1,
			}},
		},
		{
			Name: "folder",
			Signatures: []Signature{
				{Args: []*Type{TOptions, TPathon}, Impl: Go(folderOptsHandler), Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(folderHandler), Returns: []*Type{TList}, BarrierPos: -1},
			},
		},
		{
			// stat returns a FileInfo record (or none when absent). The
			// target may be a Path or a string; an optional map carries
			// {follow, resolve}.
			Name: "stat",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(statImpl(true)), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(statImpl(false)), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Impl: Go(statImpl(true)), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(statImpl(false)), Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			// list enumerates a directory's entries (names, or FileInfo
			// records with {detail:true}); {recursive:true} walks the tree.
			Name: "list",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(listImpl(true)), Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(listImpl(false)), Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Impl: Go(listImpl(true)), Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(listImpl(false)), Returns: []*Type{TList}, BarrierPos: -1},
			},
		},
		{
			// remove deletes a path, returning it. {recursive:true} removes a
			// directory tree; {force:true} ignores an absent path.
			Name: "remove",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(ioRemoveOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(ioRemoveHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Impl: Go(ioRemoveOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(ioRemoveHandler), Returns: []*Type{TString}, BarrierPos: -1},
			},
		},
		{
			// move renames/moves src to dst, returning dst.
			Name: "move",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon}, Impl: Go(moveHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TPathon}, Impl: Go(moveHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString}, Impl: Go(moveHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Impl: Go(moveHandler), Returns: []*Type{TString}, BarrierPos: -1},
			},
		},
		{
			// copy copies src to dst, returning dst. {recursive:true} copies a
			// directory tree.
			Name: "copy",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon, TMap}, Impl: Go(copyOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TPathon, TMap}, Impl: Go(copyOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString, TMap}, Impl: Go(copyOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString, TMap}, Impl: Go(copyOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TPathon, TPathon}, Impl: Go(copyHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TPathon}, Impl: Go(copyHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString}, Impl: Go(copyHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Impl: Go(copyHandler), Returns: []*Type{TString}, BarrierPos: -1},
			},
		},
		{
			// link creates a link at dst referring to src — a symbolic link by
			// default, a hard link with {hard:true}. Returns dst.
			Name: "link",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon, TMap}, Impl: Go(linkOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TPathon, TMap}, Impl: Go(linkOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString, TMap}, Impl: Go(linkOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString, TMap}, Impl: Go(linkOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TPathon, TPathon}, Impl: Go(linkHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TPathon}, Impl: Go(linkHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString}, Impl: Go(linkHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Impl: Go(linkHandler), Returns: []*Type{TString}, BarrierPos: -1},
			},
		},
		{
			// touch creates a path if absent and applies metadata options
			// {mode, mtime, atime, size} (folding chmod/utimes/truncate).
			// Returns the path.
			Name: "touch",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(touchOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(touchHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TString, TMap}, Impl: Go(touchOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(touchHandler), Returns: []*Type{TString}, BarrierPos: -1},
			},
		},
	}
}
