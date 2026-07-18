package capabilities

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// handles.go — the stateful file-handle capability (FileOps.Open). The OS
// backend returns *os.File verbatim (it already satisfies FileHandle);
// the mem backend returns a memFileHandle that re-indexes the Files map
// per op so a handle's writes are visible to a stateless read before
// Close (page-cache fidelity). Both derive their open flags from
// openFlag, the single OpenOpts → os.OpenFile-flag mapping.

// openFlag maps OpenOpts onto the os.OpenFile flag set. Read-only is the
// default; Read+Write is O_RDWR; Write alone is O_WRONLY.
func openFlag(opts OpenOpts) int {
	var flag int
	switch {
	case opts.Read && opts.Write:
		flag = os.O_RDWR
	case opts.Write:
		flag = os.O_WRONLY
	default:
		flag = os.O_RDONLY
	}
	if opts.Append {
		flag |= os.O_APPEND
	}
	if opts.Create {
		flag |= os.O_CREATE
	}
	if opts.Exclusive {
		flag |= os.O_EXCL
	}
	// O_TRUNC only with a write intent — truncating a read-only open is a
	// contradiction, and the mem backend gates truncation the same way, so
	// {Read, Truncate} must not clear the file on either backend.
	if opts.Truncate && (opts.Write || opts.Append) {
		flag |= os.O_TRUNC
	}
	return flag
}

// openPerm is the creation mode for Create opens; 0 defaults to 0644.
func openPerm(opts OpenOpts) os.FileMode {
	if opts.Perm == 0 {
		return 0o644
	}
	return opts.Perm
}

// Open acquires a real *os.File handle (which already satisfies
// FileHandle) with flags derived from OpenOpts.
func (o *OSFileOps) Open(path string, opts OpenOpts) (FileHandle, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(resolved, openFlag(opts), openPerm(opts))
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Open acquires a mem file handle per OpenOpts, mirroring open(2):
// Create makes an absent file (Exclusive then fails if it already
// exists), Truncate clears an existing one, and the read/write bits are
// permission-checked against the file's mode. The returned handle shares
// the Files map with stateless reads (page-cache visibility).
func (m *MemFileOps) Open(path string, opts OpenOpts) (FileHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true) // open THROUGH a trailing symlink, like open(2)
	if err != nil {
		return nil, err
	}
	if m.Dirs[final] {
		return nil, &os.PathError{Op: "open", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	_, exists := m.Files[c]
	canRead := opts.Read || (!opts.Write && !opts.Append)
	canWrite := opts.Write || opts.Append
	switch {
	case exists && opts.Create && opts.Exclusive:
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrExist}
	case !exists && !opts.Create:
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	case !exists:
		if perr := m.checkParentWrite("open", path, final); perr != nil {
			return nil, perr
		}
		m.recordDir(filepath.Dir(final))
		m.Files[c] = []byte{}
		m.Meta[c] = m.stampOwner(&memMeta{Mode: openPerm(opts) & os.ModePerm, MTime: m.now(), ATime: m.now()})
		m.emit("create", final)
	default:
		if canRead {
			if perr := m.checkPerm("open", path, final, 0o400); perr != nil {
				return nil, perr
			}
		}
		if canWrite {
			if perr := m.checkPerm("open", path, final, 0o200); perr != nil {
				return nil, perr
			}
		}
		if opts.Truncate && canWrite {
			m.Files[c] = []byte{}
			if meta := m.Meta[c]; meta != nil {
				meta.MTime = m.now()
			}
			m.emit("write", final)
		}
	}
	return &memFileHandle{
		m: m, canon: c, final: final, path: path,
		read: canRead, write: canWrite, append: opts.Append,
	}, nil
}

// memFileHandle is a cursor over a mem file. It holds only the canonical
// key, an offset, and the open mode — every op re-indexes m.Files[canon]
// under m's lock, so a handle's writes are immediately visible to a
// stateless ReadFile before Close (the page-cache model). ReadAt/WriteAt
// are positioned and leave the cursor untouched.
type memFileHandle struct {
	m      *MemFileOps
	canon  string // shared-inode Files/Meta key
	final  string // resolved path (what watchers watch)
	path   string // original path (Name())
	off    int64
	read   bool
	write  bool
	append bool
	closed bool
}

// errHandleClosed / errHandleMode are the handle-state refusals (mirrored
// on the OS side by os.File's own ErrClosed / bad-fd errors).
var (
	errHandleClosed = os.ErrClosed
	errHandleRead   = errors.New("file handle not opened for reading")
	errHandleWrite  = errors.New("file handle not opened for writing")
	// errAppendWriteAt mirrors os.File.WriteAt's refusal of a positioned
	// write on an O_APPEND handle (os: invalid use of WriteAt …).
	errAppendWriteAt = errors.New("invalid use of WriteAt on a file opened for append")
)

func (h *memFileHandle) Name() string { return h.path }

func (h *memFileHandle) Sync() error {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return &os.PathError{Op: "sync", Path: h.path, Err: errHandleClosed}
	}
	return nil // mem is always durable
}

func (h *memFileHandle) Close() error {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return &os.PathError{Op: "close", Path: h.path, Err: errHandleClosed}
	}
	h.closed = true
	return nil
}

// data returns the current bytes of the handle's file (nil if the file
// was removed out from under the handle). Caller holds m.mu.
func (h *memFileHandle) data() []byte { return h.m.Files[h.canon] }

// store writes b back as the file's contents and stamps mtime. Caller
// holds m.mu.
func (h *memFileHandle) store(b []byte) {
	h.m.Files[h.canon] = b
	if meta, ok := h.m.Meta[h.canon]; ok {
		meta.MTime = h.m.now()
	}
	h.m.emit("write", h.final)
}

func (h *memFileHandle) Read(p []byte) (int, error) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleClosed}
	}
	if !h.read {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleRead}
	}
	data := h.data()
	if h.off >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[h.off:])
	h.off += int64(n)
	return n, nil
}

func (h *memFileHandle) ReadAt(p []byte, off int64) (int, error) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleClosed}
	}
	if !h.read {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleRead}
	}
	if off < 0 {
		return 0, &os.PathError{Op: "readat", Path: h.path, Err: os.ErrInvalid}
	}
	data := h.data()
	if off >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// writeInto splices src into dst at off, zero-filling any gap, and
// returns the new slice.
func writeInto(dst, src []byte, off int64) []byte {
	end := off + int64(len(src))
	if end > int64(len(dst)) {
		grown := make([]byte, end)
		copy(grown, dst)
		dst = grown
	}
	copy(dst[off:end], src)
	return dst
}

func (h *memFileHandle) Write(p []byte) (int, error) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleClosed}
	}
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleWrite}
	}
	data := h.data()
	if h.append {
		h.off = int64(len(data))
	}
	h.store(writeInto(data, p, h.off))
	h.off += int64(len(p))
	return len(p), nil
}

func (h *memFileHandle) WriteAt(p []byte, off int64) (int, error) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleClosed}
	}
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleWrite}
	}
	if h.append {
		return 0, &os.PathError{Op: "writeat", Path: h.path, Err: errAppendWriteAt}
	}
	if off < 0 {
		return 0, &os.PathError{Op: "writeat", Path: h.path, Err: os.ErrInvalid}
	}
	h.store(writeInto(h.data(), p, off))
	return len(p), nil
}

func (h *memFileHandle) Seek(offset int64, whence int) (int64, error) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return 0, &os.PathError{Op: "seek", Path: h.path, Err: errHandleClosed}
	}
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = h.off
	case io.SeekEnd:
		base = int64(len(h.data()))
	default:
		return 0, &os.PathError{Op: "seek", Path: h.path, Err: os.ErrInvalid}
	}
	next := base + offset
	if next < 0 {
		return 0, &os.PathError{Op: "seek", Path: h.path, Err: os.ErrInvalid}
	}
	h.off = next
	return next, nil
}

func (h *memFileHandle) Truncate(size int64) error {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if h.closed {
		return &os.PathError{Op: "truncate", Path: h.path, Err: errHandleClosed}
	}
	if !h.write {
		return &os.PathError{Op: "truncate", Path: h.path, Err: errHandleWrite}
	}
	if size < 0 {
		return &os.PathError{Op: "truncate", Path: h.path, Err: os.ErrInvalid}
	}
	grown := make([]byte, size)
	copy(grown, h.data()) // copies min(len(data), size); shrink truncates, grow zero-fills
	h.store(grown)
	return nil
}
