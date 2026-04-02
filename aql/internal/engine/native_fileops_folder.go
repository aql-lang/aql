package engine

import "fmt"

// registerFolder registers the "folder" word for creating directories.
//
// Signatures (forward precedence):
//
//	[TOptions, TPath]  – folder (make Options {parents:true}) (make Path [...])
//	[TPath]            – folder (make Path [...])  (parents defaults to true)
//
// Creates a directory at the given path. If the path is relative, it is
// resolved against the process working directory. If parents=true (default),
// parent directories are created as needed. Idempotent: succeeds if the
// directory already exists. Returns the Path on success, or an Error.
func registerFolder(r *Registry) {
	doFolder := func(p PathInfo, parents bool, reg *Registry) ([]Value, error) {
		ops := reg.EffectiveFileOps()
		pathStr := p.String()

		if parents {
			if err := ops.MkdirAll(pathStr, 0755); err != nil {
				return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
			}
		} else {
			// Resolve path then create single directory.
			resolved, err := ops.ResolvePath(pathStr)
			if err != nil {
				return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
			}
			if err := ops.MkdirAll(resolved, 0755); err != nil {
				return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
			}
		}

		return []Value{NewPath(p.Parts, p.Abs)}, nil
	}

	folderOptsHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		optsVal := args[0]
		pathVal := args[1]
		if !pathVal.IsPath() {
			return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
		}
		parents := true
		if optsMap := optsVal.AsMap(); optsMap != nil {
			if v, ok := optsMap.Get("parents"); ok && v.VType.Matches(TBoolean) {
				parents = v.AsBoolean()
			}
		}
		return doFolder(pathVal.AsPath(), parents, reg)
	}

	folderHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		pathVal := args[0]
		if !pathVal.IsPath() {
			return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
		}
		return doFolder(pathVal.AsPath(), true, reg)
	}

	r.Register("folder",
		Signature{
			Args:    []Type{TOptions, TPath},
			Handler: folderOptsHandler,
		},
		Signature{
			Args:    []Type{TPath},
			Handler: folderHandler,
		},
	)
}
