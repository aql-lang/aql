package native

import (
	"path/filepath"
	"strings"
)

// fileInfoNatives registers the source-location words __folder and __file.
// They report the location of the AQL source FILE in which they appear:
//
//   - __folder → the absolute directory Path of the current file.
//   - __file   → the base filename String of the current file.
//
// Resolution is per-file: inside a module loaded from disk, both reflect
// that module's own file (loadFileModule sets r.BaseFile to the loaded
// file's full path; RunModuleBody propagates it to the module body). At
// the REPL or for a string-evaluated program — where there is no source
// file — both return empty (an empty Path / empty String), never an error.
var fileInfoNatives = []NativeFunc{
	{
		Name: "__folder",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Impl:    Go(srcFolderHandler),
			Returns: []*Type{TPath}, BarrierPos: 0,
		}},
	},
	{
		Name: "__file",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Impl:    Go(srcFileHandler),
			Returns: []*Type{TString}, BarrierPos: 0,
		}},
	},
}

// srcFolderHandler returns the current file's directory as an absolute Path.
// With no source file it returns an empty Path.
func srcFolderHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	dir := ""
	if r != nil && r.BaseFile != "" {
		dir = filepath.Dir(r.BaseFile)
	} else if r != nil {
		dir = r.BaseDir
	}
	if dir == "" {
		return []Value{NewPath(nil, false)}, nil
	}
	abs := filepath.IsAbs(dir)
	parts := splitPathSegments(dir)
	return []Value{NewPath(parts, abs)}, nil
}

// srcFileHandler returns the current file's base name as a String. With no
// source file it returns the empty string.
func srcFileHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if r == nil || r.BaseFile == "" {
		return []Value{NewString("")}, nil
	}
	return []Value{NewString(filepath.Base(r.BaseFile))}, nil
}

// splitPathSegments breaks an OS path into its non-empty segments,
// dropping the leading separator (the abs flag carries "/"-rootedness).
func splitPathSegments(p string) []string {
	cleaned := filepath.ToSlash(filepath.Clean(p))
	raw := strings.Split(cleaned, "/")
	parts := make([]string, 0, len(raw))
	for _, s := range raw {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}
