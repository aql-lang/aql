package engine

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/lang/engine/help"
)

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
			Name:              "read",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				// Path signatures
				{Args: []Type{TPath, TMap}, Handler: readOptsHandler, Returns: []Type{TAny}},
				{Args: []Type{TPath}, Handler: readHandler, Returns: []Type{TAny}},
				// String signatures (backward compatible)
				{Args: []Type{TString, TMap}, Handler: readOptsHandler, Returns: []Type{TAny}},
				{Args: []Type{TString}, Handler: readHandler, Returns: []Type{TAny}},
				// Reversed signatures for stack-first: "path" {opts} read
				{Args: []Type{TMap, TPath}, Handler: readOptsRevHandler, Returns: []Type{TAny}},
				{Args: []Type{TMap, TString}, Handler: readOptsRevHandler, Returns: []Type{TAny}},
			},
		},
		{
			Name:              "write",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				// Path signatures
				{Args: []Type{TPath, TString, TMap}, Handler: writeOptsHandler, Returns: []Type{}},
				{Args: []Type{TPath, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []Type{}},
				{Args: []Type{TPath, TString}, Handler: writeHandler, Returns: []Type{}},
				// String signatures (backward compatible)
				{Args: []Type{TString, TString, TMap}, Handler: writeOptsHandler, Returns: []Type{}},
				{Args: []Type{TString, TAny, TMap}, Handler: writeAnyOptsHandler, Returns: []Type{}},
				{Args: []Type{TString, TString}, Handler: writeHandler, Returns: []Type{}},
			},
		},
		{
			Name:              "stdin",
			ForwardPrecedence: true,
			Signatures: []NativeSig{{
				Args:    []Type{},
				Handler: stdinHandler,
				Returns: []Type{TString},
			}},
		},
		{
			Name:              "stdout",
			ForwardPrecedence: true,
			Signatures: []NativeSig{{
				Args:    []Type{},
				Handler: stdoutHandler,
				Returns: []Type{TString},
			}},
		},
		{
			Name:              "stderr",
			ForwardPrecedence: true,
			Signatures: []NativeSig{{
				Args:    []Type{},
				Handler: stderrHandler,
				Returns: []Type{TString},
			}},
		},

		// ---- help ----
		{
			Name:              "help",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				{Args: []Type{TString}, Handler: helpWordHandler},
				{Args: []Type{TAtom}, Handler: helpWordHandler},
				{
					Args:      []Type{TAtom},
					QuoteArgs: map[int]bool{0: true},
					Handler:   helpWordHandler,
					Returns:   []Type{},
				},
				{Args: []Type{}, Handler: helpSelfHandler},
			},
		},

		// ---- module / import ----
		{
			Name:              "module",
			ForwardPrecedence: true,
			Signatures: []NativeSig{{
				Args:           []Type{TList},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        moduleHandler,
				Returns:        []Type{TModule},
				RunInCheckMode: true,
			}},
		},
		{
			Name:              "import",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				{
					Args:           []Type{TModule},
					Handler:        importAllHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TList, TModule},
					Handler:        importRenameHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TAtom, TModule},
					Handler:        importSingleRenameHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TString},
					Handler:        importFileHandler,
					Returns:        []Type{TModule},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TList, TString},
					Handler:        importFileRenameHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				// Inline module forms: use /q to capture "module" as a quoted word
				// instead of executing it as a function.
				{
					Args:           []Type{TAtom, TList},
					QuoteArgs:      map[int]bool{0: true},
					NoEvalArgs:     map[int]bool{1: true},
					Handler:        importInlineHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TList, TAtom, TList},
					QuoteArgs:      map[int]bool{1: true},
					NoEvalArgs:     map[int]bool{2: true},
					Handler:        importInlineRenameHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
				{
					Args:           []Type{TAtom, TAtom, TList},
					QuoteArgs:      map[int]bool{1: true},
					NoEvalArgs:     map[int]bool{2: true},
					Handler:        importInlineSingleRenameHandler,
					Returns:        []Type{},
					RunInCheckMode: true,
				},
			},
		},

		// ---- temporal ----
		{
			Name:              "timeout",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				{
					Args:      []Type{TInteger, TList},
					QuoteArgs: map[int]bool{1: true},
					Handler:   timeoutListHandler,
					Returns:   []Type{TTimeout},
				},
				{
					Args:      []Type{TInteger, TAtom},
					QuoteArgs: map[int]bool{1: true},
					Handler:   timeoutWordHandler,
					Returns:   []Type{TTimeout},
				},
			},
		},
		{
			Name:              "await",
			ForwardPrecedence: true,
			Signatures: []NativeSig{
				{
					Args:       []Type{TOptions, TList},
					NoEvalArgs: map[int]bool{1: true},
					Handler:    awaitWithOptsHandler,
					Returns:    []Type{TAny},
				},
				{
					Args:       []Type{TList},
					NoEvalArgs: map[int]bool{0: true},
					Handler:    awaitDefaultHandler,
					Returns:    []Type{TAny},
				},
			},
		},
	}
}

// ---- file I/O handlers ----

// extractPath returns the path string from a String or Path value.
func extractPath(v Value) string {
	if v.IsPath() {
		_as5, _ := v.AsPath()
		return _as5.String()
	}
	_as6, _ := v.AsString()
	return _as6
}

// returnPath wraps the result path: if input was a Path, return Path; else String.
func returnPath(v Value, pathStr string) Value {
	if v.IsPath() {
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

// ---- help handlers ----

func helpSelfHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fmt.Fprintln(r.Output, "help — Show help for an AQL word.")
	fmt.Fprintln(r.Output, "")
	fmt.Fprintln(r.Output, "Usage:")
	fmt.Fprintln(r.Output, "  help              Show this message.")
	fmt.Fprintln(r.Output, "  <word> help       Show help for a word (e.g. add help).")
	fmt.Fprintln(r.Output, "  \"<name>\" help     Show help by string name (e.g. \"concat\" help).")
	return nil, nil
}

func helpWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := ValToString(args[0])
	info := BuildFuncInfo(r, name)
	if info == nil {
		fmt.Fprintf(r.Output, "help: no help available for %q\n", name)
		return nil, nil
	}
	fmt.Fprint(r.Output, help.FormatDynamic(*info))
	return nil, nil
}

// ---- module / import handlers ----

func moduleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("module: argument must be a concrete list, got type literal")
	}
	desc, err := RunModuleBody(r, args[0].AsList().Slice())
	if err != nil {
		return nil, fmt.Errorf("module: %w", err)
	}
	return []Value{NewModule(desc)}, nil
}

func importAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := args[0].AsModule()
	installExports(r, desc, nil)
	return nil, nil
}

func importRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := args[1].AsModule()
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("import: rename list must be a concrete list, got type literal")
	}
	return nil, installRenamedExports(r, desc, args[0].AsList().Slice())
}

func importSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := args[1].AsModule()
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
		return nil, installRenamedExports(r, desc, args[0].AsList().Slice())
	}
	if isDataFile(path) {
		return nil, fmt.Errorf("import: rename not supported for data files (%s)", path)
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	return nil, installRenamedExports(r, desc, args[0].AsList().Slice())
}

// import: [atom/q list] -> [] — inline module: import module [body]
// The /q captures "module" as a quoted word; the handler runs the body
// to produce a module descriptor, then imports all exports.
func importInlineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if name != "module" {
		return nil, fmt.Errorf("import: unknown inline form %q (expected 'module')", name)
	}
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("import: module body must be a concrete list, got type literal")
	}
	desc, err := RunModuleBody(r, args[1].AsList().Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	installExports(r, desc, nil)
	return nil, nil
}

func importInlineRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[1])
	if name != "module" {
		return nil, fmt.Errorf("import: unknown inline form %q (expected 'module')", name)
	}
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("import: rename list must be a concrete list, got type literal")
	}
	if !IsConcrete(args[2]) {
		return nil, fmt.Errorf("import: module body must be a concrete list, got type literal")
	}
	desc, err := RunModuleBody(r, args[2].AsList().Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	return nil, installRenamedExports(r, desc, args[0].AsList().Slice())
}

func importInlineSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	modName := defName(args[1])
	if modName != "module" {
		return nil, fmt.Errorf("import: unknown inline form %q (expected 'module')", modName)
	}
	if !IsConcrete(args[2]) {
		return nil, fmt.Errorf("import: module body must be a concrete list, got type literal")
	}
	desc, err := RunModuleBody(r, args[2].AsList().Slice())
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
		return nil, fmt.Errorf("timeout: milliseconds must be non-negative, got %d", ms)
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
	if oi, err := args[0].AsOptionsType(); err == nil {
		if v, ok := oi.Fields.Get("mode"); ok {
			if s, err := v.AsString(); err == nil {
				mode = s
			} else if a, err := v.AsAtom(); err == nil {
				mode = a
			}
		}
	} else if optsMap := args[0].AsMap(); optsMap != nil {
		if v, ok := optsMap.Get("mode"); ok {
			if s, err := v.AsString(); err == nil {
				mode = s
			} else if a, err := v.AsAtom(); err == nil {
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
	if parallels.Data == nil {
		return nil, fmt.Errorf("await: parallels must be a concrete list, got type literal")
	}
	elems := parallels.AsList().Slice()
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
		return nil, fmt.Errorf("await: unknown mode %q, expected all, full, first, or any", mode)
	}
}
