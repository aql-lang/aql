// Package capabilities provides the abstraction for host-side capabilities
// the AQL engine needs to talk to the outside world. At present it covers
// file-system access (the FileOps interface and its OS-backed and
// in-memory implementations); future host capabilities (network,
// process spawn, …) go here too. All dangerous I/O routes through these
// interfaces so it can be replaced for testing or sandboxing without
// touching the Go os/net/etc. packages directly.
package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Clock is the host capability that supplies "the current time" to AQL's
// temporal words (`now`, the aql:time `time-now*` family) and to the
// default seed of aql:rand. The default implementation reads the wall
// clock; a FixedClock can be installed for deterministic tests/specs so
// `now` and time-dependent output are reproducible.
type Clock interface {
	Now() time.Time
}

// WallClock is the default Clock: it returns the real system time.
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }

// FixedClock is a Clock frozen at a single instant — used by the spec
// runner and tests so temporal words produce deterministic results.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }

// StepAction is what a StepController decides to do at a paused step.
type StepAction int

const (
	// StepInto runs the next single step, then pauses again.
	StepInto StepAction = iota
	// StepContinue resumes until the next breakpoint (or completion).
	StepContinue
	// StepQuit abandons the stepped run.
	StepQuit
)

// StepFrame is the rendered state handed to a StepController at a pause.
// It carries only stdlib types so this package stays engine-agnostic — the
// stack is pre-rendered to strings by the caller.
type StepFrame struct {
	Step    int      // 0-based step index
	Pointer int      // index of the value about to execute
	Stack   []string // the tape, rendered, one entry per slot
	AtBreak bool     // true when paused on a Debug.break marker
}

// StepController is the host capability that drives interactive stepping
// (Debug.step). At each pause the engine renders the frame and calls
// OnStep; the returned StepAction decides what happens next. A TTY host
// reads a keypress; a test supplies a scripted list of actions; a
// non-interactive host returns StepContinue to run straight through.
type StepController interface {
	OnStep(frame StepFrame) StepAction
}

// DebugOps is the host capability backing the effectful parts of
// aql:debug that the in-process module cannot supply itself: the
// interactive step controller (and, in future, runtime hooks). Absent a
// DebugOps, Debug.step falls back to printing the trace.
type DebugOps interface {
	Controller() StepController
}

// FileInfo is the host-agnostic result of Stat / a ReadDir entry. It
// carries only stdlib types so the capabilities package stays engine-
// agnostic; the `aql:io` handlers turn it into an AQL FileInfo record.
type FileInfo struct {
	Name    string      // base name (final path segment)
	Size    int64       // length in bytes (0 for directories/symlinks)
	Mode    os.FileMode // permission + type bits
	ModTime time.Time   // last-modified time
	IsDir   bool        // true for directories
	Symlink bool        // true when the entry itself is a symlink (lstat view)
	Target  string      // symlink target, when Symlink
}

// FileOps defines the file operations that AQL's io words use. The
// default implementation delegates to the os package. Replace with a
// custom implementation for testing or sandboxing.
//
// The interface intentionally covers the batch/stateless filesystem
// surface (no open file handles): read/write/append, directory
// create/list, stat, remove, rename, links, and metadata mutation.
// `copy` is deliberately absent — it is composed in the handler layer
// from Stat/ReadFile/WriteFile/ReadDir/MkdirAll. Append is likewise
// composed (read-then-write) in the handler, so it needs no method here.
type FileOps interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	// Stat returns metadata for path. When follow is true a symlink is
	// dereferenced (stat); when false the link itself is described (lstat).
	Stat(path string, follow bool) (FileInfo, error)
	// ReadDir lists the direct children of a directory, name-sorted.
	ReadDir(path string) ([]FileInfo, error)
	// Remove deletes path. When recursive is true a non-empty directory
	// and its contents are removed; otherwise removing a non-empty
	// directory errors.
	Remove(path string, recursive bool) error
	// Rename moves oldPath to newPath.
	Rename(oldPath, newPath string) error
	// Symlink creates a symbolic link at linkPath pointing at target.
	Symlink(target, linkPath string) error
	// Link creates a hard link at linkPath referring to target.
	Link(target, linkPath string) error
	// Chmod sets path's permission bits.
	Chmod(path string, mode os.FileMode) error
	// Chtimes sets path's access and modification times.
	Chtimes(path string, atime, mtime time.Time) error
	// Truncate changes the size of the file at path.
	Truncate(path string, size int64) error
	// Watch subscribes to change events for path — a file, or a
	// directory's direct children (non-recursive, inotify semantics).
	// Returns the event stream and a stop function that releases the
	// watch and closes the stream. Watching an absent path errors.
	// See WatchEvent (watch.go) for the shared op vocabulary.
	Watch(path string) (<-chan WatchEvent, func() error, error)
	ResolvePath(path string) (string, error)
}

// OSFileOps is the default implementation using the real file system.
// The unexported getwd field allows tests to inject a failing os.Getwd.
type OSFileOps struct {
	getwd    func() (string, error)
	mkdirAll func(string, os.FileMode) error
}

func (o *OSFileOps) ReadFile(path string) ([]byte, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (o *OSFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(resolved)
	mkdirFn := o.mkdirAll
	if mkdirFn == nil {
		mkdirFn = os.MkdirAll
	}
	if err := mkdirFn(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, perm)
}

// MkdirAll creates a directory and all parents. Idempotent.
func (o *OSFileOps) MkdirAll(path string, perm os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	mkdirFn := o.mkdirAll
	if mkdirFn == nil {
		mkdirFn = os.MkdirAll
	}
	return mkdirFn(resolved, perm)
}

// ResolvePath resolves a relative path against the process working directory.
func (o *OSFileOps) ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	getwdFn := o.getwd
	if getwdFn == nil {
		getwdFn = os.Getwd
	}
	wd, err := getwdFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, path), nil
}

// osFileInfo projects an os.FileInfo onto the host-agnostic FileInfo.
func osFileInfo(fi os.FileInfo) FileInfo {
	return FileInfo{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
		Symlink: fi.Mode()&os.ModeSymlink != 0,
	}
}

// Stat returns metadata for path. follow=true dereferences a symlink
// (os.Stat); follow=false describes the link itself (os.Lstat) and fills
// Target for a symlink.
func (o *OSFileOps) Stat(path string, follow bool) (FileInfo, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return FileInfo{}, err
	}
	if follow {
		fi, err := os.Stat(resolved)
		if err != nil {
			return FileInfo{}, err
		}
		return osFileInfo(fi), nil
	}
	fi, err := os.Lstat(resolved)
	if err != nil {
		return FileInfo{}, err
	}
	info := osFileInfo(fi)
	if info.Symlink {
		// os.Lstat reported a symlink, so Readlink succeeds barring a
		// filesystem race; on the (unreproducible) race we degrade to an
		// empty Target rather than failing the stat.
		if target, rerr := os.Readlink(resolved); rerr == nil {
			info.Target = target
		}
	}
	return info, nil
}

// ReadDir lists the direct children of a directory, name-sorted (os.ReadDir
// already returns sorted entries).
func (o *OSFileOps) ReadDir(path string) ([]FileInfo, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	infos := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		// DirEntry.Info fails only if the entry vanished between ReadDir
		// and Info (a race); such an entry is simply skipped.
		if fi, ierr := e.Info(); ierr == nil {
			infos = append(infos, osFileInfo(fi))
		}
	}
	return infos, nil
}

// Remove deletes path. recursive=true removes a directory tree (os.RemoveAll);
// recursive=false uses os.Remove, which errors on a non-empty directory.
func (o *OSFileOps) Remove(path string, recursive bool) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	if recursive {
		return os.RemoveAll(resolved)
	}
	return os.Remove(resolved)
}

// Rename moves oldPath to newPath.
func (o *OSFileOps) Rename(oldPath, newPath string) error {
	oldResolved, err := o.ResolvePath(oldPath)
	if err != nil {
		return err
	}
	newResolved, err := o.ResolvePath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldResolved, newResolved)
}

// Symlink creates a symbolic link at linkPath pointing at target. The
// target is stored verbatim (it may be relative to the link's directory).
func (o *OSFileOps) Symlink(target, linkPath string) error {
	linkResolved, err := o.ResolvePath(linkPath)
	if err != nil {
		return err
	}
	return os.Symlink(target, linkResolved)
}

// Link creates a hard link at linkPath referring to target.
func (o *OSFileOps) Link(target, linkPath string) error {
	targetResolved, err := o.ResolvePath(target)
	if err != nil {
		return err
	}
	linkResolved, err := o.ResolvePath(linkPath)
	if err != nil {
		return err
	}
	return os.Link(targetResolved, linkResolved)
}

// Chmod sets path's permission bits.
func (o *OSFileOps) Chmod(path string, mode os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Chmod(resolved, mode)
}

// Chtimes sets path's access and modification times.
func (o *OSFileOps) Chtimes(path string, atime, mtime time.Time) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Chtimes(resolved, atime, mtime)
}

// Truncate changes the size of the file at path.
func (o *OSFileOps) Truncate(path string, size int64) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Truncate(resolved, size)
}

// NewDefault returns the default OS-backed file operations.
func NewDefault() FileOps {
	return &OSFileOps{}
}

// Errors returned by MemFileOps for conditions with no os.Err* constant.
// The messages mirror the kernel errno strings (ENOTDIR / EISDIR /
// ENOTEMPTY / ELOOP) so an error-CLASS comparison against the real
// filesystem — the differential parity harness's classify() — matches.
var (
	errMemNotDir       = errors.New("not a directory")
	errMemIsDir        = errors.New("is a directory")
	errMemNotEmpty     = errors.New("directory not empty")
	errMemTooManyLinks = errors.New("too many levels of symbolic links")
)

// memMeta is the per-path metadata MemFileOps tracks alongside Files/Dirs.
// It is created for every path on write/mkdir/symlink/link, so its presence
// is authoritative for an existing path's mode/mtime (Mode holds permission
// bits only — the type bits are composed at read time). A non-empty Target
// marks the path as a symlink (which then lives ONLY in Meta, not Files/Dirs).
type memMeta struct {
	Mode   os.FileMode
	MTime  time.Time
	ATime  time.Time
	Target string
}

// MemFileOps is an in-memory implementation with REAL-FILESYSTEM fidelity:
// the differential parity harness (differential_test.go) runs identical
// operation sequences against OSFileOps and MemFileOps and asserts equal
// results and error classes. The model covers regular files (Files),
// directories (Dirs), symlinks and metadata (Meta), and hard links (Links —
// alias name → canonical file path, sharing the canonical's bytes and
// metadata exactly like names sharing an inode). Path resolution follows
// symlinks component-wise with an ELOOP budget, an intermediate component
// that is a regular file raises ENOTDIR, and permission bits are enforced
// euid-aware (root bypasses checks, exactly as the kernel does — see
// checkPerm). Documented fidelity exceptions: umask is not applied to
// creation modes, access times are stored but not surfaced (FileInfo has no
// ATime — true of the portable os API too), directory sizes are 0 (real
// values are filesystem-dependent), and unrecorded ancestor directories are
// treated as implicitly writable so tests may poke Files directly.
type MemFileOps struct {
	Files map[string][]byte
	Dirs  map[string]bool     // tracked directories
	Meta  map[string]*memMeta // per-path mode/mtime/atime + symlink targets
	Links map[string]string   // hard-link alias name -> canonical file path
	Cwd   string              // simulated working directory; defaults to "." if empty

	// euidFn / nowFn are test seams for the effective uid (permission
	// enforcement is skipped for root, mirroring the kernel) and the
	// clock (mtime stamping). Nil means os.Geteuid / time.Now.
	euidFn func() int
	nowFn  func() time.Time

	// watchers holds the live Watch subscriptions; every successful
	// mutation fans an event out through emit (watch.go).
	watchers []*memWatcher
}

func NewMem() *MemFileOps {
	return &MemFileOps{
		Files: make(map[string][]byte),
		Dirs:  make(map[string]bool),
		Meta:  make(map[string]*memMeta),
		Links: make(map[string]string),
	}
}

func (m *MemFileOps) euid() int {
	if m.euidFn != nil {
		return m.euidFn()
	}
	return os.Geteuid()
}

func (m *MemFileOps) now() time.Time {
	if m.nowFn != nil {
		return m.nowFn()
	}
	return time.Now()
}

// canon maps a hard-link alias to its canonical file path (identity for
// every non-alias path). Content and metadata live under the canonical
// key, so aliases share them — the inode model.
func (m *MemFileOps) canon(p string) string {
	if c, ok := m.Links[p]; ok {
		return c
	}
	return p
}

// isFileEntry reports whether p names a regular file (canonical or alias).
func (m *MemFileOps) isFileEntry(p string) bool {
	if _, ok := m.Files[p]; ok {
		return true
	}
	_, ok := m.Links[p]
	return ok
}

// modeOf / mtimeOf read a memMeta safely, defaulting when it is absent (a
// path can be poked directly into Files by a test without Meta).
func modeOf(meta *memMeta, def os.FileMode) os.FileMode {
	if meta != nil {
		return meta.Mode
	}
	return def
}

func mtimeOf(meta *memMeta) time.Time {
	if meta != nil {
		return meta.MTime
	}
	return time.Time{}
}

func isMemRoot(p string) bool { return p == "." || p == "/" }

// resolve turns path into its final in-model location: lexical resolution
// (ResolvePath), then a component walk that chases symlinks in every
// intermediate component — and in the trailing component when
// followTrailing is set — with a shared depth budget (ELOOP), erroring
// ENOTDIR when an intermediate component is a regular file. Ops that act
// on a link itself (Remove, Rename, Link's target, lstat) pass
// followTrailing=false; ops with open(2) semantics (read/write/truncate/
// chmod/chtimes/readdir/stat) pass true.
func (m *MemFileOps) resolve(path string, followTrailing bool) (string, error) {
	resolved, _ := m.ResolvePath(path) // MemFileOps.ResolvePath never fails
	depth := 0
	return m.resolveSteps(resolved, followTrailing, &depth, path)
}

func (m *MemFileOps) resolveSteps(resolved string, followTrailing bool, depth *int, orig string) (string, error) {
	if isMemRoot(resolved) || resolved == "" {
		return resolved, nil
	}
	sep := string(filepath.Separator)
	acc := ""
	rest := resolved
	if strings.HasPrefix(resolved, sep) {
		acc, rest = sep, strings.TrimPrefix(resolved, sep)
	}
	comps := strings.Split(rest, sep)
	for i, comp := range comps {
		p := filepath.Join(acc, comp)
		last := i == len(comps)-1
		// Chase symlinks at this component (skipping the trailing one
		// when the op targets the link itself).
		for {
			meta := m.Meta[p]
			if meta == nil || meta.Target == "" || (last && !followTrailing) {
				break
			}
			*depth++
			if *depth > 40 {
				return "", &os.PathError{Op: "open", Path: orig, Err: errMemTooManyLinks}
			}
			t := meta.Target
			if !filepath.IsAbs(t) {
				t = filepath.Join(filepath.Dir(p), t)
			}
			rp, err := m.resolveSteps(filepath.Clean(t), true, depth, orig)
			if err != nil {
				return "", err
			}
			p = rp
		}
		if !last && m.isFileEntry(p) {
			return "", &os.PathError{Op: "open", Path: orig, Err: errMemNotDir}
		}
		acc = p
	}
	return acc, nil
}

// checkPerm enforces an owner permission bit on an existing entry.
// Root (euid 0) bypasses permission checks entirely, exactly as the
// kernel does — so the model agrees with the real filesystem whether the
// process runs privileged or not. Symlinks are always 0777 and absent
// paths are left to the caller's not-exist error.
func (m *MemFileOps) checkPerm(op, path, resolved string, bit os.FileMode) error {
	if m.euid() == 0 {
		return nil
	}
	info, ok := m.infoFor(resolved)
	if !ok || info.Symlink {
		return nil
	}
	if info.Mode&bit == 0 {
		return &os.PathError{Op: op, Path: path, Err: os.ErrPermission}
	}
	return nil
}

// checkParentWrite enforces the write bit on the nearest RECORDED ancestor
// directory for namespace mutations (create/remove/rename/link). Unrecorded
// ancestors (implicit dirs) impose nothing — the model treats them as
// writable so tests may poke Files directly without recording parents.
func (m *MemFileOps) checkParentWrite(op, path, resolved string) error {
	if m.euid() == 0 {
		return nil
	}
	dir := filepath.Dir(resolved)
	for !isMemRoot(dir) && dir != "" {
		if m.Dirs[dir] {
			if modeOf(m.Meta[dir], 0o755)&0o200 == 0 {
				return &os.PathError{Op: op, Path: path, Err: os.ErrPermission}
			}
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// infoFor builds the FileInfo for a resolved path, or ok=false if absent.
// A hard-link alias reads the canonical entry's data/metadata but reports
// its own base name; a symlink's Size is the target length (lstat fidelity).
func (m *MemFileOps) infoFor(resolved string) (FileInfo, bool) {
	name := filepath.Base(resolved)
	if meta := m.Meta[resolved]; meta != nil && meta.Target != "" {
		return FileInfo{Name: name, Size: int64(len(meta.Target)), Mode: os.ModeSymlink | 0o777, ModTime: meta.MTime, Symlink: true, Target: meta.Target}, true
	}
	if c := m.canon(resolved); m.isFileEntry(resolved) {
		data := m.Files[c]
		meta := m.Meta[c]
		return FileInfo{Name: name, Size: int64(len(data)), Mode: modeOf(meta, 0o644), ModTime: mtimeOf(meta)}, true
	}
	if m.Dirs[resolved] {
		meta := m.Meta[resolved]
		return FileInfo{Name: name, Mode: os.ModeDir | (modeOf(meta, 0o755) & os.ModePerm), ModTime: mtimeOf(meta), IsDir: true}, true
	}
	return FileInfo{}, false
}

func (m *MemFileOps) ReadFile(path string) ([]byte, error) {
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if m.Dirs[final] {
		return nil, &os.PathError{Op: "read", Path: path, Err: errMemIsDir}
	}
	data, ok := m.Files[m.canon(final)]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm("open", path, final, 0o400); perr != nil {
		return nil, perr
	}
	return data, nil
}

func (m *MemFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	final, err := m.resolve(path, true) // writes THROUGH a trailing symlink, like open(2)
	if err != nil {
		return err
	}
	if m.Dirs[final] {
		return &os.PathError{Op: "open", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	_, exists := m.Files[c]
	if exists {
		if perr := m.checkPerm("open", path, final, 0o200); perr != nil {
			return perr
		}
	} else if perr := m.checkParentWrite("open", path, final); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(final))
	buf := make([]byte, len(data))
	copy(buf, data)
	m.Files[c] = buf
	if exists {
		// open(2) on an existing file keeps its mode; only mtime moves.
		meta := m.Meta[c]
		if meta == nil {
			meta = &memMeta{Mode: 0o644}
			m.Meta[c] = meta
		}
		meta.MTime = m.now()
		m.emit("write", final)
	} else {
		m.Meta[c] = &memMeta{Mode: perm & os.ModePerm, MTime: m.now(), ATime: m.now()}
		m.emit("create", final)
	}
	return nil
}

// MkdirAll records a directory path and its parents. Idempotent — an
// existing directory keeps its mode, exactly like os.MkdirAll.
func (m *MemFileOps) MkdirAll(path string, perm os.FileMode) error {
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	if m.isFileEntry(final) {
		return &os.PathError{Op: "mkdir", Path: path, Err: errMemNotDir}
	}
	if m.Dirs[final] {
		return nil
	}
	if perr := m.checkParentWrite("mkdir", path, final); perr != nil {
		return perr
	}
	m.recordDir(final)
	m.Meta[final] = &memMeta{Mode: perm & os.ModePerm, MTime: m.now(), ATime: m.now()}
	m.emit("create", final)
	return nil
}

// recordDir records dir and every ancestor as a directory, stamping default
// metadata where none exists. Root ("." / "/") is implicit and not recorded.
func (m *MemFileOps) recordDir(dir string) {
	// filepath.Dir's only POSIX fixed points are "/" and ".", both excluded
	// by the loop condition, so this always terminates.
	for dir != "." && dir != "/" && dir != "" {
		m.Dirs[dir] = true
		if m.Meta[dir] == nil {
			m.Meta[dir] = &memMeta{Mode: 0o755, MTime: m.now()}
		}
		dir = filepath.Dir(dir)
	}
}

// Stat returns metadata for path, dereferencing symlinks when follow is true.
func (m *MemFileOps) Stat(path string, follow bool) (FileInfo, error) {
	final, err := m.resolve(path, follow)
	if err != nil {
		return FileInfo{}, err
	}
	info, ok := m.infoFor(final)
	if !ok {
		return FileInfo{}, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	return info, nil
}

// ReadDir lists the direct children of a directory, name-sorted. A
// symlink-to-directory path is followed, like os.ReadDir.
func (m *MemFileOps) ReadDir(path string) ([]FileInfo, error) {
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if m.isFileEntry(final) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: errMemNotDir}
	}
	children := map[string]FileInfo{}
	m.collectChildren(final, children)
	if len(children) == 0 && !m.Dirs[final] && !isMemRoot(final) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm("readdir", path, final, 0o400); perr != nil {
		return nil, perr
	}
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)
	infos := make([]FileInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, children[name])
	}
	return infos, nil
}

// collectChildren fills out with the immediate children of dir.
func (m *MemFileOps) collectChildren(dir string, out map[string]FileInfo) {
	add := func(p string) {
		if p == dir || filepath.Dir(p) != dir {
			return
		}
		info, _ := m.infoFor(p)
		out[info.Name] = info
	}
	for p := range m.Files {
		add(p)
	}
	for p := range m.Dirs {
		add(p)
	}
	for p := range m.Links {
		add(p)
	}
	for p, meta := range m.Meta {
		if meta.Target != "" {
			add(p)
		}
	}
}

// Remove deletes path (recursive removes a directory tree). Mirrors os:
// RemoveAll (recursive) is idempotent on an absent path; os.Remove is not.
// A symlink is removed itself (no follow); removing one hard-link name
// keeps the shared content alive under the remaining names.
func (m *MemFileOps) Remove(path string, recursive bool) error {
	final, err := m.resolve(path, false)
	if err != nil {
		return err
	}
	info, ok := m.infoFor(final)
	if !ok {
		if recursive {
			return nil
		}
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkParentWrite("remove", path, final); perr != nil {
		return perr
	}
	switch {
	case info.IsDir:
		children := map[string]FileInfo{}
		m.collectChildren(final, children)
		if len(children) > 0 && !recursive {
			return &os.PathError{Op: "remove", Path: path, Err: errMemNotEmpty}
		}
		m.removeTree(final)
		m.emit("remove", final)
	case info.Symlink:
		delete(m.Meta, final)
		m.emit("remove", final)
	default:
		m.removeFileEntry(final)
		m.emit("remove", final)
	}
	return nil
}

// removeFileEntry unlinks ONE name of a regular file. Removing an alias
// deletes just that name; removing the canonical name with aliases
// remaining promotes the first alias (sorted, for determinism) to
// canonical and repoints the rest — the content survives until its last
// name goes, exactly like an inode's link count.
func (m *MemFileOps) removeFileEntry(p string) {
	if _, isAlias := m.Links[p]; isAlias {
		delete(m.Links, p)
		return
	}
	var aliases []string
	for a, c := range m.Links {
		if c == p {
			aliases = append(aliases, a)
		}
	}
	if len(aliases) > 0 {
		sort.Strings(aliases)
		a0 := aliases[0]
		m.Files[a0] = m.Files[p]
		if meta := m.Meta[p]; meta != nil {
			m.Meta[a0] = meta
		}
		delete(m.Links, a0)
		for _, a := range aliases[1:] {
			m.Links[a] = a0
		}
	}
	delete(m.Files, p)
	delete(m.Meta, p)
}

// removeTree removes everything under root. Alias names inside the tree
// are plain unlinks; canonical files inside the tree promote any alias
// OUTSIDE the tree (their content survives under the surviving name).
func (m *MemFileOps) removeTree(root string) {
	prefix := root + string(filepath.Separator)
	within := func(p string) bool { return p == root || strings.HasPrefix(p, prefix) }
	for p := range m.Links {
		if within(p) {
			delete(m.Links, p)
		}
	}
	var files []string
	for p := range m.Files {
		if within(p) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	for _, p := range files {
		m.removeFileEntry(p)
	}
	for p := range m.Dirs {
		if within(p) {
			delete(m.Dirs, p)
		}
	}
	for p := range m.Meta {
		if within(p) {
			delete(m.Meta, p)
		}
	}
}

// Rename moves oldPath (and, for a directory, its whole subtree) to
// newPath, applying the rename(2) destination rules: file over file
// replaces; a directory may only replace an EMPTY directory (ENOTEMPTY
// otherwise); renaming a directory over a file is ENOTDIR and a file over
// a directory EISDIR. Neither trailing path is symlink-followed.
func (m *MemFileOps) Rename(oldPath, newPath string) error {
	oldFinal, err := m.resolve(oldPath, false)
	if err != nil {
		return err
	}
	newFinal, err := m.resolve(newPath, false)
	if err != nil {
		return err
	}
	srcInfo, ok := m.infoFor(oldFinal)
	if !ok {
		return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
	}
	if perr := m.checkParentWrite("rename", oldPath, oldFinal); perr != nil {
		return perr
	}
	if perr := m.checkParentWrite("rename", newPath, newFinal); perr != nil {
		return perr
	}
	if dstInfo, exists := m.infoFor(newFinal); exists {
		switch {
		case srcInfo.IsDir && !dstInfo.IsDir:
			return &os.PathError{Op: "rename", Path: newPath, Err: errMemNotDir}
		case !srcInfo.IsDir && dstInfo.IsDir:
			return &os.PathError{Op: "rename", Path: newPath, Err: errMemIsDir}
		case srcInfo.IsDir && dstInfo.IsDir:
			children := map[string]FileInfo{}
			m.collectChildren(newFinal, children)
			if len(children) > 0 {
				return &os.PathError{Op: "rename", Path: newPath, Err: errMemNotEmpty}
			}
			m.removeTree(newFinal)
		default: // file over file — replace (alias-correct unlink first)
			m.removeFileEntry(newFinal)
		}
	}
	m.recordDir(filepath.Dir(newFinal))
	oldPrefix := oldFinal + string(filepath.Separator)
	newPrefix := newFinal + string(filepath.Separator)
	remap := func(p string) (string, bool) {
		if p == oldFinal {
			return newFinal, true
		}
		if strings.HasPrefix(p, oldPrefix) {
			return newPrefix + p[len(oldPrefix):], true
		}
		return "", false
	}
	remapMapKeys := func(keys []string, move func(oldKey, newKey string)) {
		for _, p := range keys {
			if np, ok := remap(p); ok {
				move(p, np)
			}
		}
	}
	fileKeys := make([]string, 0, len(m.Files))
	for p := range m.Files {
		fileKeys = append(fileKeys, p)
	}
	remapMapKeys(fileKeys, func(o, n string) { m.Files[n] = m.Files[o]; delete(m.Files, o) })
	dirKeys := make([]string, 0, len(m.Dirs))
	for p := range m.Dirs {
		dirKeys = append(dirKeys, p)
	}
	remapMapKeys(dirKeys, func(o, n string) { m.Dirs[n] = true; delete(m.Dirs, o) })
	metaKeys := make([]string, 0, len(m.Meta))
	for p := range m.Meta {
		metaKeys = append(metaKeys, p)
	}
	remapMapKeys(metaKeys, func(o, n string) { m.Meta[n] = m.Meta[o]; delete(m.Meta, o) })
	linkKeys := make([]string, 0, len(m.Links))
	for p := range m.Links {
		linkKeys = append(linkKeys, p)
	}
	remapMapKeys(linkKeys, func(o, n string) { m.Links[n] = m.Links[o]; delete(m.Links, o) })
	// Aliases pointing INTO the moved tree follow their canonical.
	for a, c := range m.Links {
		if nc, ok := remap(c); ok {
			m.Links[a] = nc
		}
	}
	m.emit("rename", oldFinal)
	m.emit("create", newFinal)
	return nil
}

// Symlink creates a symbolic link at linkPath pointing at target (stored
// verbatim). Errors if linkPath already exists.
func (m *MemFileOps) Symlink(target, linkPath string) error {
	linkFinal, err := m.resolve(linkPath, false)
	if err != nil {
		return err
	}
	if _, ok := m.infoFor(linkFinal); ok {
		return &os.PathError{Op: "symlink", Path: linkPath, Err: os.ErrExist}
	}
	if perr := m.checkParentWrite("symlink", linkPath, linkFinal); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(linkFinal))
	m.Meta[linkFinal] = &memMeta{Mode: os.ModeSymlink | 0o777, MTime: m.now(), Target: target}
	m.emit("create", linkFinal)
	return nil
}

// Link creates a hard link at linkPath referring to target. Like link(2),
// the target is NOT symlink-followed: hard-linking a symlink clones the
// link entry, and hard-linking a directory is refused (EPERM). A file
// link records an alias to the canonical path, sharing bytes and metadata.
func (m *MemFileOps) Link(target, linkPath string) error {
	targetFinal, err := m.resolve(target, false)
	if err != nil {
		return err
	}
	linkFinal, err := m.resolve(linkPath, false)
	if err != nil {
		return err
	}
	info, ok := m.infoFor(targetFinal)
	if !ok {
		return &os.PathError{Op: "link", Path: target, Err: os.ErrNotExist}
	}
	if _, exists := m.infoFor(linkFinal); exists {
		return &os.PathError{Op: "link", Path: linkPath, Err: os.ErrExist}
	}
	if perr := m.checkParentWrite("link", linkPath, linkFinal); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(linkFinal))
	switch {
	case info.IsDir:
		return &os.PathError{Op: "link", Path: target, Err: os.ErrPermission}
	case info.Symlink:
		src := m.Meta[targetFinal]
		m.Meta[linkFinal] = &memMeta{Mode: src.Mode, MTime: src.MTime, ATime: src.ATime, Target: src.Target}
	default:
		m.Links[linkFinal] = m.canon(targetFinal)
	}
	m.emit("create", linkFinal)
	return nil
}

// metaForExisting returns the mutable metadata for an existing path
// (through its canonical name, so hard links share it).
func (m *MemFileOps) metaForExisting(resolved string) (*memMeta, bool) {
	if _, ok := m.infoFor(resolved); !ok {
		return nil, false
	}
	c := m.canon(resolved)
	meta := m.Meta[c]
	if meta == nil {
		meta = &memMeta{}
		m.Meta[c] = meta
	}
	return meta, true
}

// Chmod sets path's permission bits (symlink-followed, like chmod(2)).
func (m *MemFileOps) Chmod(path string, mode os.FileMode) error {
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return &os.PathError{Op: "chmod", Path: path, Err: os.ErrNotExist}
	}
	meta.Mode = mode & os.ModePerm
	m.emit("chmod", final)
	return nil
}

// Chtimes sets path's access and modification times (symlink-followed).
func (m *MemFileOps) Chtimes(path string, atime, mtime time.Time) error {
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return &os.PathError{Op: "chtimes", Path: path, Err: os.ErrNotExist}
	}
	meta.MTime = mtime
	meta.ATime = atime
	m.emit("chmod", final)
	return nil
}

// Truncate changes the size of the file at path (grows with zero bytes).
func (m *MemFileOps) Truncate(path string, size int64) error {
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	if m.Dirs[final] {
		return &os.PathError{Op: "truncate", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	data, ok := m.Files[c]
	if !ok {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrNotExist}
	}
	if size < 0 {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrInvalid}
	}
	if perr := m.checkPerm("truncate", path, final, 0o200); perr != nil {
		return perr
	}
	if int64(len(data)) > size {
		m.Files[c] = data[:size]
	} else {
		grown := make([]byte, size)
		copy(grown, data)
		m.Files[c] = grown
	}
	if meta, ok := m.metaForExisting(final); ok {
		meta.MTime = m.now()
	}
	m.emit("write", final)
	return nil
}

func (m *MemFileOps) ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	base := m.Cwd
	if base == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, path)), nil
}
