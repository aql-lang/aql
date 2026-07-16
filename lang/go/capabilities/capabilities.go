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
var (
	errMemNotDir       = errors.New("not a directory")
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
	Target string
}

// MemFileOps is an in-memory implementation for testing. It models regular
// files (Files), directories (Dirs), symlinks and metadata (Meta) well enough
// for the full aql:io surface to be exercised hermetically.
type MemFileOps struct {
	Files map[string][]byte
	Dirs  map[string]bool     // tracked directories
	Meta  map[string]*memMeta // per-path mode/mtime + symlink targets
	Cwd   string              // simulated working directory; defaults to "." if empty
}

func NewMem() *MemFileOps {
	return &MemFileOps{
		Files: make(map[string][]byte),
		Dirs:  make(map[string]bool),
		Meta:  make(map[string]*memMeta),
	}
}

func (m *MemFileOps) ReadFile(path string) ([]byte, error) {
	// MemFileOps.ResolvePath never fails (both of its arms return a nil
	// error), so there is no error to propagate here or in WriteFile /
	// MkdirAll — see design/TEST-SEAMS.10.md on provably-dead guards.
	resolved, _ := m.ResolvePath(path)
	data, ok := m.Files[resolved]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return data, nil
}

func (m *MemFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	resolved, _ := m.ResolvePath(path) // never fails — see ReadFile
	m.recordDir(filepath.Dir(resolved))
	buf := make([]byte, len(data))
	copy(buf, data)
	m.Files[resolved] = buf
	m.Meta[resolved] = &memMeta{Mode: perm & os.ModePerm}
	return nil
}

// MkdirAll records a directory path and its parents. Idempotent.
func (m *MemFileOps) MkdirAll(path string, perm os.FileMode) error {
	resolved, _ := m.ResolvePath(path) // never fails — see ReadFile
	m.recordDir(resolved)
	m.Meta[resolved] = &memMeta{Mode: perm & os.ModePerm}
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
			m.Meta[dir] = &memMeta{Mode: 0o755}
		}
		dir = filepath.Dir(dir)
	}
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

// infoFor builds the FileInfo for a resolved path, or ok=false if absent.
func (m *MemFileOps) infoFor(resolved string) (FileInfo, bool) {
	name := filepath.Base(resolved)
	if meta := m.Meta[resolved]; meta != nil && meta.Target != "" {
		return FileInfo{Name: name, Mode: os.ModeSymlink | 0o777, ModTime: meta.MTime, Symlink: true, Target: meta.Target}, true
	}
	if data, ok := m.Files[resolved]; ok {
		meta := m.Meta[resolved]
		return FileInfo{Name: name, Size: int64(len(data)), Mode: modeOf(meta, 0o644), ModTime: mtimeOf(meta)}, true
	}
	if m.Dirs[resolved] {
		meta := m.Meta[resolved]
		return FileInfo{Name: name, Mode: os.ModeDir | (modeOf(meta, 0o755) & os.ModePerm), ModTime: mtimeOf(meta), IsDir: true}, true
	}
	return FileInfo{}, false
}

// Stat returns metadata for path, dereferencing symlinks when follow is true.
func (m *MemFileOps) Stat(path string, follow bool) (FileInfo, error) {
	resolved, _ := m.ResolvePath(path)
	return m.statResolved(resolved, path, follow, 0)
}

func (m *MemFileOps) statResolved(resolved, orig string, follow bool, depth int) (FileInfo, error) {
	info, ok := m.infoFor(resolved)
	if !ok {
		return FileInfo{}, &os.PathError{Op: "stat", Path: orig, Err: os.ErrNotExist}
	}
	if info.Symlink && follow {
		if depth > 40 {
			return FileInfo{}, &os.PathError{Op: "stat", Path: orig, Err: errMemTooManyLinks}
		}
		target := info.Target
		if !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(filepath.Dir(resolved), target))
		}
		return m.statResolved(target, orig, follow, depth+1)
	}
	return info, nil
}

// ReadDir lists the direct children of a directory, name-sorted.
func (m *MemFileOps) ReadDir(path string) ([]FileInfo, error) {
	resolved, _ := m.ResolvePath(path)
	if _, isFile := m.Files[resolved]; isFile {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: errMemNotDir}
	}
	children := map[string]FileInfo{}
	m.collectChildren(resolved, children)
	if len(children) == 0 && !m.Dirs[resolved] && !isMemRoot(resolved) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
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
	for p, meta := range m.Meta {
		if meta.Target != "" {
			add(p)
		}
	}
}

// Remove deletes path (recursive removes a directory tree). Mirrors os:
// RemoveAll (recursive) is idempotent on an absent path; os.Remove is not.
func (m *MemFileOps) Remove(path string, recursive bool) error {
	resolved, _ := m.ResolvePath(path)
	info, ok := m.infoFor(resolved)
	if !ok {
		if recursive {
			return nil
		}
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	if info.IsDir {
		children := map[string]FileInfo{}
		m.collectChildren(resolved, children)
		if len(children) > 0 && !recursive {
			return &os.PathError{Op: "remove", Path: path, Err: errMemNotEmpty}
		}
		m.removeTree(resolved)
		return nil
	}
	delete(m.Files, resolved)
	delete(m.Meta, resolved)
	return nil
}

func (m *MemFileOps) removeTree(root string) {
	prefix := root + string(filepath.Separator)
	for p := range m.Files {
		if p == root || strings.HasPrefix(p, prefix) {
			delete(m.Files, p)
		}
	}
	for p := range m.Dirs {
		if p == root || strings.HasPrefix(p, prefix) {
			delete(m.Dirs, p)
		}
	}
	for p := range m.Meta {
		if p == root || strings.HasPrefix(p, prefix) {
			delete(m.Meta, p)
		}
	}
}

// Rename moves oldPath (and, for a directory, its whole subtree) to newPath.
func (m *MemFileOps) Rename(oldPath, newPath string) error {
	oldResolved, _ := m.ResolvePath(oldPath)
	newResolved, _ := m.ResolvePath(newPath)
	if _, ok := m.infoFor(oldResolved); !ok {
		return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
	}
	m.recordDir(filepath.Dir(newResolved))
	oldPrefix := oldResolved + string(filepath.Separator)
	newPrefix := newResolved + string(filepath.Separator)
	remap := func(p string) (string, bool) {
		if p == oldResolved {
			return newResolved, true
		}
		if strings.HasPrefix(p, oldPrefix) {
			return newPrefix + p[len(oldPrefix):], true
		}
		return "", false
	}
	fileKeys := make([]string, 0, len(m.Files))
	for p := range m.Files {
		fileKeys = append(fileKeys, p)
	}
	for _, p := range fileKeys {
		if np, ok := remap(p); ok {
			m.Files[np] = m.Files[p]
			delete(m.Files, p)
		}
	}
	dirKeys := make([]string, 0, len(m.Dirs))
	for p := range m.Dirs {
		dirKeys = append(dirKeys, p)
	}
	for _, p := range dirKeys {
		if np, ok := remap(p); ok {
			m.Dirs[np] = true
			delete(m.Dirs, p)
		}
	}
	metaKeys := make([]string, 0, len(m.Meta))
	for p := range m.Meta {
		metaKeys = append(metaKeys, p)
	}
	for _, p := range metaKeys {
		if np, ok := remap(p); ok {
			m.Meta[np] = m.Meta[p]
			delete(m.Meta, p)
		}
	}
	return nil
}

// Symlink creates a symbolic link at linkPath pointing at target (stored
// verbatim). Errors if linkPath already exists.
func (m *MemFileOps) Symlink(target, linkPath string) error {
	linkResolved, _ := m.ResolvePath(linkPath)
	if _, ok := m.infoFor(linkResolved); ok {
		return &os.PathError{Op: "symlink", Path: linkPath, Err: os.ErrExist}
	}
	m.recordDir(filepath.Dir(linkResolved))
	m.Meta[linkResolved] = &memMeta{Mode: os.ModeSymlink | 0o777, Target: target}
	return nil
}

// Link creates a hard link at linkPath referring to an existing file target.
func (m *MemFileOps) Link(target, linkPath string) error {
	targetResolved, _ := m.ResolvePath(target)
	data, ok := m.Files[targetResolved]
	if !ok {
		return &os.PathError{Op: "link", Path: target, Err: os.ErrNotExist}
	}
	linkResolved, _ := m.ResolvePath(linkPath)
	if _, exists := m.infoFor(linkResolved); exists {
		return &os.PathError{Op: "link", Path: linkPath, Err: os.ErrExist}
	}
	m.recordDir(filepath.Dir(linkResolved))
	buf := make([]byte, len(data))
	copy(buf, data)
	m.Files[linkResolved] = buf
	m.Meta[linkResolved] = &memMeta{Mode: modeOf(m.Meta[targetResolved], 0o644)}
	return nil
}

// metaForExisting returns the mutable metadata for an existing path.
func (m *MemFileOps) metaForExisting(resolved string) (*memMeta, bool) {
	if _, ok := m.infoFor(resolved); !ok {
		return nil, false
	}
	meta := m.Meta[resolved]
	if meta == nil {
		meta = &memMeta{}
		m.Meta[resolved] = meta
	}
	return meta, true
}

// Chmod sets path's permission bits.
func (m *MemFileOps) Chmod(path string, mode os.FileMode) error {
	resolved, _ := m.ResolvePath(path)
	meta, ok := m.metaForExisting(resolved)
	if !ok {
		return &os.PathError{Op: "chmod", Path: path, Err: os.ErrNotExist}
	}
	meta.Mode = mode & os.ModePerm
	return nil
}

// Chtimes sets path's modification time (access time is not modelled).
func (m *MemFileOps) Chtimes(path string, atime, mtime time.Time) error {
	resolved, _ := m.ResolvePath(path)
	meta, ok := m.metaForExisting(resolved)
	if !ok {
		return &os.PathError{Op: "chtimes", Path: path, Err: os.ErrNotExist}
	}
	meta.MTime = mtime
	return nil
}

// Truncate changes the size of the file at path (grows with zero bytes).
func (m *MemFileOps) Truncate(path string, size int64) error {
	resolved, _ := m.ResolvePath(path)
	data, ok := m.Files[resolved]
	if !ok {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrNotExist}
	}
	if size < 0 {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrInvalid}
	}
	if int64(len(data)) > size {
		m.Files[resolved] = data[:size]
	} else {
		grown := make([]byte, size)
		copy(grown, data)
		m.Files[resolved] = grown
	}
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
