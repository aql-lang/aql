package native
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"fmt"
)
// RegisterFolder registers the "folder" word for creating directories.
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
func RegisterFolder(r *engine.Registry) {
	doFolder := func(p engine.PathInfo, parents bool, reg *engine.Registry) ([]engine.Value, error) {
		ops := engine.EffectiveFileOps(reg)
		pathStr := p.String()

		if parents {
			if err := ops.MkdirAll(pathStr, 0755); err != nil {
				return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
			}
		} else {
			// Resolve path then create single directory.
			resolved, err := ops.ResolvePath(pathStr)
			if err != nil {
				return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
			}
			if err := ops.MkdirAll(resolved, 0755); err != nil {
				return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
			}
		}

		return []engine.Value{engine.NewPath(p.Parts, p.Abs)}, nil
	}

	folderOptsHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
		optsVal := args[0]
		pathVal := args[1]
		if !pathVal.IsPath() {
			return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
		}
		parents := true
		if optsMap := optsVal.AsMap(); optsMap != nil {
			if v, ok := optsMap.Get("parents"); ok && v.VType.Matches(engine.TBoolean) {
				parents, _ = v.AsBoolean()
			}
		}
		_as0, _ := pathVal.AsPath()
		return doFolder(_as0, parents, reg)
	}

	folderHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
		pathVal := args[0]
		if !pathVal.IsPath() {
			return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
		}
		_as1, _ := pathVal.AsPath()
		return doFolder(_as1, true, reg)
	}

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "folder",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TOptions, engine.TPath},
				Handler: folderOptsHandler,
				Returns: []engine.Type{engine.TList},
			},
			{
				Args:    []engine.Type{engine.TPath},
				Handler: folderHandler,
				Returns: []engine.Type{engine.TList},
			},
		},
	})
}
