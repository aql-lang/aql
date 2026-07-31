package native

import (
	"errors"
	"io"
	"os"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_mount_handle.go — Open over a mount is EMULATED, not native: a mount
// has no fd model, only whole-value read/write handlers. So a handle
// buffers the whole file in memory and flushes through the bridged
// WriteFile on Sync/Close. DURABILITY CAVEAT: a handle's writes are NOT
// visible to a stateless read until the handle is Synced or Closed (the
// mem/OS backends, which share storage, differ here) — pinned by test.

// Open acquires a buffered mount handle. Create/Truncate materialise the
// (empty) file immediately so a stateless reader sees it; subsequent
// writes accumulate in the buffer until Sync/Close.
func (a *boruFileOps) Open(path string, opts capabilities.OpenOpts) (capabilities.FileHandle, error) {
	existing, rerr := a.ReadFile(path)
	exists := rerr == nil
	switch {
	case exists && opts.Create && opts.Exclusive:
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrExist}
	case !exists && !opts.Create:
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	var buf []byte
	if exists && !opts.Truncate {
		buf = append([]byte(nil), existing...)
	}
	canWrite := opts.Write || opts.Append
	// Materialise an empty file ONLY when creating a NEW one or truncating
	// a writable handle — NOT on every Create of an existing file (that
	// would fire the write handler and rewrite the whole file with no
	// caller write, diverging from the mem/OS backends).
	if (!exists && opts.Create) || (opts.Truncate && canWrite) {
		if err := a.WriteFile(path, buf, 0o644); err != nil {
			return nil, err
		}
	}
	return &mountFileHandle{
		a: a, path: path, buf: buf,
		read:   opts.Read || !canWrite,
		write:  canWrite,
		append: opts.Append,
	}, nil
}

// mountFileHandle is a whole-file buffer over a mount. Reads/writes hit
// the buffer; Sync and Close flush it through the mount's write handler.
type mountFileHandle struct {
	a      *boruFileOps
	path   string
	buf    []byte
	off    int64
	read   bool
	write  bool
	append bool
	dirty  bool
	closed bool
}

func (h *mountFileHandle) Name() string { return h.path }

func (h *mountFileHandle) flush() error {
	if !h.dirty {
		return nil
	}
	if err := h.a.WriteFile(h.path, h.buf, 0o644); err != nil {
		return err
	}
	h.dirty = false
	return nil
}

func (h *mountFileHandle) Sync() error {
	if h.closed {
		return &os.PathError{Op: "sync", Path: h.path, Err: os.ErrClosed}
	}
	return h.flush()
}

func (h *mountFileHandle) Close() error {
	if h.closed {
		return &os.PathError{Op: "close", Path: h.path, Err: os.ErrClosed}
	}
	err := h.flush()
	h.closed = true
	return err
}

func (h *mountFileHandle) Read(p []byte) (int, error) {
	if h.closed {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: os.ErrClosed}
	}
	if !h.read {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleNotReadable}
	}
	if h.off >= int64(len(h.buf)) {
		return 0, io.EOF
	}
	n := copy(p, h.buf[h.off:])
	h.off += int64(n)
	return n, nil
}

func (h *mountFileHandle) ReadAt(p []byte, off int64) (int, error) {
	if h.closed {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: os.ErrClosed}
	}
	if !h.read {
		return 0, &os.PathError{Op: "read", Path: h.path, Err: errHandleNotReadable}
	}
	if off < 0 {
		return 0, &os.PathError{Op: "readat", Path: h.path, Err: os.ErrInvalid}
	}
	if off >= int64(len(h.buf)) {
		return 0, io.EOF
	}
	n := copy(p, h.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *mountFileHandle) Write(p []byte) (int, error) {
	if h.closed {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: os.ErrClosed}
	}
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleNotWritable}
	}
	if h.append {
		h.off = int64(len(h.buf))
	}
	h.buf = spliceAt(h.buf, p, h.off)
	h.off += int64(len(p))
	h.dirty = true
	return len(p), nil
}

func (h *mountFileHandle) WriteAt(p []byte, off int64) (int, error) {
	if h.closed {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: os.ErrClosed}
	}
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.path, Err: errHandleNotWritable}
	}
	if h.append {
		return 0, &os.PathError{Op: "writeat", Path: h.path, Err: errAppendWriteAt}
	}
	if off < 0 {
		return 0, &os.PathError{Op: "writeat", Path: h.path, Err: os.ErrInvalid}
	}
	h.buf = spliceAt(h.buf, p, off)
	h.dirty = true
	return len(p), nil
}

func (h *mountFileHandle) Seek(offset int64, whence int) (int64, error) {
	if h.closed {
		return 0, &os.PathError{Op: "seek", Path: h.path, Err: os.ErrClosed}
	}
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = h.off
	case io.SeekEnd:
		base = int64(len(h.buf))
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

func (h *mountFileHandle) Truncate(size int64) error {
	if h.closed {
		return &os.PathError{Op: "truncate", Path: h.path, Err: os.ErrClosed}
	}
	if !h.write {
		return &os.PathError{Op: "truncate", Path: h.path, Err: errHandleNotWritable}
	}
	if size < 0 {
		return &os.PathError{Op: "truncate", Path: h.path, Err: os.ErrInvalid}
	}
	grown := make([]byte, size)
	copy(grown, h.buf)
	h.buf = grown
	h.dirty = true
	return nil
}

// spliceAt writes src into dst at off, zero-filling any gap.
func spliceAt(dst, src []byte, off int64) []byte {
	end := off + int64(len(src))
	if end > int64(len(dst)) {
		grown := make([]byte, end)
		copy(grown, dst)
		dst = grown
	}
	copy(dst[off:end], src)
	return dst
}

var (
	errHandleNotReadable = errors.New("file handle not opened for reading")
	errHandleNotWritable = errors.New("file handle not opened for writing")
	errAppendWriteAt     = errors.New("invalid use of WriteAt on a file opened for append")
)
