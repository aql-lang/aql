package engine

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
	if mem, ok := parent.Capability(CapMemFileOps); ok {
		modReg.SetCapability(CapMemFileOps, mem)
	}
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	// CheckMode is deliberately NOT propagated to the module sub-
	// registry. Module bodies need concrete string literals (used as
	// export names / map keys) which carrier-stripping under CheckMode
	// destroys. A typo inside an inline module body therefore raises
	// a hard `undefined_word` error from stepWord and aborts the
	// import; the user sees a clear single-error diagnostic and can
	// fix the body before re-running. Top-level / if / do / for / fn
	// bodies all stay in CheckMode and collect every typo as usual.

	// Let the native package (or other extension packages) register
	// their words in the module's sub-registry. Propagate the hook
	// so nested modules also get these words.
	if parent.ModuleInitFunc != nil {
		parent.ModuleInitFunc(modReg)
		modReg.ModuleInitFunc = parent.ModuleInitFunc
	}

	// Inherit parent context so module can read parent values.
	// The module's Run will push its own copy-on-write layer on top.
	if parentCtx := parent.ContextStore(); parentCtx != nil {
		modReg.PushExistingContext(parentCtx)
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

	modReg.RegisterNativeFunc(NativeFunc{
		Name:              "export",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TAtom, TMap},
				Handler: func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, fmt.Errorf("export: value must be a concrete map, got type literal")
					}
					_as1, _ := eargs[0].AsConcreteAtom()
					exportHandler(_as1, eargs[1].AsMap())
					return nil, nil
				},
				Returns: []Type{},
			},
			{
				Args: []Type{TString, TMap},
				Handler: func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, fmt.Errorf("export: value must be a concrete map, got type literal")
					}
					_as2, _ := eargs[0].AsConcreteString()
					exportHandler(_as2, eargs[1].AsMap())
					return nil, nil
				},
				Returns: []Type{},
			},
		},
	})

	// Promote strings to words for code evaluation inside module.
	promoteToWord := func(v Value) Value {
		if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
			name, _ := v.AsString()
			if modReg.Lookup(name) != nil {
				return NewWord(name)
			}
		}
		return v
	}

	input := make([]Value, len(elems))
	for i, e := range elems {
		input[i] = promoteToWord(e)
	}
	sub := New(modReg)
	_, err = sub.Run(input)
	if err != nil {
		return ModuleDesc{}, err
	}

	modID := parent.NextModuleID()
	desc := ModuleDesc{
		ID:      modID,
		Exports: exports,
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
// (.json, .jsonic, .csv, .tsv).
func isDataFile(path string) bool {
	f := formatFromExt(path)
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
// property, falling back to index.aql.
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
// Checks .aql/aql.json for a main property, falling back to index.aql.
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
	format := formatFromExt(path)
	if format == "" {
		return nil, fmt.Errorf("import: unknown format for %s", path)
	}
	resolved := resolveImportPath(parent, path)
	result, err := doRead(parent, resolved, "utf8", format, "lf")
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

	// Temporarily set parent BaseDir so the child module inherits the
	// loaded file's directory (RunModuleBody copies BaseDir).
	modDir := filepath.Dir(resolved)
	saved := parent.BaseDir
	parent.BaseDir = modDir
	desc, err := RunModuleBody(parent, parsed)
	parent.BaseDir = saved
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", resolved, err)
	}

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
		format := formatFromExt(filename)
		if format == "" {
			return fmt.Errorf("resource %q: unsupported file format %q", key, filename)
		}
		filePath := filepath.Join(modDir, filename)
		vals, err := doRead(r, filePath, "utf8", format, "lf")
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
func installExports(r *Registry, desc ModuleDesc, names []string) {
	if names == nil {
		for name, exportMap := range desc.Exports {
			InstallDef(r, name, NewMap(exportMap))
		}
		return
	}
	for _, name := range names {
		if exportMap, ok := desc.Exports[name]; ok {
			InstallDef(r, name, NewMap(exportMap))
		}
	}
}

// installRenamedExports applies a rename list to module exports and installs them.
func installRenamedExports(r *Registry, desc ModuleDesc, renameList []Value) error {
	if len(renameList) == 0 {
		return fmt.Errorf("import: empty rename list")
	}

	if renameList[0].VType.Equal(TList) {
		// Multiple rename pairs: [[from1 to1] [from2 to2] ...]
		for _, pair := range renameList {
			pairElems := pair.AsList()
			if pairElems.Len() != 2 {
				return fmt.Errorf("import: rename pair must have exactly 2 elements")
			}
			fromName := valToAtomOrString(pairElems.Get(0))
			toName := valToAtomOrString(pairElems.Get(1))
			exportMap, ok := desc.Exports[fromName]
			if !ok {
				return fmt.Errorf("import: export %q not found in module", fromName)
			}
			InstallDef(r, toName, NewMap(exportMap))
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
		InstallDef(r, toName, NewMap(exportMap))
	}
	return nil
}

// installSingleRename renames the single export in a module to newName.
// If the module has zero or more than one export, an error is returned.
func installSingleRename(r *Registry, desc ModuleDesc, newName string) error {
	if len(desc.Exports) == 0 {
		return fmt.Errorf("import: module has no exports to rename")
	}
	if len(desc.Exports) != 1 {
		return fmt.Errorf("import: rename requires module with exactly one export, got %d", len(desc.Exports))
	}
	for _, exportMap := range desc.Exports {
		InstallDef(r, newName, NewMap(exportMap))
	}
	return nil
}

// resolveModuleExport resolves an export map value through the module's
// def stacks. If the value is a string, atom, or word that names a def'd word,
// the def body is returned. Otherwise the value is returned as-is.
func resolveModuleExport(modReg *Registry, v Value) Value {
	var name string
	if v.IsWord() {
		_as3, _ := v.AsWord()
		name = _as3.Name
	} else if v.VType.Matches(TString) {
		name, _ = v.AsString()
	} else if v.IsAtom() {
		name, _ = v.AsAtom()
	} else {
		return v
	}
	// Resolution order: r.Types (canonical home for user-defined
	// types post-§5.2) wins, then DefStacks (value defs and the
	// fn-def stash). Without the r.Types check, exports of named
	// types (`export "color" {Color:Color}`) would leave the value
	// side as an unresolved Word.
	if tv, ok := modReg.TopOfTypeStack(name); ok {
		if fnDef, ok := tv.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				if tv.VType.Equal(TFnDef) {
					return NewFnDef(fnDef)
				}
				return NewFunction(fnDef)
			}
		}
		return tv
	}
	if val, ok := modReg.TopOfDefStack(name); ok {
		// Tag FnDef values with the module's registry so they can
		// execute in the correct context (closure semantics).
		if fnDef, ok := val.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				if val.VType.Equal(TFnDef) {
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

// resolveNativeMod resolves a native module import (e.g. "aql:math").
// The module name is extracted from the "aql:" prefix and resolved via the
// registry's NativeModResolver callback. The resolver returns a ModuleDesc
// whose exports are installed as defs, just like file-based modules.
// Each native module is loaded at most once per registry.
func resolveNativeMod(r *Registry, path string) error {
	name := strings.TrimPrefix(path, "aql:")
	if name == "" {
		return fmt.Errorf("import: empty native module name in %q", path)
	}
	if r.IsNativeModLoaded(name) {
		return nil // already loaded
	}
	if r.NativeModResolver == nil {
		return fmt.Errorf("import: native module resolver not configured (cannot import %q)", path)
	}
	desc, err := r.NativeModResolver(name, r)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	installExports(r, desc, nil)
	r.MarkNativeModLoaded(name)
	return nil
}

// valToAtomOrString extracts a string from a Value that is an atom, string, or word.
func valToAtomOrString(v Value) string {
	if v.IsWord() {
		_as4, _ := v.AsWord()
		return _as4.Name
	}
	if v.IsAtom() {
		_as5, _ := v.AsAtom()
		return _as5
	}
	if v.VType.Matches(TString) {
		_as6, _ := v.AsString()
		return _as6
	}
	return v.String()
}
