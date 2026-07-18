package native

import (
	"fmt"
	"strings"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native/help"
)

// The Timeout / Interval timer types are owned by aql:time-util — the
// timeout/interval handlers live in this file, and the types are
// per-import module mints (former global FixedIDs 4000-4001, retired)
// minted alongside the temporal leaves by MintTemporalModuleTypes
// (native_temporal.go). The constructors are methods on
// TemporalModuleTypes so every timer handle carries its import's own
// type identity.

// NewTimeout constructs a Timeout value carrying the given
// TimeoutInfo payload.
func (tt TemporalModuleTypes) NewTimeout(info *TimeoutInfo) Value {
	return eng.NewValueRaw(tt.Timeout, info)
}

// NewInterval constructs an Interval value carrying the given
// IntervalInfo payload. See NewTimeout.
func (tt TemporalModuleTypes) NewInterval(info *IntervalInfo) Value {
	return eng.NewValueRaw(tt.Interval, info)
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
		// file I/O (read/write) and stream words (stdin/stdout/stderr)
		// moved to the aql:io module — see io_module.go.

		// ---- help (language overview) ----
		{
			Name: "help",

			Signatures: []Signature{
				// Prints the overview to r.Output; produces no value.
				{Args: []*Type{}, Impl: Go(helpOverviewHandler), Returns: []*Type{}, BarrierPos: -1},
			},
		},

		// ---- describe (per-word documentation) ----
		{
			Name: "describe",

			Signatures: []Signature{
				// Prints the documentation to r.Output; produces no value.
				{Args: []*Type{TString}, Impl: Go(describeWordHandler), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TAtom}, Impl: Go(describeWordHandler), Returns: []*Type{}, BarrierPos: -1},
				{
					Args:      []*Type{TAtom},
					QuoteArgs: map[int]bool{0: true},
					Impl:      Go(describeWordHandler),
					Returns:   []*Type{}, BarrierPos: -1,
				},
				{Args: []*Type{}, Impl: Go(describeSelfHandler), Returns: []*Type{}, BarrierPos: -1},
			},
		},

		// ---- referent (what a quoted atom refers to) ----
		{
			Name: "referent",

			Signatures: []Signature{
				{Args: []*Type{TAtom}, Impl: Go(referentHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},

		// ---- module / import ----
		{
			Name: "module",

			Signatures: []Signature{{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(moduleHandler, RunInCheck()),
				Returns:    []*Type{TModuleInst},
				BarrierPos: -1,
			}},
		},
		{
			Name: "import",

			Signatures: []Signature{
				{
					Args:       []*Type{TModuleInst},
					Impl:       Go(importAllHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					// The leading list holds export *names* to rename
					// (`import [Orig Renamed] mod`) — import name syntax,
					// not evaluable expressions. NoEvalArgs keeps them raw
					// (bare words never degrade to data, so without this
					// the unbound names would raise undefined_word).
					Args:       []*Type{TList, TModuleInst},
					NoEvalArgs: map[int]bool{0: true},
					Impl:       Go(importRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TAtom, TModuleInst},
					Impl:       Go(importSingleRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TString},
					Impl:       Go(importFileHandler, RunInCheck()),
					Returns:    []*Type{TModuleInst},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TList, TString},
					NoEvalArgs: map[int]bool{0: true},
					Impl:       Go(importFileRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				// Inline module forms: use /q to capture "module" as a quoted word
				// instead of executing it as a function.
				{
					Args:       []*Type{TAtom, TList},
					QuoteArgs:  map[int]bool{0: true},
					NoEvalArgs: map[int]bool{1: true},
					Impl:       Go(importInlineHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TList, TAtom, TList},
					QuoteArgs:  map[int]bool{1: true},
					NoEvalArgs: map[int]bool{0: true, 2: true},
					Impl:       Go(importInlineRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TAtom, TAtom, TList},
					QuoteArgs:  map[int]bool{1: true},
					NoEvalArgs: map[int]bool{2: true},
					Impl:       Go(importInlineSingleRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
			},
		},
		// ---- export (top-level no-op) ----
		//
		// `export` does its real work only inside an import context, where
		// RunModuleBody registers a collecting handler on the module
		// sub-registry that shadows this one. At the top level — running a
		// file directly (`aql foo.aql`) or in the REPL — there is nowhere
		// to export to, so `export` is a no-op that simply consumes its
		// (name, map) arguments. This lets a single file both run
		// standalone and export a namespace when imported. See §8.3 in the
		// DX report. The branch arity/types mirror the real handler in
		// native_module_module.go so dispatch behaves identically.
		{
			Name: "export",

			Signatures: []Signature{
				{
					Args:       []*Type{TAtom, TMap},
					Impl:       Go(exportNoopHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TString, TMap},
					Impl:       Go(exportNoopHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
			},
		},

		// timeout / await moved to the aql:time-util module — see
		// native/time_async_module.go.
	}
}

// ---- file I/O handlers ----

// extractPath returns the routing path from a Stream handle or a Pathon. A
// Stream atom (stdin/stdout/stderr) resolves to its internal stream sentinel
// so doRead/doWrite route to the host stream; a Pathon renders to its path
// string. File I/O is Pathon-only — string paths are not accepted — so these
// are the only two shapes reaching here.
func extractPath(v Value) string {
	if sentinel, ok := streamSentinel(v); ok {
		return sentinel
	}
	_as5, _ := AsPathon(v)
	return _as5.String()
}

// returnPath is the value a filesystem word hands back: the Pathon or Stream
// handle the caller passed. Every io target is now a value-carrying micron or
// stream handle, so the input is returned verbatim — the resolved path string
// is used only for the operation itself, never as the return.
func returnPath(v Value) Value {
	return v
}

func readHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	format := formatFromExt(r, path)
	if format == "" {
		format = "text"
	}
	return doRead(r, path, "utf8", format, "lf", nil)
}

func readOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	enc, format, _, nl, fmtExplicit, parserOpts := parseFileOpts(args[1])
	// Binary ({enc:'bytes'}) and positioned ({offset}/{length}) reads bypass
	// format decoding; anything else falls through to the normal decode path.
	if res, handled, err := tryBinaryRead(r, path, enc, args[1]); handled {
		if err != nil {
			return nil, r.AqlError("read_error", fmt.Sprintf("read: %v", err), "read")
		}
		return res, nil
	}
	if !fmtExplicit {
		if extFmt := formatFromExt(r, path); extFmt != "" {
			format = extFmt
		}
	}
	return doRead(r, path, enc, format, nl, parserOpts)
}

// Reversed handler for stack-first usage: "path" {opts} read
// In nearest-first stack matching, opts (top) maps to sig[0], path to sig[1].
func readOptsRevHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return readOptsHandler([]Value{args[1], args[0]}, ctx, stack, r)
}

func writeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	result, err := doWrite(r, path, content, "utf8", "text", "write", "lf", false)
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

func writeOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	enc, format, mode, nl, fmtExplicit, _ := parseFileOpts(args[2])
	if err := checkWritableFormat(r, format, fmtExplicit); err != nil {
		return nil, err
	}
	result, err := doWrite(r, path, content, enc, format, mode, nl, mapBoolOpt(args[2], "atomic", false))
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// checkWritableFormat rejects a write whose explicit {fmt:…} resolves to a
// read-only (parser-backed) format. Write is otherwise extension-blind —
// only an explicit fmt triggers the check — so every existing write call
// (no fmt, or an encodable fmt) is unaffected.
func checkWritableFormat(r *Registry, format string, fmtExplicit bool) error {
	if !fmtExplicit {
		return nil
	}
	f, ok := HostFormats(r)[format]
	if !ok {
		return nil // unknown format is doWrite's concern; nothing read-only to reject
	}
	if ro, ok := f.(ReadOnlyFormat); ok && ro.ReadOnly() {
		return r.AqlErrorHint("write_error",
			fmt.Sprintf("write: format %q is read-only", format), "write",
			"write supports text/json/jsonic/lines/csv/tsv")
	}
	return nil
}

// writeAnyHandler: [path/string/stream, any] -> [path/string/stream].
// Serializes non-string data with no options map required — the value is
// encoded as jsonic (the same default the options form upgrades to).
func writeAnyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content := valueToJsonic(args[1])
	result, err := doWrite(r, path, content, "utf8", "jsonic", "write", "lf", false)
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// write: [path/string, any, map] -> [path/string] (for non-string data with fmt)
func writeAnyOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	_, format, mode, nl, fmtExplicit, _ := parseFileOpts(args[2])
	if err := checkWritableFormat(r, format, fmtExplicit); err != nil {
		return nil, err
	}
	if format == "text" {
		format = "jsonic"
	}
	content := valueToJsonic(args[1])
	result, err := doWrite(r, path, content, "utf8", format, mode, nl, mapBoolOpt(args[2], "atomic", false))
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// ---- help / describe handlers ----

// helpOverviewHandler implements the 0-arg `help` word: a language
// overview plus a pointer at `describe` for per-word docs.
func helpOverviewHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fmt.Fprint(r.Output, help.Overview())
	return nil, nil
}

// describeSelfHandler implements the 0-arg `describe` word: a categorised
// guide to every built-in word and loadable module (the same guide the CLI
// `aql describe` prints).
func describeSelfHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	DescribeIndex(r.Output)
	return nil, nil
}

// describeWordHandler implements `describe <name>` for a word, category,
// module, or module word. The full dispatch — including class/surface schema
// views, module resolution, and load-if-unknown — lives in DescribeName so the
// `/describe` meta-command shares one implementation. See describe.go.
func describeWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	DescribeName(r, r.Output, ValToString(args[0]))
	return nil, nil
}

// formatTypeSchema renders the `describe <Class>` schema view for a
// name def'd to a class or object type. Field rule mirrors make: a
// type-valued constraint declares a required field, a concrete value
// declares a default (whose own type then constrains the field).
// Inherited fields from the refine chain are listed and marked.
func formatTypeSchema(name string, v Value) string {
	info, _ := AsClassType(v)
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", name, "class")
	if info.Name != "" {
		fmt.Fprintf(&b, " (%s)", info.Name)
	}
	b.WriteString("\n")
	if info.Parent != nil {
		fmt.Fprintf(&b, "\nRefines: %s\n", info.Parent.Name)
	}
	b.WriteString("\nFields:\n")
	all := info.AllFields()
	w := 0
	for _, k := range all.Keys() {
		if len(k) > w {
			w = len(k)
		}
	}
	for _, k := range all.Keys() {
		c, _ := all.Get(k)
		_, own := info.Fields.Get(k)
		inherited := ""
		if !own {
			inherited = "  (inherited)"
		}
		if IsConcrete(c) {
			fmt.Fprintf(&b, "  %-*s : %s = %s  (default)%s\n", w, k, c.Parent.Name(), c.String(), inherited)
		} else {
			fmt.Fprintf(&b, "  %-*s : %s  (required)%s\n", w, k, c.String(), inherited)
		}
	}
	fmt.Fprintf(&b, "\nInstances: make %s {…} — sealed (typed set of existing fields only).\n", name)
	return b.String()
}

// formatSurfaceSchema renders the `describe <Surface>` contract view:
// the required operation shapes and how to declare conformance.
func formatSurfaceSchema(name string, v Value) string {
	info, err := AsSurfaceType(v)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — surface", name)
	if info.Name != "" {
		fmt.Fprintf(&b, " (%s)", info.Name)
	}
	b.WriteString("\n\nRequired operations:\n")
	w := 0
	for _, k := range info.Required.Keys() {
		if len(k) > w {
			w = len(k)
		}
	}
	for _, k := range info.Required.Keys() {
		shape, _ := info.Required.Get(k)
		shapeStr := shape.String()
		if undef, ok := shape.Data.(FnUndefInfo); ok {
			var parts []string
			for _, spec := range undef.Sigs {
				parts = append(parts, renderSpec(spec))
			}
			shapeStr = strings.Join(parts, ", ")
		}
		fmt.Fprintf(&b, "  %-*s : %s\n", w, k, shapeStr)
	}
	fmt.Fprintf(&b, "\nConformance: <Type> exposes %s — explicit, checked loudly at declaration.\n", name)
	return b.String()
}

// referentHandler implements `referent`: given an atom, return what its name
// refers to. A captured snapshot (from `quote` or the load-time resolution
// pass) is returned as-is; otherwise the atom's name is resolved against the
// current bindings (the lazy fallback, which covers a bare `name/q` whose
// binding was made at runtime). An atom whose name is unbound is an error.
func referentHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	if ref, ok := AtomReferent(v); ok {
		return []Value{ref}, nil
	}
	name, err := AsAtom(v)
	if err != nil {
		return nil, r.AqlError("referent_error", "referent: expected an atom", "referent")
	}
	if bound, ok := r.Defs.Top(name); ok {
		return []Value{bound}, nil
	}
	return nil, r.AqlError("referent_error",
		fmt.Sprintf("referent: atom %q has no referent (name is unbound)", name), "referent")
}

// ---- module / import handlers ----

// exportNoopHandler is the top-level `export` handler: it discards the
// export name and map, producing no value. The real collecting handler
// is registered per-module in RunModuleBody and shadows this one. See
// the "export (top-level no-op)" entry above and §8.3 in the DX report.
//
// In CHECK mode it does one thing before discarding: it records every export
// value as a use of its def. A standalone module file (`aql check trie.aql`)
// reaches this no-op, not the collecting handler — so without this, every
// reference-exported public word (`export "X" { make: impl/r }`) is falsely
// flagged unused_def precisely because it is public. The map arrives already
// auto-evaluated, so `name/r` values are fn data whose name is read off the
// FnDefInfo; a bare name resolves through ResolveRef (which also records it).
func exportNoopHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if r == nil || !r.Check.IsActive() || len(args) < 2 || !IsConcrete(args[1]) {
		return nil, nil
	}
	m, err := RequireConcreteMap(args[1], "export")
	if err != nil {
		return nil, nil
	}
	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		if fnDef, ok := v.Data.(FnDefInfo); ok && fnDef.Name != "" {
			r.Check.RecordUse(fnDef.Name)
			continue
		}
		switch {
		case IsWord(v):
			w, _ := AsWord(v)
			r.Check.RecordUse(w.Name)
		case IsAtom(v):
			a, _ := AsAtom(v)
			r.Check.RecordUse(a)
		case v.Parent.ConformsTo(TString):
			// A bare name in an export map can survive as a String after
			// auto-eval; treat its text as the referenced name.
			if s, sok := AsString(v); sok == nil {
				r.Check.RecordUse(s)
			}
		}
	}
	return nil, nil
}

func moduleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("module_error", "module: argument must be a concrete list, got type literal", "module")
	}
	_lst, _ := AsList(args[0])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("module: %w", err)
	}
	return []Value{NewModuleInstance(desc)}, nil
}

func importAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[0])
	return nil, installExports(r, desc, nil)
}

func importRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[1])
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("import_error", "import: rename list must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

func importSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[1])
	newName, _ := args[0].AsConcreteAtom()
	return nil, installSingleRename(r, desc, newName)
}

func importFileHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[0].AsConcreteString()
	// In check mode the path string literal has been stripped to a
	// carrier (StripToCarriers), so `path` is empty. We can't read,
	// parse, or analyse the target file without it — but the importing
	// file itself is valid, so a hard error here would block `aql check`
	// on every file that imports a sibling. Treat the import as opaque:
	// return a Module carrier and let analysis continue (imported names
	// resolve to Any, which is the conservative check-mode default).
	if path == "" && r.Check.IsActive() {
		return []Value{NewCarrier(TModuleInst)}, nil
	}
	// In check mode the path literal is preserved (see StripToCarriers /
	// §4.3) so the importing file's cross-module references can be
	// resolved. Loading the target binds its export namespaces, so
	// `Pkg.word` no longer flags undefined_word. But the target may be
	// missing, unparseable, or itself fail to load — none of which should
	// block `aql check` on the importing file. On any such error, fall
	// back to an opaque Module carrier and let analysis continue.
	if r.Check.IsActive() {
		if err := loadImportForCheck(r, path); err != nil {
			return []Value{NewCarrier(TModuleInst)}, nil
		}
		return nil, nil
	}
	if isNativeModImport(path) {
		return nil, resolveNativeMod(r, path)
	}
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, err
		}
		return nil, installExports(r, desc, nil)
	}
	if isDataFile(r, path) {
		return loadDataFile(r, path)
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	return nil, installExports(r, desc, nil)
}

// loadImportForCheck resolves an import in check mode for its export
// symbols only. It mirrors the runtime import paths (native module,
// bare module, file module) but is the single guarded entry the
// check-mode branch of importFileHandler uses so a failure to load the
// target degrades to an opaque module rather than a hard check error.
// Data-file imports are skipped (no exports to bind). See §4.3.
func loadImportForCheck(r *Registry, path string) error {
	if isNativeModImport(path) {
		return resolveNativeMod(r, path)
	}
	if isDataFile(r, path) {
		return nil
	}
	resolved := path
	if !isFilePath(path) {
		var err error
		resolved, err = resolveBareModule(r, path)
		if err != nil {
			return err
		}
	}
	desc, err := loadFileModule(r, resolved)
	if err != nil {
		return err
	}
	return installExports(r, desc, nil)
}

func importFileRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[1].AsConcreteString()
	// Check mode: the path literal was stripped to a carrier. Skip the
	// import gracefully so `aql check` doesn't hard-fail (see
	// importFileHandler for the full rationale).
	if path == "" && r.Check.IsActive() {
		return nil, nil
	}
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, err
		}
		_lst, _ := AsList(args[0])
		return nil, installRenamedExports(r, desc, _lst.Slice())
	}
	if isDataFile(r, path) {
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
	return nil, installExports(r, desc, nil)
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

func (tt TemporalModuleTypes) timeoutListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return tt.doTimeout(r, args, true)
}

func (tt TemporalModuleTypes) timeoutWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return tt.doTimeout(r, args, false)
}

func (tt TemporalModuleTypes) doTimeout(r *Registry, args []Value, isList bool) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, r.AqlError("timeout_error", fmt.Sprintf("timeout: milliseconds must be non-negative, got %d", ms), "timeout")
	}
	callback := args[1]

	id := GenerateID("T_")
	// Fork now, on the scheduling goroutine, so the callback runs on an
	// isolated registry and cannot race the main interpreter when it
	// fires later on the timer goroutine.
	fork := r.ForkConcurrent()
	timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		RunTimerCallback(fork, callback, isList)
	})

	info := &TimeoutInfo{
		ID:    id,
		Ms:    ms,
		Timer: timer,
	}
	return []Value{tt.NewTimeout(info)}, nil
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

// ToMap / ToList implement eng.IdealConverter for Timeout / Interval:
// {id:… ms:…} and [id ms].
func (timeoutFormatBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ti, ok := v.Data.(*TimeoutInfo); ok {
		m.Set("id", NewString(ti.ID))
		m.Set("ms", NewInteger(ti.Ms))
	}
	return NewMap(m), nil
}
func (timeoutFormatBehavior) ToList(v Value) (Value, error) {
	if ti, ok := v.Data.(*TimeoutInfo); ok {
		return NewList([]Value{NewString(ti.ID), NewInteger(ti.Ms)}), nil
	}
	return NewList(nil), nil
}
func (intervalFormatBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ii, ok := v.Data.(*IntervalInfo); ok {
		m.Set("id", NewString(ii.ID))
		m.Set("ms", NewInteger(ii.Ms))
	}
	return NewMap(m), nil
}
func (intervalFormatBehavior) ToList(v Value) (Value, error) {
	if ii, ok := v.Data.(*IntervalInfo); ok {
		return NewList([]Value{NewString(ii.ID), NewInteger(ii.Ms)}), nil
	}
	return NewList(nil), nil
}
