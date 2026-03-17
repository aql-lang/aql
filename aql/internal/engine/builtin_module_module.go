package engine

import (
	"fmt"
)

// registerModule registers the "module", "export", and "import" words.
//
// module works like do but with a completely fresh, isolated sub-engine
// (new registry, new bus). Inside a module the "export" word is available
// to declare exports. The return value is a module descriptor (ModuleDesc)
// which can be the subject of def.
//
// export: [atom_or_string map] registers an export name and map within
// the module. The export map's values are evaluated in the module context.
//
// import: injects exported names from a module descriptor into the current
// engine as defs.
//   - [module-desc]                    — use export names as-is
//   - [[atom atom] module-desc]        — rename single export (from to)
//   - [[:pairs...] module-desc]        — rename multiple exports
func registerModule(r *Registry) {
	// module: [list] -> [module-desc]
	r.Register("module", Signature{
		Args: []Type{TList},
		Handler: func(args []Value) ([]Value, error) {
			desc, err := runModuleBody(r, args[0].AsList())
			if err != nil {
				return nil, fmt.Errorf("module: %w", err)
			}
			return []Value{NewModule(desc)}, nil
		},
	})

	// import: [module-desc] -> [] — import all exports as defs
	importAllHandler := func(args []Value) ([]Value, error) {
		desc := args[0].AsModule()
		installExports(r, desc, nil)
		return nil, nil
	}

	// import: [list module-desc] -> [] — rename imports
	importRenameHandler := func(args []Value) ([]Value, error) {
		desc := args[1].AsModule()
		return nil, installRenamedExports(r, desc, args[0].AsList())
	}

	// import: [string] -> [] or [value] — import from a file path.
	// For .json/.jsonic files, parses the file and pushes the data value.
	// For other files, reads, parses as AQL, and executes in an isolated module engine.
	importFileHandler := func(args []Value) ([]Value, error) {
		path := args[0].AsString()
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

	// import: [list string] -> [] — import from file with renaming.
	importFileRenameHandler := func(args []Value) ([]Value, error) {
		path := args[1].AsString()
		if isDataFile(path) {
			return nil, fmt.Errorf("import: rename not supported for data files (%s)", path)
		}
		desc, err := loadFileModule(r, path)
		if err != nil {
			return nil, err
		}
		return nil, installRenamedExports(r, desc, args[0].AsList())
	}

	r.Register("import",
		Signature{
			Args:    []Type{TModule},
			Handler: importAllHandler,
		},
		Signature{
			Args:    []Type{TList, TModule},
			Handler: importRenameHandler,
		},
		Signature{
			Args:    []Type{TString},
			Handler: importFileHandler,
		},
		Signature{
			Args:    []Type{TList, TString},
			Handler: importFileRenameHandler,
		},
	)
}

// runModuleBody creates an isolated module engine, runs the given values,
// and returns a ModuleDesc with the collected exports.
func runModuleBody(parent *Registry, elems []Value) (ModuleDesc, error) {
	modReg, err := DefaultRegistry()
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("module init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	modReg.FileOps = parent.FileOps
	modReg.ParseFunc = parent.ParseFunc

	// Inherit parent context so module can read parent values.
	// The module's Run will push its own copy-on-write layer on top.
	if parentCtx := parent.Context(); parentCtx != nil {
		copied := make(map[string]Value, len(parentCtx))
		for k, v := range parentCtx {
			copied[k] = v
		}
		modReg.ctxStack = append(modReg.ctxStack, copied)
	}

	exports := make(map[string]*OrderedMap)

	exportHandler := func(name string, rawMap *OrderedMap) {
		resolved := NewOrderedMap()
		for _, key := range rawMap.Keys() {
			val, _ := rawMap.Get(key)
			resolved.Set(key, resolveModuleExport(modReg, val))
		}
		exports[name] = resolved
	}

	modReg.Register("export", Signature{
		Args: []Type{TWord, TMap},
		Handler: func(eargs []Value) ([]Value, error) {
			exportHandler(defName(eargs[0]), eargs[1].AsMap())
			return nil, nil
		},
	}, Signature{
		Args: []Type{TAtom, TMap},
		Handler: func(eargs []Value) ([]Value, error) {
			exportHandler(eargs[0].AsAtom(), eargs[1].AsMap())
			return nil, nil
		},
	}, Signature{
		Args: []Type{TString, TMap},
		Handler: func(eargs []Value) ([]Value, error) {
			exportHandler(eargs[0].AsString(), eargs[1].AsMap())
			return nil, nil
		},
	})

	// Promote strings to words for code evaluation inside module.
	promoteToWord := func(v Value) Value {
		if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
			name := v.AsString()
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
	parent.Modules[modID] = desc
	return desc, nil
}

// isDataFile returns true if the path has a .json or .jsonic extension.
func isDataFile(path string) bool {
	f := formatFromExt(path)
	return f == "json" || f == "jsonic"
}

// loadDataFile reads a .json or .jsonic file, parses it with jsonic,
// and returns the result as an AQL data value on the stack.
func loadDataFile(parent *Registry, path string) ([]Value, error) {
	data, err := parent.FileOps.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}

	format := formatFromExt(path)
	f, ok := parent.Formats[format]
	if !ok {
		return nil, fmt.Errorf("import: unknown format: %s", format)
	}

	result, err := f.Decode(string(data))
	if err != nil {
		return nil, fmt.Errorf("import: %s: %w", path, err)
	}

	return result, nil
}

// loadFileModule reads a file, parses it as AQL, and runs it as a module.
func loadFileModule(parent *Registry, path string) (ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return ModuleDesc{}, fmt.Errorf("import: parser not configured for file import")
	}

	data, err := parent.FileOps.ReadFile(path)
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %w", err)
	}

	parsed, err := parent.ParseFunc(string(data))
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: parse %s: %w", path, err)
	}

	desc, err := runModuleBody(parent, parsed)
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", path, err)
	}
	return desc, nil
}

// installExports installs all exports from a module descriptor as defs.
// If names is nil, all exports are installed using their original names.
func installExports(r *Registry, desc ModuleDesc, names []string) {
	if names == nil {
		for name, exportMap := range desc.Exports {
			installDef(r, name, NewMap(exportMap))
		}
		return
	}
	for _, name := range names {
		if exportMap, ok := desc.Exports[name]; ok {
			installDef(r, name, NewMap(exportMap))
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
			if len(pairElems) != 2 {
				return fmt.Errorf("import: rename pair must have exactly 2 elements")
			}
			fromName := valToAtomOrString(pairElems[0])
			toName := valToAtomOrString(pairElems[1])
			exportMap, ok := desc.Exports[fromName]
			if !ok {
				return fmt.Errorf("import: export %q not found in module", fromName)
			}
			installDef(r, toName, NewMap(exportMap))
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
		installDef(r, toName, NewMap(exportMap))
	}
	return nil
}

// resolveModuleExport resolves an export map value through the module's
// def stacks. If the value is a string, atom, or word that names a def'd word,
// the def body is returned. Otherwise the value is returned as-is.
func resolveModuleExport(modReg *Registry, v Value) Value {
	var name string
	if v.IsWord() {
		name = v.AsWord().Name
	} else if v.VType.Matches(TString) {
		name = v.AsString()
	} else if v.IsAtom() {
		name = v.AsAtom()
	} else {
		return v
	}
	stack := modReg.DefStacks[name]
	if len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return v
}

// valToAtomOrString extracts a string from a Value that is an atom, string, or word.
func valToAtomOrString(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
	if v.IsAtom() {
		return v.AsAtom()
	}
	if v.VType.Matches(TString) {
		return v.AsString()
	}
	return v.String()
}
