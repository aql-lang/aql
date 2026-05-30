package native

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native/help"
)

// TTimeout / TInterval are owned by the lang/go/engine package — the
// timeout/interval handlers live in this file. Registered via
// eng.Builtin.RegisterExternalBuiltin in the var initialisers so
// any package-level vars (signature slices) that reference them
// see a non-nil pointer at slice-init time. FixedIDs 4000-4001
// come from the documented lang/go/engine range (4000-4999).
var (
	TTimeout  = registerTimerType("Ideal/Timeout", 4000, timeoutFormatBehavior{})
	TInterval = registerTimerType("Ideal/Interval", 4001, intervalFormatBehavior{})
)

func registerTimerType(path string, fixedID int, behavior eng.TypeBehavior) *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, fixedID, behavior)
	if err != nil {
		// lint:allow-panic — init-time builtin registration with
		// hardcoded path and FixedID; failure indicates a build-time
		// programmer error (collision or malformed path), not a
		// runtime condition. See CLAUDE.md "Panic Prevention".
		panic(fmt.Sprintf("native_misc: register %s: %v", path, err))
	}
	return t
}

// NewTimeout constructs a Timeout value carrying the given
// TimeoutInfo payload. Moved out of eng at Step 8 — the kernel
// no longer carries a constructor for a type it doesn't own.
func NewTimeout(info *TimeoutInfo) Value {
	return eng.NewValueRaw(TTimeout, info)
}

// NewInterval constructs an Interval value carrying the given
// IntervalInfo payload. See NewTimeout.
func NewInterval(info *IntervalInfo) Value {
	return eng.NewValueRaw(TInterval, info)
}

// timeoutFormatBehavior renders a Timeout as "Timeout(id,Nms)".
// Moved from eng/coretype_format_behaviors.go at Step 8.
type timeoutFormatBehavior struct{}

func (timeoutFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (timeoutFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (timeoutFormatBehavior) Format(v Value) string {
	if ti, ok := v.Data.(*TimeoutInfo); ok {
		return fmt.Sprintf("Timeout(%s,%dms)", ti.ID, ti.Ms)
	}
	return "Timeout(nil)"
}

// intervalFormatBehavior renders an Interval as "Interval(id,Nms)".
type intervalFormatBehavior struct{}

func (intervalFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (intervalFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (intervalFormatBehavior) Format(v Value) string {
	if ii, ok := v.Data.(*IntervalInfo); ok {
		return fmt.Sprintf("Interval(%s,%dms)", ii.ID, ii.Ms)
	}
	return "Interval(nil)"
}

// miscNatives covers the smaller engine word groupings: file I/O
// (read, write, stdin, stdout, stderr), help, module/import,
// temporal (timeout, await).
//
// The supporting helpers (formatFromExt, parseFileOpts, valueToJsonic,
// doRead, doWrite, RunModuleBody, install*Exports, runParallelBranch,
// awaitAll/Full/First/Any, makeDynamicEval, etc.) live in their
// original feature files (fileio.go, native_help.go,
// native_module_module.go, native_temporal_await.go,
// native_temporal_timeout.go).
//
// Initialised in init() rather than as a direct var literal because
// the module/import handlers transitively call DefaultRegistry ->
// Register -> miscNatives, and Go's package-init cycle detector
// flags that as a forbidden cycle when the slice literal is at file
// scope. init-time assignment defers the function-value capture
// past the cycle check.
var miscNatives []NativeFunc

func init() {
	miscNatives = []NativeFunc{
		// ---- file I/O ----
		{
			Name: "read",

			Signatures: []NativeSig{
				// Path signatures
				{Args: []*Type{TPath, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TPath}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos:
				// String signatures (backward compatible)
				-1},

				{Args: []*Type{TString, TMap}, Handler: readOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TString}, Handler: readHandler, Returns: []*Type{TAny}, BarrierPos:
				// Reversed signatures for stack-first: "path" {opts} read
				-1},

				{Args: []*Type{TMap, TPath}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TString}, Handler: readOptsRevHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			Name: "write",

			Signatures: []NativeSig{
				// Path signatures
				{Args: []*Type{TPath, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TPath, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TPath, TString}, Handler: writeHandler, Returns: []*Type{}, BarrierPos:
				// String signatures (backward compatible)
				-1},

				{Args: []*Type{TString, TString, TMap}, Handler: writeOptsHandler, Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TString, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TString, TString}, Handler: writeHandler, Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			Name: "stdin",

			Signatures: []NativeSig{{
				Args:    []*Type{},
				Handler: stdinHandler,
				Returns: []*Type{TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "stdout",

			Signatures: []NativeSig{{
				Args:    []*Type{},
				Handler: stdoutHandler,
				Returns: []*Type{TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "stderr",

			Signatures: []NativeSig{{
				Args:    []*Type{},
				Handler: stderrHandler,
				Returns: []*Type{TString}, BarrierPos: -1,
			}},
		},

		// ---- help (language overview) ----
		{
			Name: "help",

			Signatures: []NativeSig{
				{Args: []*Type{}, Handler: helpOverviewHandler, BarrierPos: -1},
			},
		},

		// ---- describe (per-word documentation) ----
		{
			Name: "describe",

			Signatures: []NativeSig{
				{Args: []*Type{TString}, Handler: describeWordHandler, BarrierPos: -1},
				{Args: []*Type{TAtom}, Handler: describeWordHandler, BarrierPos: -1},
				{
					Args:      []*Type{TAtom},
					QuoteArgs: map[int]bool{0: true},
					Handler:   describeWordHandler,
					Returns:   []*Type{}, BarrierPos: -1,
				},
				{Args: []*Type{}, Handler: describeSelfHandler, BarrierPos: -1},
			},
		},

		// ---- module / import ----
		{
			Name: "module",

			Signatures: []NativeSig{{
				Args:           []*Type{TList},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        moduleHandler,
				Returns:        []*Type{TModule},
				RunInCheckMode: true, BarrierPos: -1,
			}},
		},
		{
			Name: "import",

			Signatures: []NativeSig{
				{
					Args:           []*Type{TModule},
					Handler:        importAllHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					// The leading list holds export *names* to rename
					// (`import [Orig Renamed] mod`) — import name syntax,
					// not evaluable expressions. NoEvalArgs keeps them raw
					// (bare words never degrade to data, so without this
					// the unbound names would raise undefined_word).
					Args:           []*Type{TList, TModule},
					NoEvalArgs:     map[int]bool{0: true},
					Handler:        importRenameHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					Args:           []*Type{TAtom, TModule},
					Handler:        importSingleRenameHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					Args:           []*Type{TString},
					Handler:        importFileHandler,
					Returns:        []*Type{TModule},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					Args:           []*Type{TList, TString},
					NoEvalArgs:     map[int]bool{0: true},
					Handler:        importFileRenameHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				// Inline module forms: use /q to capture "module" as a quoted word
				// instead of executing it as a function.
				{
					Args:           []*Type{TAtom, TList},
					QuoteArgs:      map[int]bool{0: true},
					NoEvalArgs:     map[int]bool{1: true},
					Handler:        importInlineHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					Args:           []*Type{TList, TAtom, TList},
					QuoteArgs:      map[int]bool{1: true},
					NoEvalArgs:     map[int]bool{0: true, 2: true},
					Handler:        importInlineRenameHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
				{
					Args:           []*Type{TAtom, TAtom, TList},
					QuoteArgs:      map[int]bool{1: true},
					NoEvalArgs:     map[int]bool{2: true},
					Handler:        importInlineSingleRenameHandler,
					Returns:        []*Type{},
					RunInCheckMode: true, BarrierPos: -1,
				},
			},
		},

		// ---- temporal ----
		{
			Name: "timeout",

			Signatures: []NativeSig{
				{
					Args:      []*Type{TInteger, TList},
					QuoteArgs: map[int]bool{1: true},
					Handler:   timeoutListHandler,
					Returns:   []*Type{TTimeout}, BarrierPos: -1,
				},
				{
					Args:      []*Type{TInteger, TAtom},
					QuoteArgs: map[int]bool{1: true},
					Handler:   timeoutWordHandler,
					Returns:   []*Type{TTimeout}, BarrierPos: -1,
				},
			},
		},
		{
			Name: "await",

			Signatures: []NativeSig{
				{
					Args:       []*Type{TOptions, TList},
					NoEvalArgs: map[int]bool{1: true},
					Handler:    awaitWithOptsHandler,
					Returns:    []*Type{TAny}, BarrierPos: -1,
				},
				{
					Args:       []*Type{TList},
					NoEvalArgs: map[int]bool{0: true},
					Handler:    awaitDefaultHandler,
					Returns:    []*Type{TAny}, BarrierPos: -1,
				},
			},
		},
	}
}

// ---- file I/O handlers ----

// extractPath returns the path string from a String or Path value.
func extractPath(v Value) string {
	if IsPath(v) {
		_as5, _ := AsPath(v)
		return _as5.String()
	}
	_as6, _ := AsString(v)
	return _as6
}

// returnPath wraps the result path: if input was a Path, return Path; else String.
func returnPath(v Value, pathStr string) Value {
	if IsPath(v) {
		return v
	}
	return NewString(pathStr)
}

func readHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	format := formatFromExt(path)
	if format == "" {
		format = "text"
	}
	return doRead(r, path, "utf8", format, "lf")
}

func readOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	enc, format, _, nl, fmtExplicit := parseFileOpts(args[1])
	if !fmtExplicit {
		if extFmt := formatFromExt(path); extFmt != "" {
			format = extFmt
		}
	}
	return doRead(r, path, enc, format, nl)
}

// Reversed handler for stack-first usage: "path" {opts} read
// In nearest-first stack matching, opts (top) maps to sig[0], path to sig[1].
func readOptsRevHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return readOptsHandler([]Value{args[1], args[0]}, ctx, stack, r)
}

func writeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	result, err := doWrite(r, path, content, "utf8", "text", "write", "lf")
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0], path)}, nil
}

func writeOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	enc, format, mode, nl, _ := parseFileOpts(args[2])
	result, err := doWrite(r, path, content, enc, format, mode, nl)
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0], path)}, nil
}

// write: [path/string, any, map] -> [path/string] (for non-string data with fmt)
func writeAnyOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	_, format, mode, nl, _ := parseFileOpts(args[2])
	if format == "text" {
		format = "jsonic"
	}
	content := valueToJsonic(args[1])
	result, err := doWrite(r, path, content, "utf8", format, mode, nl)
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0], path)}, nil
}

func stdinHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(pathStdin)}, nil
}

func stdoutHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(pathStdout)}, nil
}

func stderrHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(pathStderr)}, nil
}

// ---- help / describe handlers ----

// helpOverviewHandler implements the 0-arg `help` word: a language
// overview plus a pointer at `describe` for per-word docs.
func helpOverviewHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fmt.Fprint(r.Output, help.Overview())
	return nil, nil
}

// describeSelfHandler implements the 0-arg `describe` word: a reminder
// of how to call describe on a specific word.
func describeSelfHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fmt.Fprintln(r.Output, "describe — Describe an AQL word: signatures, examples, and notes.")
	fmt.Fprintln(r.Output, "")
	fmt.Fprintln(r.Output, "Usage:")
	fmt.Fprintln(r.Output, "  describe <word>     Describe a word (e.g. describe add).")
	fmt.Fprintln(r.Output, "  \"<name>\" describe   Describe by string name (e.g. \"concat\" describe).")
	fmt.Fprintln(r.Output, "")
	fmt.Fprintln(r.Output, "Run `help` for a language overview.")
	return nil, nil
}

func describeWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := ValToString(args[0])
	// Prefer live registry data (signatures + examples). Fall back to
	// the static entry for words that are documented but not registered
	// in this build, then report nothing if neither exists.
	if info := BuildFuncInfo(r, name); info != nil {
		fmt.Fprint(r.Output, help.FormatDynamic(*info))
		return nil, nil
	}
	if entry := help.Lookup(name); entry != nil {
		fmt.Fprint(r.Output, help.Format(entry))
		return nil, nil
	}
	fmt.Fprintf(r.Output, "describe: no description available for %q\n", name)
	return nil, nil
}

// ---- module / import handlers ----

func moduleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("module_error", "module: argument must be a concrete list, got type literal", "module")
	}
	_lst, _ := AsList(args[0])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("module: %w", err)
	}
	return []Value{NewModule(desc)}, nil
}

func importAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := AsModule(args[0])
	installExports(r, desc, nil)
	return nil, nil
}

func importRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := AsModule(args[1])
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("import_error", "import: rename list must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

func importSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := AsModule(args[1])
	newName, _ := args[0].AsConcreteAtom()
	return nil, installSingleRename(r, desc, newName)
}

func importFileHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[0].AsConcreteString()
	if isNativeModImport(path) {
		return nil, resolveNativeMod(r, path)
	}
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil {
			return nil, err
		}
		installExports(r, desc, nil)
		return nil, nil
	}
	if isDataFile(path) {
		return loadDataFile(r, path)
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	installExports(r, desc, nil)
	return nil, nil
}

func importFileRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[1].AsConcreteString()
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil {
			return nil, err
		}
		_lst, _ := AsList(args[0])
		return nil, installRenamedExports(r, desc, _lst.Slice())
	}
	if isDataFile(path) {
		return nil, r.AqlError("import_error", fmt.Sprintf("import: rename not supported for data files (%s)", path), "import")
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

// import: [atom/q list] -> [] — inline module: import module [body]
// The /q captures "module" as a quoted word; the handler runs the body
// to produce a module descriptor, then imports all exports.
func importInlineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if name != "module" {
		return nil, r.AqlError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", name), "import")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[1])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	installExports(r, desc, nil)
	return nil, nil
}

func importInlineRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[1])
	if name != "module" {
		return nil, r.AqlError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", name), "import")
	}
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("import_error", "import: rename list must be a concrete list, got type literal", "import")
	}
	if !IsConcrete(args[2]) {
		return nil, r.AqlError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst2, _ := AsList(args[2])
	desc, err := RunModuleBody(r, _lst2.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

func importInlineSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	modName := defName(args[1])
	if modName != "module" {
		return nil, r.AqlError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", modName), "import")
	}
	if !IsConcrete(args[2]) {
		return nil, r.AqlError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[2])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	return nil, installSingleRename(r, desc, defName(args[0]))
}

// ---- temporal handlers ----

func timeoutListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTimeout(r, args, true)
}

func timeoutWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTimeout(r, args, false)
}

func doTimeout(r *Registry, args []Value, isList bool) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, r.AqlError("timeout_error", fmt.Sprintf("timeout: milliseconds must be non-negative, got %d", ms), "timeout")
	}
	callback := args[1]

	id := GenerateID("T_")
	timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		RunTimerCallback(r, callback, isList)
	})

	info := &TimeoutInfo{
		ID:    id,
		Ms:    ms,
		Timer: timer,
	}
	return []Value{NewTimeout(info)}, nil
}

func awaitWithOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// args[0] = Options, args[1] = List (parallels)
	mode := "all"
	if oi, err := AsOptionsType(args[0]); err == nil {
		if v, ok := oi.Fields.Get("mode"); ok {
			if s, err := AsString(v); err == nil {
				mode = s
			} else if a, err := AsAtom(v); err == nil {
				mode = a
			}
		}
	} else if optsMap, _ := AsMap(args[0]); optsMap != nil {
		if v, ok := optsMap.Get("mode"); ok {
			if s, err := AsString(v); err == nil {
				mode = s
			} else if a, err := AsAtom(v); err == nil {
				mode = a
			}
		}
	}
	return doAwait(r, mode, args[1])
}

func awaitDefaultHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doAwait(r, "all", args[0])
}

func doAwait(r *Registry, mode string, parallels Value) ([]Value, error) {
	if !IsConcrete(parallels) {
		return nil, r.AqlError("await_error", "await: parallels must be a concrete list, got type literal", "await")
	}
	_lst, _ := AsList(parallels)
	elems := _lst.Slice()
	if len(elems) == 0 {
		return []Value{NewList([]Value{})}, nil
	}

	switch mode {
	case "all":
		return awaitAll(r, elems)
	case "full":
		return awaitFull(r, elems)
	case "first":
		return awaitFirst(r, elems)
	case "any":
		return awaitAny(r, elems)
	default:
		return nil, r.AqlError("await_error", fmt.Sprintf("await: unknown mode %q, expected all, full, first, or any", mode), "await")
	}
}
