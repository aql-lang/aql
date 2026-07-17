package capabilities

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// OverlayFileOps composes a writable UPPER FileOps over a read-only LOWER
// one — the standard union-mount shape (upper=mem over lower=real is the
// "partially in-memory" unit-test configuration). Semantics:
//
//   - reads / stat / list consult the upper first; a path absent there
//     falls through to the lower unless a WHITEOUT hides it;
//   - every mutation lands in the upper: writing over a lower file
//     copies nothing up front (the new upper entry simply shadows), and
//     metadata mutations (chmod/chtimes/truncate/append-style rewrites)
//     COPY UP the lower file first, then mutate the upper copy;
//   - deleting a path that exists in the lower records a whiteout, so
//     the union stops showing it; deleting an upper-only path is a plain
//     upper delete. Whiteouts are stored in the overlay itself (not the
//     upper), keyed by resolved path;
//   - directory listings merge both layers name-wise (upper entry wins a
//     collision; whiteouts subtract), sorted like every FileOps.ReadDir.
//
// The zero value is not usable; build with NewOverlay. The overlay is a
// full FileOps, so it installs anywhere one does (SetHostFileOps, the
// io module's mount word) and nests (an overlay may itself be a layer).
type OverlayFileOps struct {
	Upper FileOps
	Lower FileOps
	// whiteouts records union-deleted paths (resolved through Upper's
	// ResolvePath, which the overlay uses as its path authority). A
	// whiteout hides the lower entry AND any lower subtree below it.
	whiteouts map[string]bool
}

// NewOverlay builds an overlay of upper (writable) over lower (read-only).
func NewOverlay(upper, lower FileOps) *OverlayFileOps {
	return &OverlayFileOps{Upper: upper, Lower: lower, whiteouts: map[string]bool{}}
}

// deref follows TRAILING symlinks at the UNION level: a symlink in one
// layer may point at an entry in the other, so single-layer resolution is
// not enough. Intermediate-component symlinks that cross layers are an
// accepted overlay limitation (each layer still resolves chains that live
// wholly within it); the practical partially-in-mem test surface needs
// only the trailing hop. Bounded like the kernel (ELOOP).
func (o *OverlayFileOps) deref(path string) (string, error) {
	for depth := 0; ; depth++ {
		if depth > 40 {
			return "", &os.PathError{Op: "open", Path: path, Err: errMemTooManyLinks}
		}
		fi, err := o.Stat(path, false)
		if err != nil || !fi.Symlink {
			return path, nil // absent or concrete: the caller owns the outcome
		}
		t := fi.Target
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(path), t)
		}
		path = filepath.Clean(t)
	}
}

// resolve is the overlay's path authority: the UPPER layer's resolution
// (for a mem upper that is lexical cleaning, matching the differential
// harness's absolute-path convention).
func (o *OverlayFileOps) resolve(path string) string {
	resolved, err := o.Upper.ResolvePath(path)
	if err != nil {
		// Upper resolution can only fail on a relative path with an
		// unreadable cwd; degrade to the lexical form so the whiteout
		// keys stay deterministic.
		return filepath.Clean(path)
	}
	return resolved
}

// hidden reports whether resolved (or an ancestor of it) is whited out.
func (o *OverlayFileOps) hidden(resolved string) bool {
	p := resolved
	for {
		if o.whiteouts[p] {
			return true
		}
		parent := filepath.Dir(p)
		if parent == p || isMemRoot(parent) {
			return false
		}
		p = parent
	}
}

// inUpper reports whether the upper layer knows the path at all (any
// kind — file, dir, symlink).
func (o *OverlayFileOps) inUpper(path string) bool {
	_, err := o.Upper.Stat(path, false)
	return err == nil
}

// copyUp materialises a lower FILE into the upper layer so a mutation can
// apply there. Directories need no copy-up (upper auto-creates parents).
func (o *OverlayFileOps) copyUp(path string) error {
	fi, err := o.Lower.Stat(path, true)
	if err != nil {
		return err
	}
	if fi.IsDir {
		return o.Upper.MkdirAll(path, fi.Mode.Perm())
	}
	data, err := o.Lower.ReadFile(path)
	if err != nil {
		return err
	}
	if err := o.Upper.WriteFile(path, data, fi.Mode.Perm()); err != nil {
		return err
	}
	// Preserve the lower timestamps on the fresh copy.
	return o.Upper.Chtimes(path, fi.ModTime, fi.ModTime)
}

func (o *OverlayFileOps) ReadFile(path string) ([]byte, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, err
	}
	if o.inUpper(path) {
		return o.Upper.ReadFile(path)
	}
	if o.hidden(o.resolve(path)) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return o.Lower.ReadFile(path)
}

func (o *OverlayFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	// A write lands in the upper and unhides the path: the new content
	// shadows whatever the lower held. Writing through a union symlink
	// lands on its (union-resolved) target, like open(2).
	path, err := o.deref(path)
	if err != nil {
		return err
	}
	if err := o.Upper.WriteFile(path, data, perm); err != nil {
		return err
	}
	delete(o.whiteouts, o.resolve(path))
	return nil
}

func (o *OverlayFileOps) MkdirAll(path string, perm os.FileMode) error {
	if err := o.Upper.MkdirAll(path, perm); err != nil {
		return err
	}
	delete(o.whiteouts, o.resolve(path))
	return nil
}

func (o *OverlayFileOps) Stat(path string, follow bool) (FileInfo, error) {
	if follow {
		derefed, err := o.deref(path)
		if err != nil {
			return FileInfo{}, err
		}
		path = derefed
	}
	if o.inUpper(path) {
		return o.Upper.Stat(path, follow)
	}
	if o.hidden(o.resolve(path)) {
		return FileInfo{}, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	return o.Lower.Stat(path, follow)
}

func (o *OverlayFileOps) ReadDir(path string) ([]FileInfo, error) {
	path, err := o.deref(path) // a symlink-to-dir lists its target, like os.ReadDir
	if err != nil {
		return nil, err
	}
	resolved := o.resolve(path)
	upperInfos, upperErr := o.Upper.ReadDir(path)
	var lowerInfos []FileInfo
	var lowerErr error
	if o.hidden(resolved) {
		lowerErr = &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
	} else {
		lowerInfos, lowerErr = o.Lower.ReadDir(path)
	}
	if upperErr != nil && lowerErr != nil {
		// Neither layer lists it — report the upper's error (the layers
		// agree on the not-exist case, the common one).
		return nil, upperErr
	}
	merged := map[string]FileInfo{}
	for _, fi := range lowerInfos {
		if !o.whiteouts[filepath.Join(resolved, fi.Name)] {
			merged[fi.Name] = fi
		}
	}
	for _, fi := range upperInfos {
		merged[fi.Name] = fi // upper shadows a lower name collision
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]FileInfo, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func (o *OverlayFileOps) Remove(path string, recursive bool) error {
	resolved := o.resolve(path)
	upperKnows := o.inUpper(path)
	lowerVisible := false
	if !o.hidden(resolved) {
		if _, err := o.Lower.Stat(path, false); err == nil {
			lowerVisible = true
		}
	}
	if !upperKnows && !lowerVisible {
		if recursive {
			return nil // RemoveAll is idempotent on an absent path
		}
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	// A non-recursive remove of a union directory that still shows lower
	// children must refuse like rmdir(2).
	if !recursive {
		if fi, err := o.Stat(path, false); err == nil && fi.IsDir {
			if infos, rerr := o.ReadDir(path); rerr == nil && len(infos) > 0 {
				return &os.PathError{Op: "remove", Path: path, Err: errMemNotEmpty}
			}
		}
	}
	if upperKnows {
		if err := o.Upper.Remove(path, recursive); err != nil {
			return err
		}
	}
	if lowerVisible {
		o.whiteouts[resolved] = true
	}
	return nil
}

func (o *OverlayFileOps) Rename(oldPath, newPath string) error {
	// Union rename = copy-up the source into the upper (if it only lived
	// below), rename there, and white-out the old lower name.
	if !o.inUpper(oldPath) {
		if o.hidden(o.resolve(oldPath)) {
			return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
		}
		fi, err := o.Lower.Stat(oldPath, false)
		if err != nil {
			return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
		}
		if fi.IsDir {
			if err := o.copyUpTree(oldPath); err != nil {
				return err
			}
		} else if err := o.copyUp(oldPath); err != nil {
			return err
		}
	}
	if err := o.Upper.Rename(oldPath, newPath); err != nil {
		return err
	}
	oldResolved, newResolved := o.resolve(oldPath), o.resolve(newPath)
	if _, err := o.Lower.Stat(oldPath, false); err == nil {
		o.whiteouts[oldResolved] = true
	}
	delete(o.whiteouts, newResolved)
	return nil
}

// copyUpTree materialises a whole lower directory tree into the upper.
func (o *OverlayFileOps) copyUpTree(path string) error {
	fi, err := o.Lower.Stat(path, true)
	if err != nil {
		return err
	}
	if !fi.IsDir {
		return o.copyUp(path)
	}
	if err := o.Upper.MkdirAll(path, fi.Mode.Perm()); err != nil {
		return err
	}
	infos, err := o.Lower.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range infos {
		if o.whiteouts[filepath.Join(o.resolve(path), child.Name)] {
			continue
		}
		if err := o.copyUpTree(filepath.Join(path, child.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (o *OverlayFileOps) Symlink(target, linkPath string) error {
	// Union EEXIST: a visible lower entry blocks the link even though the
	// upper would accept it.
	if _, err := o.Stat(linkPath, false); err == nil {
		return &os.PathError{Op: "symlink", Path: linkPath, Err: os.ErrExist}
	}
	if err := o.Upper.Symlink(target, linkPath); err != nil {
		return err
	}
	delete(o.whiteouts, o.resolve(linkPath))
	return nil
}

func (o *OverlayFileOps) Link(target, linkPath string) error {
	if _, err := o.Stat(linkPath, false); err == nil {
		return &os.PathError{Op: "link", Path: linkPath, Err: os.ErrExist}
	}
	// A hard link needs its target in the writable layer.
	if !o.inUpper(target) {
		if o.hidden(o.resolve(target)) {
			return &os.PathError{Op: "link", Path: target, Err: os.ErrNotExist}
		}
		if err := o.copyUp(target); err != nil {
			return err
		}
	}
	if err := o.Upper.Link(target, linkPath); err != nil {
		return err
	}
	delete(o.whiteouts, o.resolve(linkPath))
	return nil
}

// mutateInUpper union-derefs the path, copies a lower-only entry up, then
// applies fn to the upper at the resolved location.
func (o *OverlayFileOps) mutateInUpper(path, op string, fn func(resolved string) error) error {
	path, err := o.deref(path)
	if err != nil {
		return err
	}
	if !o.inUpper(path) {
		if o.hidden(o.resolve(path)) {
			return &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
		}
		if _, err := o.Lower.Stat(path, false); err != nil {
			return &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
		}
		if err := o.copyUp(path); err != nil {
			return err
		}
	}
	return fn(path)
}

func (o *OverlayFileOps) Chmod(path string, mode os.FileMode) error {
	return o.mutateInUpper(path, "chmod", func(p string) error { return o.Upper.Chmod(p, mode) })
}

func (o *OverlayFileOps) Chtimes(path string, atime, mtime time.Time) error {
	return o.mutateInUpper(path, "chtimes", func(p string) error { return o.Upper.Chtimes(p, atime, mtime) })
}

func (o *OverlayFileOps) Truncate(path string, size int64) error {
	return o.mutateInUpper(path, "truncate", func(p string) error { return o.Upper.Truncate(p, size) })
}

func (o *OverlayFileOps) ResolvePath(path string) (string, error) {
	return o.Upper.ResolvePath(path)
}

// Whiteouts returns a sorted snapshot of the union-deleted paths — the
// observable overlay state a test may assert on.
func (o *OverlayFileOps) Whiteouts() []string {
	out := make([]string, 0, len(o.whiteouts))
	for p := range o.whiteouts {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

var _ FileOps = (*OverlayFileOps)(nil)
