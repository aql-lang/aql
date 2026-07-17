package native

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// io_fs.go holds the aql:io filesystem-structure words — stat, list, remove,
// move, and copy — plus the shared record/option helpers. Handlers route
// every filesystem touch through EffectiveFileOps(r) (so the in-memory
// backing engages under context.__sys.fs) and reuse extractPath for target
// routing. Mutating words call r.NoteEffect() (the compiled-mode effect fence).

// mapBoolOpt reads one boolean key from an options map, returning def when the
// value is not a concrete map or the key is absent. It is the single option
// reader all of the io words share.
func mapBoolOpt(opts Value, key string, def bool) bool {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return def
	}
	m, _ := AsMap(opts)
	if m == nil {
		return def
	}
	if v, ok := MapFieldBoolean(m, key); ok {
		return v
	}
	return def
}

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

// statHandler implements IO.stat. It returns a stat record, or `none` when
// the path does not exist (folding in exists/access); other errors (e.g. a
// symlink cycle, a policy denial) surface loudly. follow defaults true
// (dereference a symlink; {follow:false} for lstat); {resolve:true} sets
// .path to the absolute resolved form.
func statHandler(args []Value, r *Registry, fileType *Type, hasOpts bool) ([]Value, error) {
	follow, resolve := true, false
	if hasOpts {
		follow = mapBoolOpt(args[1], "follow", true)
		resolve = mapBoolOpt(args[1], "resolve", false)
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
		detail = mapBoolOpt(args[1], "detail", false)
		recursive = mapBoolOpt(args[1], "recursive", false)
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

// doRemoveWord implements IO.remove: {recursive:true} removes a directory
// tree, {force:true} ignores an absent path. Returns the target it removed.
func doRemoveWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	recursive, force := false, false
	if hasOpts {
		recursive = mapBoolOpt(args[1], "recursive", false)
		force = mapBoolOpt(args[1], "force", false)
	}
	path := extractPath(args[0])
	r.NoteEffect()
	err := EffectiveFileOps(r).Remove(path, recursive)
	if err != nil {
		if force && errors.Is(err, os.ErrNotExist) {
			return []Value{returnPath(args[0])}, nil
		}
		return nil, r.AqlError("remove_error", fmt.Sprintf("remove: %v", err), "remove")
	}
	return []Value{returnPath(args[0])}, nil
}

func ioRemoveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doRemoveWord(args, r, false)
}

func ioRemoveOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doRemoveWord(args, r, true)
}

// moveHandler implements IO.move (rename/move). Returns the destination.
func moveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	src := extractPath(args[0])
	dst := extractPath(args[1])
	r.NoteEffect()
	if err := EffectiveFileOps(r).Rename(src, dst); err != nil {
		return nil, r.AqlError("move_error", fmt.Sprintf("move: %v", err), "move")
	}
	return []Value{returnPath(args[1])}, nil
}

// doCopy copies src to dst: a symlink is recreated, a directory is copied
// recursively (requires recursive), a file's bytes are read and written.
func doCopy(r *Registry, src, dst string, recursive bool) error {
	ops := EffectiveFileOps(r)
	fi, err := ops.Stat(src, false)
	if err != nil {
		return err
	}
	switch {
	case fi.Symlink:
		return ops.Symlink(fi.Target, dst)
	case fi.IsDir:
		if !recursive {
			return fmt.Errorf("%q is a directory (use {recursive:true})", src)
		}
		return copyTree(r, src, dst)
	default:
		data, rerr := ops.ReadFile(src)
		if rerr != nil {
			return rerr
		}
		return ops.WriteFile(dst, data, fi.Mode.Perm())
	}
}

// copyTree recursively copies a directory, delegating each child to doCopy.
func copyTree(r *Registry, src, dst string) error {
	ops := EffectiveFileOps(r)
	if err := ops.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := ops.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := doCopy(r, src+"/"+e.Name, dst+"/"+e.Name, true); err != nil {
			return err
		}
	}
	return nil
}

// doCopyWord implements IO.copy. {recursive:true} copies a directory tree.
// Returns the destination.
func doCopyWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	src := extractPath(args[0])
	dst := extractPath(args[1])
	recursive := false
	if hasOpts {
		recursive = mapBoolOpt(args[2], "recursive", false)
	}
	r.NoteEffect()
	if err := doCopy(r, src, dst, recursive); err != nil {
		return nil, r.AqlError("copy_error", fmt.Sprintf("copy: %v", err), "copy")
	}
	return []Value{returnPath(args[1])}, nil
}

func copyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doCopyWord(args, r, false)
}

func copyOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doCopyWord(args, r, true)
}

// mapIntOpt reads one integer key from an options map, reporting presence.
func mapIntOpt(opts Value, key string) (int64, bool) {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return 0, false
	}
	m, _ := AsMap(opts)
	if m == nil {
		return 0, false
	}
	return MapFieldInteger(m, key)
}

// doLinkWord implements IO.link: a symbolic link (default) or a hard link
// ({hard:true}) at dst referring to src. Returns dst.
func doLinkWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	target := extractPath(args[0])
	link := extractPath(args[1])
	hard := false
	if hasOpts {
		hard = mapBoolOpt(args[2], "hard", false)
	}
	r.NoteEffect()
	var err error
	if hard {
		err = EffectiveFileOps(r).Link(target, link)
	} else {
		err = EffectiveFileOps(r).Symlink(target, link)
	}
	if err != nil {
		return nil, r.AqlError("link_error", fmt.Sprintf("link: %v", err), "link")
	}
	return []Value{returnPath(args[1])}, nil
}

func linkHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doLinkWord(args, r, false)
}

func linkOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doLinkWord(args, r, true)
}

// applyTouch is IO.touch's core: create the file if absent (Unix touch), then
// apply the metadata options {mode, mtime, atime, size}. It is a standalone
// function so its error branches are directly reachable in tests.
func applyTouch(ops capabilities.FileOps, path string, opts Value, hasOpts bool) error {
	if _, err := ops.Stat(path, false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if werr := ops.WriteFile(path, []byte{}, 0o644); werr != nil {
			return werr
		}
	}
	if !hasOpts {
		return nil
	}
	if mode, ok := mapIntOpt(opts, "mode"); ok {
		if err := ops.Chmod(path, os.FileMode(mode)); err != nil {
			return err
		}
	}
	mtime, hasM := mapIntOpt(opts, "mtime")
	atime, hasA := mapIntOpt(opts, "atime")
	if hasM || hasA {
		mt, at := time.Unix(mtime, 0), time.Unix(atime, 0)
		if !hasM {
			mt = at
		}
		if !hasA {
			at = mt
		}
		if err := ops.Chtimes(path, at, mt); err != nil {
			return err
		}
	}
	if size, ok := mapIntOpt(opts, "size"); ok {
		if err := ops.Truncate(path, size); err != nil {
			return err
		}
	}
	return nil
}

// doTouchWord implements IO.touch (the chmod/utimes/truncate/touch setter).
// Returns the path.
func doTouchWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	path := extractPath(args[0])
	var opts Value
	if hasOpts {
		opts = args[1]
	}
	r.NoteEffect()
	if err := applyTouch(EffectiveFileOps(r), path, opts, hasOpts); err != nil {
		return nil, r.AqlError("touch_error", fmt.Sprintf("touch: %v", err), "touch")
	}
	return []Value{returnPath(args[0])}, nil
}

func touchHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTouchWord(args, r, false)
}

func touchOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTouchWord(args, r, true)
}
