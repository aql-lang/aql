package native

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The "module", "import", and "export" words. The "module" and
// "import" words live in miscNatives (native_misc.go); "export" is
// registered dynamically inside RunModuleBody on the per-module
// sub-registry. The big bag of helpers (loadFileModule,
// installExports, resolveBareModule, etc.) lives below.

// RunModuleBody creates an isolated module engine, runs the given values,
// and returns a ModuleDesc with the collected exports.
func RunModuleBody(parent *Registry, elems []Value) (ModuleDesc, error) {
	modReg, err := DefaultRegistry()
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("module init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	// Inherit host-installed capabilities so the module body can read
	// files, encode/decode formats, and query the SQLite store using
	// the same backends as the parent registry.
	if ops := HostFileOps(parent); ops != nil {
		SetHostFileOps(modReg, ops)
	}
	mem, ok, err := parent.Capabilities.Get(CapMemFileOps)
	if err != nil {
		return ModuleDesc{}, err
	}
	if ok {
		if err := modReg.Capabilities.Set(CapMemFileOps, mem); err != nil {
			return ModuleDesc{}, err
		}
	}
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.BaseFile = parent.BaseFile
	// The TCO kill switch must follow module code: intra-module tail
	// recursion runs on THIS registry (module fns are InstallFnDef'd
	// here and dispatch via execMatch inside CallAQL's sub-engine), so
	// a host that disables elision on the parent expects module frames
	// to nest too. Counters stay per-registry.
	modReg.TCO.Disable = parent.TCO.Disable
	// The module's own source text, for error excerpts from fns that
	// run later via CallAQL on this sub-registry (file imports set it
	// to the module file; inline modules inherit the entry source).
	modReg.Source = parent.Source
	// CheckMode is deliberately NOT propagated to the module sub-
	// registry. Module bodies need concrete string literals (used as
	// export names / map keys) which carrier-stripping under CheckMode
	// destroys. A typo inside an inline module body therefore raises
	// a hard `undefined_word` error from stepWord and aborts the
	// import; the user sees a clear single-error diagnostic and can
	// fix the body before re-running. Top-level / if / do / for / fn
	// bodies all stay in CheckMode and collect every typo as usual.

	// Inherit the module CONFIG (InitFunc + Resolver) as a unit, then
	// run InitFunc to seed the child sub-registry with native words.
	// Using ModuleRegistry.InheritConfig rather than copying fields one
	// at a time is what keeps a future config field from being silently
	// dropped here — the Resolver omission that broke `import "aql:math-util"`
	// from file-imported modules (native imports only worked at the top
	// level) was exactly that field-by-field bug.
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	}

	// Inherit parent context so module can read parent values.
	// The module's Run will push its own copy-on-write layer on top.
	if parentCtx := parent.Contexts.Top(); parentCtx != nil {
		modReg.Contexts.PushExisting(parentCtx)
	}

	exports := make(map[string]*OrderedMap)

	exportHandler := func(name string, rawMap ReadMap) {
		resolved := NewOrderedMap()
		for _, key := range rawMap.Keys() {
			val, _ := rawMap.Get(key)
			resolved.Set(key, resolveModuleExport(modReg, val))
		}
		exports[name] = resolved
	}

	// Drop the inherited top-level no-op `export` (registered in the
	// default registry so files run standalone — see §8.3) before
	// installing the real collecting handler. RegisterNativeFunc APPENDS
	// signatures to an existing word, so without this delete the no-op's
	// (Atom|String, Map) sigs would shadow the collecting sigs and module
	// exports would be silently discarded.
	modReg.Defs.Delete("export")
	modReg.RegisterNativeFunc(NativeFunc{
		Name: "export",

		Signatures: []NativeSig{
			{
				Args: []*Type{TAtom, TMap},
				Handler: func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, parent.AqlError("export_error", "export: value must be a concrete map, got type literal", "export")
					}
					_as1, _ := eargs[0].AsConcreteAtom()
					_m, _ := AsMap(eargs[1])
					exportHandler(_as1, _m)
					return nil, nil
				},
				Returns: []*Type{}, BarrierPos: -1,
			},
			{
				Args: []*Type{TString, TMap},
				Handler: func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, parent.AqlError("export_error", "export: value must be a concrete map, got type literal", "export")
					}
					_as2, _ := eargs[0].AsConcreteString()
					_m, _ := AsMap(eargs[1])
					exportHandler(_as2, _m)
					return nil, nil
				},
				Returns: []*Type{}, BarrierPos: -1,
			},
		},
	})

	// A module body is code: unquoted words (as the parser yields them)
	// run; quoted strings and atoms are DATA and are left untouched. A
	// string is never silently promoted to a word and dispatched — a
	// quoted value whose text happens to name a word (`"def"`, `"print"`)
	// stays a string, the same contract `do` and every other evaluator
	// honour.
	input := make([]Value, len(elems))
	copy(input, elems)
	sub := New(modReg)
	_, err = sub.Run(input)
	if err != nil {
		return ModuleDesc{}, err
	}

	modID := parent.Modules.NextID()
	desc := ModuleDesc{
		ID:      modID,
		Exports: exports,
		Kind:    "inline", // overridden by loadFileModule / Resolve
	}
	return desc, nil
}

// isFilePath returns true if the string looks like a file path
// (starts with "/", "./", or "../").
func isFilePath(path string) bool {
	return strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../")
}

// isDataFile returns true if the path has a data file extension
// (.json, .jsonic, .csv, .tsv) — the formats `import` loads as values.
// Parser-backed formats (ini/xml/yaml/…) are deliberately excluded: they
// are read-only and reached via `read`, not `import`.
func isDataFile(r *Registry, path string) bool {
	f := formatFromExt(r, path)
	return f == "json" || f == "jsonic" || f == "csv" || f == "tsv"
}

// resolveModuleMain checks for .aql/aql.json in the given directory and
// returns the main file specified there. If the file doesn't exist or has
// no main property, returns "index.aql".
func resolveModuleMain(r *Registry, dir string) string {
	data, err := EffectiveFileOps(r).ReadFile(filepath.Join(dir, ".aql", "aql.json"))
	if err != nil {
		return "index.aql"
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "index.aql"
	}
	if main, ok := m["main"].(string); ok && main != "" {
		return main
	}
	return "index.aql"
}

// resolveImportPath resolves a file import path. If the registry has a BaseDir
// set (i.e. we are inside a module loaded from a file), relative paths are
// resolved against that directory. Otherwise the path is returned as-is
// (FileOps.ReadFile will resolve it against the process CWD).
// If the resolved path has no file extension, checks .aql/aql.json for a main
// property, falling back to index.lang.
func resolveImportPath(r *Registry, path string) string {
	resolved := path
	if r.BaseDir != "" && !filepath.IsAbs(path) {
		resolved = filepath.Join(r.BaseDir, path)
	}
	if filepath.Ext(resolved) == "" {
		main := resolveModuleMain(r, resolved)
		resolved = filepath.Join(resolved, main)
	}
	return resolved
}

// resolveBareModule resolves a bare module name (e.g. "foo") by searching for
// .aql/foo/ starting from the importing module's directory (BaseDir) or the
// current working directory, and walking up parent directories, following the
// CommonJS node_modules resolution pattern.
// Checks .aql/aql.json for a main property, falling back to index.lang.
func resolveBareModule(r *Registry, name string) (string, error) {
	var startDir string
	if r.BaseDir != "" {
		startDir = r.BaseDir
	} else {
		var err error
		startDir, err = EffectiveFileOps(r).ResolvePath(".")
		if err != nil {
			return "", fmt.Errorf("import: cannot resolve working directory: %w", err)
		}
	}

	dir := startDir
	for {
		modDir := filepath.Join(dir, ".aql", name)
		main := resolveModuleMain(r, modDir)
		candidate := filepath.Join(modDir, main)
		if _, err := EffectiveFileOps(r).ReadFile(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("import: module %q not found (searched .aql/%s/ from %s to /)", name, name, startDir)
}

// loadDataFile reads a data file (.json, .jsonic, .csv, .tsv) and returns
// the result as an AQL value on the stack. Uses doRead so CSV/TSV files
// get the same table + SQLite handling as the read word.
func loadDataFile(parent *Registry, path string) ([]Value, error) {
	format := formatFromExt(parent, path)
	if format == "" {
		return nil, parent.AqlError("import_error", fmt.Sprintf("import: unknown format for %s", path), "import")
	}
	resolved := resolveImportPath(parent, path)
	result, err := doRead(parent, resolved, "utf8", format, "lf", nil)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	return result, nil
}

// loadFileModule reads a file, parses it as AQL, and runs it as a module.
// The child module's BaseDir is set to the directory of the loaded file so
// that relative imports inside it resolve correctly.
func loadFileModule(parent *Registry, path string) (ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return ModuleDesc{}, fmt.Errorf("import: parser not configured for file import")
	}

	resolved := resolveImportPath(parent, path)

	data, err := EffectiveFileOps(parent).ReadFile(resolved)
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %w", err)
	}

	parsed, err := parent.ParseFunc(string(data))
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: parse %s: %w", resolved, err)
	}

	// Temporarily set parent BaseDir / BaseFile / Source so the child
	// module inherits the loaded file's directory, path, and SOURCE
	// TEXT (RunModuleBody copies all three; the source is what lets an
	// error raised later inside the module's fns render the module
	// file's excerpt — decision DX report finding 4).
	modDir := filepath.Dir(resolved)
	savedDir, savedFile, savedSrc := parent.BaseDir, parent.BaseFile, parent.Source
	parent.BaseDir = modDir
	parent.BaseFile = resolved
	parent.Source = string(data)
	desc, err := RunModuleBody(parent, parsed)
	parent.BaseDir, parent.BaseFile, parent.Source = savedDir, savedFile, savedSrc
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", resolved, err)
	}
	desc.Ref = path
	desc.Kind = "file"
	desc.File = resolved
	desc.Folder = modDir

	// If the module's aql.json declares resources, load them as a
	// "resource" export so they are available as Module.resource.key.
	if err := loadModuleResources(parent, modDir, &desc); err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", resolved, err)
	}

	return desc, nil
}

// loadModuleResources checks the module's .aql/aql.json for a "resource"
// property (map of key→filename). For each entry it loads the data file
// from the module directory and adds a "resource" export to the descriptor.
func loadModuleResources(r *Registry, modDir string, desc *ModuleDesc) error {
	data, err := EffectiveFileOps(r).ReadFile(filepath.Join(modDir, ".aql", "aql.json"))
	if err != nil {
		return nil // no aql.json — nothing to do
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	resMap, ok := m["resource"].(map[string]any)
	if !ok || len(resMap) == 0 {
		return nil
	}

	resources := NewOrderedMap()
	for key, v := range resMap {
		filename, ok := v.(string)
		if !ok {
			return fmt.Errorf("resource %q: value must be a string filename", key)
		}
		format := formatFromExt(r, filename)
		if format == "" {
			return fmt.Errorf("resource %q: unsupported file format %q", key, filename)
		}
		filePath := filepath.Join(modDir, filename)
		vals, err := doRead(r, filePath, "utf8", format, "lf", nil)
		if err != nil {
			return fmt.Errorf("resource %q: %w", key, err)
		}
		if len(vals) > 0 {
			resources.Set(key, vals[0])
		}
	}

	desc.Exports["resource"] = resources
	return nil
}

// installExports installs all exports from a module descriptor as defs.
// If names is nil, all exports are installed using their original names.
// Each export is bound as a ModuleExport whose $module points at the shared
// Module instance.
func installExports(r *Registry, desc ModuleDesc, names []string) {
	mod := NewModuleInstance(desc)
	if names == nil {
		for name, exportMap := range desc.Exports {
			InstallDef(r, name, NewModuleExport(name, exportMap, mod))
		}
		return
	}
	for _, name := range names {
		if exportMap, ok := desc.Exports[name]; ok {
			InstallDef(r, name, NewModuleExport(name, exportMap, mod))
		}
	}
}

// installRenamedExports applies a rename list to module exports and installs them.
func installRenamedExports(r *Registry, desc ModuleDesc, renameList []Value) error {
	if len(renameList) == 0 {
		return fmt.Errorf("import: empty rename list")
	}
	mod := NewModuleInstance(desc)

	if renameList[0].Parent.Equal(TList) {
		// Multiple rename pairs: [[from1 to1] [from2 to2] ...]
		for _, pair := range renameList {
			pairElems, _ := AsList(pair)
			if pairElems.Len() != 2 {
				return fmt.Errorf("import: rename pair must have exactly 2 elements")
			}
			fromName := valToAtomOrString(pairElems.Get(0))
			toName := valToAtomOrString(pairElems.Get(1))
			exportMap, ok := desc.Exports[fromName]
			if !ok {
				return fmt.Errorf("import: export %q not found in module", fromName)
			}
			InstallDef(r, toName, NewModuleExport(fromName, exportMap, mod))
		}
	} else {
		// Single rename pair: [from to]
		if len(renameList) != 2 {
			return fmt.Errorf("import: rename list must have exactly 2 elements (from to)")
		}
		fromName := valToAtomOrString(renameList[0])
		toName := valToAtomOrString(renameList[1])
		exportMap, ok := desc.Exports[fromName]
		if !ok {
			return fmt.Errorf("import: export %q not found in module", fromName)
		}
		InstallDef(r, toName, NewModuleExport(fromName, exportMap, mod))
	}
	return nil
}

// installSingleRename renames the single export in a module to newName.
// If the module has zero or more than one export, an error is returned.
func installSingleRename(r *Registry, desc ModuleDesc, newName string) error {
	mod := NewModuleInstance(desc)
	if len(desc.Exports) == 0 {
		return fmt.Errorf("import: module has no exports to rename")
	}
	if len(desc.Exports) != 1 {
		return fmt.Errorf("import: rename requires module with exactly one export, got %d", len(desc.Exports))
	}
	for name, exportMap := range desc.Exports {
		InstallDef(r, newName, NewModuleExport(name, exportMap, mod))
	}
	return nil
}

// resolveModuleExport resolves an export map value through the module's
// def stacks. If the value is a string, atom, or word that names a def'd word,
// the def body is returned. Otherwise the value is returned as-is.
func resolveModuleExport(modReg *Registry, v Value) Value {
	// A function value — typically produced by `name/r` in the export
	// map, which auto-evaluates to the bound fn as data — must carry the
	// module registry so it executes in module scope (resolving module-
	// private words) when called after import.
	if fnDef, ok := v.Data.(FnDefInfo); ok {
		if fnDef.Registry == nil {
			fnDef.Registry = modReg
			if v.Parent.Equal(TFnDef) {
				return NewFnDef(fnDef)
			}
			return NewFunction(fnDef)
		}
		return v
	}
	// A bare name (word/string/atom) resolves by lookup in the module
	// registry. After auto-eval this path is reached mainly in check
	// mode and for type/value names that resolved to themselves.
	var name string
	if IsWord(v) {
		_as3, _ := AsWord(v)
		name = _as3.Name
	} else if v.Parent.ConformsTo(TString) {
		name, _ = AsString(v)
	} else if IsAtom(v) {
		name, _ = AsAtom(v)
	} else {
		return v
	}
	// Resolution order: r.Types (canonical home for user-defined
	// types post-§5.2) wins, then DefStacks (value defs and the
	// fn-def stash). Without the r.Types check, exports of named
	// types (`export "color" {Color:Color}`) would leave the value
	// side as an unresolved Word.
	if tv, ok := modReg.TopTypeBody(name); ok {
		if fnDef, ok := tv.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				if tv.Parent.Equal(TFnDef) {
					return NewFnDef(fnDef)
				}
				return NewFunction(fnDef)
			}
		}
		return tv
	}
	if val, ok := modReg.Defs.Top(name); ok {
		// Tag FnDef values with the module's registry so they can
		// execute in the correct context (closure semantics).
		if fnDef, ok := val.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				if val.Parent.Equal(TFnDef) {
					return NewFnDef(fnDef)
				}
				return NewFunction(fnDef)
			}
		}
		return val
	}
	return v
}

// isNativeModImport returns true if the path looks like a native module
// import (starts with "aql:").
func isNativeModImport(path string) bool {
	return strings.HasPrefix(path, "aql:")
}

// resolveNativeMod resolves a native module import (e.g. "aql:math-util").
// The module name is extracted from the "aql:" prefix and resolved via the
// registry's NativeModResolver callback. The resolver returns a ModuleDesc
// whose exports are installed as defs, just like file-based modules.
// Each native module is loaded at most once per registry.
func resolveNativeMod(r *Registry, path string) error {
	name := strings.TrimPrefix(path, "aql:")
	if name == "" {
		return fmt.Errorf("import: empty native module name in %q", path)
	}
	if desc, ok := r.Modules.LoadedDesc(name); ok {
		// Already resolved once in this registry. Re-bind any namespace
		// defs that are no longer present — a fn-body / property-body
		// import installs the namespace via InstallDef, which CallAQL's
		// def-cleanup then strips, leaving the module marked loaded but
		// `pkg` unbound for the next call. Rebinding only the absent names
		// keeps repeated top-level imports from stacking shadow bindings.
		// See §11b.1 in the DX report.
		ensureExportsBound(r, desc)
		return nil
	}
	if r.Modules.Resolver == nil {
		return fmt.Errorf("import: native module resolver not configured (cannot import %q)", path)
	}
	desc, err := r.Modules.Resolver(name, r)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	installExports(r, desc, nil)
	r.Modules.MarkLoaded(name, desc)
	return nil
}

// ResolveAnyModule resolves a module reference to its descriptor WITHOUT
// installing its exports, mirroring import's native / bare / file dispatch. It
// lets tooling — notably `aql describe` — load and introspect a module that is
// not one of the resolver's compiled-in builtins ("attempt to load a module if
// unknown"). For an "aql:" reference the registry must have a module Resolver
// installed (modules.InstallResolver). Data-file references are rejected: they
// have no exports to describe.
func ResolveAnyModule(r *Registry, ref string) (ModuleDesc, error) {
	if isNativeModImport(ref) {
		name := strings.TrimPrefix(ref, "aql:")
		if name == "" {
			return ModuleDesc{}, fmt.Errorf("empty native module name in %q", ref)
		}
		if r.Modules.Resolver == nil {
			return ModuleDesc{}, fmt.Errorf("native module resolver not configured (cannot resolve %q)", ref)
		}
		return r.Modules.Resolver(name, r)
	}
	if isDataFile(r, ref) {
		return ModuleDesc{}, fmt.Errorf("%q is a data file, not a module", ref)
	}
	if !isFilePath(ref) {
		resolved, err := resolveBareModule(r, ref)
		if err != nil {
			return ModuleDesc{}, err
		}
		return loadFileModule(r, resolved)
	}
	return loadFileModule(r, ref)
}

// ensureExportsBound re-installs any module-namespace defs that are not
// currently bound. Used when re-importing an already-resolved native
// module whose namespace binding was torn down (e.g. by CallAQL's
// fn-body def-cleanup). Only absent names are installed, so a plain
// repeated top-level import stays a no-op rather than stacking bindings.
func ensureExportsBound(r *Registry, desc ModuleDesc) {
	for name, exportMap := range desc.Exports {
		if !r.Defs.Has(name) {
			InstallDef(r, name, NewMap(exportMap))
		}
	}
}

// valToAtomOrString extracts a string from a Value that is an atom, string, or word.
func valToAtomOrString(v Value) string {
	if IsWord(v) {
		_as4, _ := AsWord(v)
		return _as4.Name
	}
	if IsAtom(v) {
		_as5, _ := AsAtom(v)
		return _as5
	}
	if v.Parent.ConformsTo(TString) {
		_as6, _ := AsString(v)
		return _as6
	}
	return v.String()
}
