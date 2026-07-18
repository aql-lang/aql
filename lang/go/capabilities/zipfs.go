package capabilities

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// zipfs.go — ZipFileOps presents a ZIP archive as a read-only FileOps
// backend. The whole archive is decompressed into memory at construction
// (documented cost: an N-byte archive holds ~N bytes of entry data), then
// served like any other filesystem: read, stat, list, and a read-only Open
// (buffered so Seek/ReadAt work) all succeed, while every mutation returns
// the read-only sentinel. Directory entries a zip may omit are synthesised
// from the file paths, and separators are normalised to the archive's "/".
//
// It is engaged through the existing `mount` word: `IO.mount p {zip:true}`
// (or a ".zip" path) installs a ZipFileOps; `{writable:true}` layers it as
// the read-only lower of a fresh in-memory overlay, giving a copy-on-write
// "editable zip" whose edits live only in memory. Archive bytes are read
// through the effective FileOps, so a zip staged in the mem backend is
// itself mountable.

// errReadOnlyFS is returned by every ZipFileOps mutation (and by the
// advisory-lock / mmap / watch refusals): a mounted archive cannot change.
// Its message carries "read-only file system" so the differential harness'
// classify() folds it to the "read-only" class.
var errReadOnlyFS = errors.New("read-only file system")

// zipEntry is one archive member (a file or a synthesised directory).
type zipEntry struct {
	name    string // canonical slash path ("." for the root)
	isDir   bool
	data    []byte // decompressed contents; nil for directories
	mode    os.FileMode
	modTime time.Time
}

// ZipFileOps is a read-only FileOps over an in-memory ZIP index.
type ZipFileOps struct {
	entries map[string]*zipEntry
	total   int64 // sum of file sizes, for the synthetic Statfs
}

// NewZip decompresses archive into an in-memory index, synthesising the
// directory entries the archive omits. It errors only when the bytes are
// not a valid zip or a member cannot be decompressed.
func NewZip(archive []byte) (*ZipFileOps, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	z := &ZipFileOps{entries: map[string]*zipEntry{
		".": {name: ".", isDir: true, mode: os.ModeDir | 0o755, modTime: time.Time{}},
	}}
	for _, f := range zr.File {
		name := zipClean(f.Name)
		if name == "." {
			continue // an explicit root entry: the synthesised root already covers it
		}
		z.ensureParents(name)
		if strings.HasSuffix(f.Name, "/") || f.FileInfo().IsDir() {
			z.entries[name] = &zipEntry{name: name, isDir: true, mode: os.ModeDir | f.Mode().Perm(), modTime: f.Modified}
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return nil, oerr
		}
		data, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return nil, rerr
		}
		z.entries[name] = &zipEntry{name: name, data: data, mode: f.Mode().Perm(), modTime: f.Modified}
		z.total += int64(len(data))
	}
	return z, nil
}

// ensureParents synthesises every ancestor directory of name that the
// archive did not list explicitly.
func (z *ZipFileOps) ensureParents(name string) {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if _, ok := z.entries[parent]; ok {
			continue
		}
		z.entries[parent] = &zipEntry{name: parent, isDir: true, mode: os.ModeDir | 0o755, modTime: time.Time{}}
	}
}

// zipClean normalises a query or member path to the archive's canonical
// slash form: separators unified, cleaned, root as ".".
func zipClean(p string) string {
	p = path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// readOnly wraps the read-only sentinel with path/op context.
func readOnly(op, p string) error {
	return &os.PathError{Op: op, Path: p, Err: errReadOnlyFS}
}

// notExist wraps ENOENT with path/op context.
func zipNotExist(op, p string) error {
	return &os.PathError{Op: op, Path: p, Err: os.ErrNotExist}
}

// lookup resolves a query path to its entry.
func (z *ZipFileOps) lookup(p string) (*zipEntry, bool) {
	e, ok := z.entries[zipClean(p)]
	return e, ok
}

func (z *ZipFileOps) infoFor(e *zipEntry) FileInfo {
	return FileInfo{
		Name:    path.Base(e.name),
		Size:    int64(len(e.data)),
		Mode:    e.mode,
		ModTime: e.modTime,
		IsDir:   e.isDir,
		UID:     -1,
		GID:     -1,
	}
}

// --- read operations ---------------------------------------------------

func (z *ZipFileOps) ReadFile(p string) ([]byte, error) {
	e, ok := z.lookup(p)
	if !ok {
		return nil, zipNotExist("open", p)
	}
	if e.isDir {
		return nil, &os.PathError{Op: "read", Path: p, Err: errors.New("is a directory")}
	}
	return append([]byte(nil), e.data...), nil
}

func (z *ZipFileOps) Stat(p string, _ bool) (FileInfo, error) {
	e, ok := z.lookup(p)
	if !ok {
		return FileInfo{}, zipNotExist("stat", p)
	}
	return z.infoFor(e), nil
}

func (z *ZipFileOps) ReadDir(p string) ([]FileInfo, error) {
	dir, ok := z.lookup(p)
	if !ok {
		return nil, zipNotExist("readdir", p)
	}
	if !dir.isDir {
		return nil, &os.PathError{Op: "readdir", Path: p, Err: errors.New("not a directory")}
	}
	canon := zipClean(p)
	var out []FileInfo
	for name, e := range z.entries {
		if name != "." && path.Dir(name) == canon {
			out = append(out, z.infoFor(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (z *ZipFileOps) Statfs(p string) (FsInfo, error) {
	if _, ok := z.lookup(p); !ok {
		return FsInfo{}, zipNotExist("statfs", p)
	}
	return FsInfo{TotalBytes: uint64(z.total), FreeBytes: 0, AvailBytes: 0, BlockSize: 512, Type: "zip"}, nil
}

func (z *ZipFileOps) ResolvePath(p string) (string, error) { return zipClean(p), nil }

// Open returns a read-only, seekable handle over a member's bytes. Any
// write intent (write/append/create/truncate) is refused read-only.
func (z *ZipFileOps) Open(p string, opts OpenOpts) (FileHandle, error) {
	if opts.Write || opts.Append || opts.Create || opts.Truncate {
		return nil, readOnly("open", p)
	}
	e, ok := z.lookup(p)
	if !ok {
		return nil, zipNotExist("open", p)
	}
	if e.isDir {
		return nil, &os.PathError{Op: "open", Path: p, Err: errors.New("is a directory")}
	}
	return &zipFileHandle{name: e.name, r: bytes.NewReader(e.data)}, nil
}

// XattrGet reports no attribute (a zip carries none); XattrList is empty.
func (z *ZipFileOps) XattrGet(p, name string) ([]byte, error) {
	if _, ok := z.lookup(p); !ok {
		return nil, zipNotExist("getxattr", p)
	}
	return nil, &os.PathError{Op: "getxattr", Path: p, Err: errors.New("no data available")}
}

func (z *ZipFileOps) XattrList(p string) ([]string, error) {
	if _, ok := z.lookup(p); !ok {
		return nil, zipNotExist("listxattr", p)
	}
	return nil, nil
}

// --- mutations: all refused read-only ----------------------------------

func (z *ZipFileOps) WriteFile(p string, _ []byte, _ os.FileMode) error { return readOnly("write", p) }
func (z *ZipFileOps) MkdirAll(p string, _ os.FileMode) error            { return readOnly("mkdir", p) }
func (z *ZipFileOps) Remove(p string, _ bool) error                     { return readOnly("remove", p) }
func (z *ZipFileOps) Rename(oldPath, _ string) error                    { return readOnly("rename", oldPath) }
func (z *ZipFileOps) Symlink(_, linkPath string) error                  { return readOnly("symlink", linkPath) }
func (z *ZipFileOps) Link(_, linkPath string) error                     { return readOnly("link", linkPath) }
func (z *ZipFileOps) Chmod(p string, _ os.FileMode) error               { return readOnly("chmod", p) }
func (z *ZipFileOps) Chtimes(p string, _, _ time.Time) error            { return readOnly("chtimes", p) }
func (z *ZipFileOps) Truncate(p string, _ int64) error                  { return readOnly("truncate", p) }
func (z *ZipFileOps) Chown(p string, _, _ int, _ bool) error            { return readOnly("chown", p) }
func (z *ZipFileOps) XattrSet(p, _ string, _ []byte) error              { return readOnly("setxattr", p) }
func (z *ZipFileOps) XattrRemove(p, _ string) error                     { return readOnly("removexattr", p) }
func (z *ZipFileOps) TempFile(dir, _ string) (string, error)            { return "", readOnly("tempfile", dir) }
func (z *ZipFileOps) TempDir(dir, _ string) (string, error)             { return "", readOnly("tempdir", dir) }

// Lock, Mmap, and Watch refuse: an archive cannot be locked or mapped
// writably, and it has no event source.
func (z *ZipFileOps) Lock(p string, _, _ bool) (io.Closer, error) { return nil, readOnly("lock", p) }
func (z *ZipFileOps) Mmap(p string, _ int64, _ int, _ bool) (MmapRegion, error) {
	return nil, readOnly("mmap", p)
}
func (z *ZipFileOps) Watch(p string, _ WatchOpts) (<-chan WatchEvent, func() error, error) {
	return nil, nil, readOnly("watch", p)
}

// --- read-only handle --------------------------------------------------

// zipFileHandle is a read-only FileHandle over a member's bytes: Read /
// Seek / ReadAt come from a bytes.Reader; every write is refused.
type zipFileHandle struct {
	name string
	r    *bytes.Reader
}

func (h *zipFileHandle) Read(p []byte) (int, error)                { return h.r.Read(p) }
func (h *zipFileHandle) Seek(off int64, whence int) (int64, error) { return h.r.Seek(off, whence) }
func (h *zipFileHandle) ReadAt(p []byte, off int64) (int, error)   { return h.r.ReadAt(p, off) }
func (h *zipFileHandle) Write([]byte) (int, error)                 { return 0, readOnly("write", h.name) }
func (h *zipFileHandle) WriteAt([]byte, int64) (int, error)        { return 0, readOnly("writeat", h.name) }
func (h *zipFileHandle) Truncate(int64) error                      { return readOnly("truncate", h.name) }
func (h *zipFileHandle) Sync() error                               { return nil }
func (h *zipFileHandle) Close() error                              { return nil }
func (h *zipFileHandle) Name() string                              { return h.name }

var _ FileOps = (*ZipFileOps)(nil)
var _ FileHandle = (*zipFileHandle)(nil)
