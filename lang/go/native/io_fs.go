package native

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// io_fs.go holds the aql:io filesystem-structure words — stat and list —
// plus the shared record/option helpers. Handlers route every filesystem
// touch through EffectiveFileOps(r) (so the in-memory backing engages under
// context.__sys.fs) and reuse extractPath for target routing.

// mtimeUnix renders a modification time as Unix seconds, mapping the zero
// time (an unset in-memory mtime) to 0 rather than a large negative number.
func mtimeUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// buildStatRecord turns a host FileInfo into the AQL stat record: a Map with
// name/path/type/size/mode/mtime (and target for a symlink). `type` is a
// FileType atom so `(stat p).type is IO.FileType` holds.
func buildStatRecord(fi capabilities.FileInfo, path string, fileType *Type) Value {
	om := NewOrderedMap()
	om.Set("name", NewString(fi.Name))
	om.Set("path", NewString(path))
	om.Set("type", newFileTypeAtom(fileTypeName(fi), fileType))
	om.Set("size", NewInteger(fi.Size))
	om.Set("mode", NewInteger(int64(fi.Mode.Perm())))
	om.Set("mtime", NewInteger(mtimeUnix(fi.ModTime)))
	if fi.Symlink {
		om.Set("target", NewString(fi.Target))
	}
	return NewMap(om)
}

// parseStatOpts reads stat's option map: follow (default true — dereference a
// symlink, {follow:false} for lstat) and resolve ({resolve:true} sets .path
// to the absolute resolved form).
func parseStatOpts(opts Value) (follow, resolve bool) {
	follow = true
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return
	}
	m, _ := AsMap(opts)
	if m == nil {
		return
	}
	if v, ok := MapFieldBoolean(m, "follow"); ok {
		follow = v
	}
	if v, ok := MapFieldBoolean(m, "resolve"); ok {
		resolve = v
	}
	return
}

// statHandler implements IO.stat. It returns a stat record, or `none` when
// the path does not exist (folding in exists/access); other errors (e.g. a
// symlink cycle, a policy denial) surface loudly.
func statHandler(args []Value, r *Registry, fileType *Type, hasOpts bool) ([]Value, error) {
	follow, resolve := true, false
	if hasOpts {
		follow, resolve = parseStatOpts(args[1])
	}
	path := extractPath(args[0])
	fi, err := EffectiveFileOps(r).Stat(path, follow)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return nil, r.AqlError("stat_error", fmt.Sprintf("stat: %v", err), "stat")
	}
	displayPath := path
	if resolve {
		if rp, rerr := EffectiveFileOps(r).ResolvePath(path); rerr == nil {
			displayPath = rp
		}
	}
	return []Value{buildStatRecord(fi, displayPath, fileType)}, nil
}

// parseListOpts reads list's option map: detail ({detail:true} returns stat
// records instead of names) and recursive ({recursive:true} walks the tree).
func parseListOpts(opts Value) (detail, recursive bool) {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return
	}
	m, _ := AsMap(opts)
	if m == nil {
		return
	}
	if v, ok := MapFieldBoolean(m, "detail"); ok {
		detail = v
	}
	if v, ok := MapFieldBoolean(m, "recursive"); ok {
		recursive = v
	}
	return
}

// listEntry pairs a directory entry's metadata with its path relative to the
// listed root (the bare name for a shallow listing, a slash-joined path for a
// recursive walk).
type listEntry struct {
	info    capabilities.FileInfo
	relPath string
}

// collectEntries lists root's children (recursing into subdirectories when
// recursive is set), prefixing relative paths with prefix.
func collectEntries(r *Registry, root, prefix string, recursive bool) ([]listEntry, error) {
	infos, err := EffectiveFileOps(r).ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]listEntry, 0, len(infos))
	for _, fi := range infos {
		rel := fi.Name
		if prefix != "" {
			rel = prefix + "/" + fi.Name
		}
		out = append(out, listEntry{info: fi, relPath: rel})
		if recursive && fi.IsDir {
			child, cerr := collectEntries(r, root+"/"+fi.Name, rel, recursive)
			if cerr != nil {
				return nil, cerr
			}
			out = append(out, child...)
		}
	}
	return out, nil
}

// listHandler implements IO.list (readdir/walk). It returns a List of name
// strings, or of stat records with {detail:true}; {recursive:true} walks the
// whole subtree.
func listHandler(args []Value, r *Registry, fileType *Type, hasOpts bool) ([]Value, error) {
	detail, recursive := false, false
	if hasOpts {
		detail, recursive = parseListOpts(args[1])
	}
	path := extractPath(args[0])
	entries, err := collectEntries(r, path, "", recursive)
	if err != nil {
		return nil, r.AqlError("list_error", fmt.Sprintf("list: %v", err), "list")
	}
	items := make([]Value, 0, len(entries))
	for _, e := range entries {
		if detail {
			items = append(items, buildStatRecord(e.info, e.relPath, fileType))
		} else {
			items = append(items, NewString(e.relPath))
		}
	}
	return []Value{NewList(items)}, nil
}
