package native

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/capabilities"
)

// io_mount.go — the AQL→FileOps bridge: `IO.mount {read: fn, write: fn,
// …}` installs a filesystem IMPLEMENTED IN AQL as the host FileOps, so
// every io word (and any other consumer of the capability) routes its
// operations through user-supplied AQL handlers. This is how AQL code
// exposes an arbitrary backing — a Store, a database table, a remote
// service — as a filesystem. `IO.unmount` restores the previous FileOps.
//
// The handler map's keys are operation names; each value is a Function.
// Core operations (the minimal viable filesystem):
//
//	read   ([p:Pathon]        → String|Bytes|none)   none ⇒ not-exist
//	write  ([p:Pathon data]   → any)                 data is String (or Bytes when not valid UTF-8)
//	stat   ([p:Pathon]        → map|none)            {name size mode mtime type target} shape; none ⇒ not-exist
//	list   ([p:Pathon]        → list)                of name Strings, or of stat-shaped maps
//	remove ([p:Pathon opts]   → any)                 opts = {recursive: Boolean}
//
// Optional operations: mkdir ([p]), rename ([old new]), symlink /
// link ([target linkpath]), chmod ([p mode:Integer]), chtimes ([p
// atime:Integer mtime:Integer]), truncate ([p size:Integer]), resolve
// ([p] → String). An operation with no handler returns a clean
// mount-unsupported error — the documented-no-op posture for exotic ops.
// Watch is not bridgeable (no event source in AQL) and always refuses.
//
// Precedence: the __sys.fs {mem}/{overlay} context toggles still win —
// EffectiveFileOps consults them before the host slot — so tests can
// always escape a mount hermetically.

// errMountUnsupported marks an operation the mounted handler map does
// not implement.
var errMountUnsupported = errors.New("operation not supported by the mounted filesystem")

// aqlFileOps adapts a map of AQL Function handlers to the FileOps
// interface. Handlers run through CallAQL on the mounting registry, on
// the calling goroutine — the same execution shape as any fn value
// invoked by a native word.
type aqlFileOps struct {
	r        *Registry
	handlers *OrderedMap
	// tempSeq numbers the temp-emulation tokens (deterministic, like mem).
	tempSeq int
}

// call dispatches one operation to its handler, returning the LAST value
// of the handler's result (the conventional fn return).
func (a *aqlFileOps) call(op string, args []Value) (Value, error) {
	fnVal, ok := a.handlers.Get(op)
	if !ok {
		return Value{}, &os.PathError{Op: op, Path: pathOfArgs(args), Err: errMountUnsupported}
	}
	sig := MatchFnSig(fnVal, args)
	if sig == nil {
		return Value{}, &os.PathError{Op: op, Path: pathOfArgs(args),
			Err: fmt.Errorf("mounted %s handler has no signature matching %d argument(s)", op, len(args))}
	}
	fnDef, _ := fnVal.Data.(FnDefInfo)
	res, err := a.r.CallAQL(sig, args, fnDef.Captured)
	if err != nil {
		return Value{}, err
	}
	if len(res) == 0 {
		return NewTypeLiteral(TNone), nil
	}
	return res[len(res)-1], nil
}

// pathOfArgs renders the first argument (the Pathon) for error paths.
func pathOfArgs(args []Value) string {
	if len(args) == 0 {
		return ""
	}
	return args[0].String()
}

// isNoneResult reports the handler's "absent" convention: any none value
// (the AQL none literal is a concrete NonePayload; a handler returning no
// value at all yields the None type literal — both mean absent).
func isNoneResult(v Value) bool { return v.Is(TNone) }

func (a *aqlFileOps) ReadFile(path string) ([]byte, error) {
	res, err := a.call("read", []Value{NewPathonFromString(path)})
	if err != nil {
		return nil, err
	}
	if isNoneResult(res) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	if b, ok := AsBytesValue(res); ok {
		return b, nil
	}
	s, err := res.AsConcreteString()
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path,
			Err: fmt.Errorf("mounted read handler returned %s (want String, Bytes, or none)", res.Parent)}
	}
	return []byte(s), nil
}

func (a *aqlFileOps) WriteFile(path string, data []byte, _ os.FileMode) error {
	var payload Value
	if utf8.Valid(data) {
		payload = NewString(string(data))
	} else {
		payload = NewBytesValue(data)
	}
	_, err := a.call("write", []Value{NewPathonFromString(path), payload})
	return err
}

func (a *aqlFileOps) MkdirAll(path string, _ os.FileMode) error {
	_, err := a.call("mkdir", []Value{NewPathonFromString(path)})
	return err
}

// statFromMap projects a handler's stat-shaped map onto FileInfo.
// Ownership defaults to the -1 "cannot know" sentinel; a handler may
// supply integer `owner` / `group` fields to report it.
func statFromMap(path string, m ReadMap) capabilities.FileInfo {
	fi := capabilities.FileInfo{Name: filepath.Base(path), UID: -1, GID: -1}
	if v, ok := m.Get("owner"); ok {
		if n, nerr := v.AsConcreteInteger(); nerr == nil {
			fi.UID = int(n)
		}
	}
	if v, ok := m.Get("group"); ok {
		if n, nerr := v.AsConcreteInteger(); nerr == nil {
			fi.GID = int(n)
		}
	}
	if v, ok := m.Get("name"); ok {
		if s, serr := v.AsConcreteString(); serr == nil {
			fi.Name = s
		}
	}
	if v, ok := m.Get("size"); ok {
		if n, nerr := v.AsConcreteInteger(); nerr == nil {
			fi.Size = n
		}
	}
	if v, ok := m.Get("mode"); ok {
		if n, nerr := v.AsConcreteInteger(); nerr == nil {
			fi.Mode = os.FileMode(n) & os.ModePerm
		}
	}
	if v, ok := m.Get("mtime"); ok {
		if n, nerr := v.AsConcreteInteger(); nerr == nil {
			fi.ModTime = time.Unix(n, 0)
		}
	}
	if v, ok := m.Get("type"); ok {
		switch kind, _ := v.AsConcreteAtom(); kind {
		case "dir":
			fi.IsDir = true
			fi.Mode |= os.ModeDir
		case "symlink":
			fi.Symlink = true
			fi.Mode |= os.ModeSymlink
		}
	}
	if v, ok := m.Get("target"); ok {
		if s, serr := v.AsConcreteString(); serr == nil {
			fi.Target = s
		}
	}
	return fi
}

func (a *aqlFileOps) Stat(path string, _ bool) (capabilities.FileInfo, error) {
	res, err := a.call("stat", []Value{NewPathonFromString(path)})
	if err != nil {
		return capabilities.FileInfo{}, err
	}
	if isNoneResult(res) {
		return capabilities.FileInfo{}, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	m, merr := RequireConcreteMap(res, "stat")
	if merr != nil {
		return capabilities.FileInfo{}, &os.PathError{Op: "stat", Path: path,
			Err: fmt.Errorf("mounted stat handler returned %s (want a map or none)", res.Parent)}
	}
	return statFromMap(path, m), nil
}

func (a *aqlFileOps) ReadDir(path string) ([]capabilities.FileInfo, error) {
	res, err := a.call("list", []Value{NewPathonFromString(path)})
	if err != nil {
		return nil, err
	}
	lst, lerr := RequireConcreteList(res, "list")
	if lerr != nil {
		return nil, &os.PathError{Op: "readdir", Path: path,
			Err: fmt.Errorf("mounted list handler returned %s (want a list)", res.Parent)}
	}
	out := make([]capabilities.FileInfo, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		entry := lst.Get(i)
		if s, serr := entry.AsConcreteString(); serr == nil {
			out = append(out, capabilities.FileInfo{Name: s, UID: -1, GID: -1})
			continue
		}
		if m, merr := RequireConcreteMap(entry, "list"); merr == nil {
			out = append(out, statFromMap(path, m))
			continue
		}
		return nil, &os.PathError{Op: "readdir", Path: path,
			Err: fmt.Errorf("mounted list entry %d is %s (want a name String or a stat map)", i, entry.Parent)}
	}
	return out, nil
}

func (a *aqlFileOps) Remove(path string, recursive bool) error {
	opts := NewOrderedMap()
	opts.Set("recursive", NewBoolean(recursive))
	_, err := a.call("remove", []Value{NewPathonFromString(path), NewMap(opts)})
	return err
}

func (a *aqlFileOps) Rename(oldPath, newPath string) error {
	_, err := a.call("rename", []Value{NewPathonFromString(oldPath), NewPathonFromString(newPath)})
	return err
}

func (a *aqlFileOps) Symlink(target, linkPath string) error {
	_, err := a.call("symlink", []Value{NewPathonFromString(target), NewPathonFromString(linkPath)})
	return err
}

func (a *aqlFileOps) Link(target, linkPath string) error {
	_, err := a.call("link", []Value{NewPathonFromString(target), NewPathonFromString(linkPath)})
	return err
}

func (a *aqlFileOps) Chmod(path string, mode os.FileMode) error {
	_, err := a.call("chmod", []Value{NewPathonFromString(path), NewInteger(int64(mode & os.ModePerm))})
	return err
}

func (a *aqlFileOps) Chtimes(path string, atime, mtime time.Time) error {
	_, err := a.call("chtimes", []Value{NewPathonFromString(path), NewInteger(atime.Unix()), NewInteger(mtime.Unix())})
	return err
}

func (a *aqlFileOps) Truncate(path string, size int64) error {
	_, err := a.call("truncate", []Value{NewPathonFromString(path), NewInteger(size)})
	return err
}

// Chown bridges to an optional `chown ([p uid gid])` handler (the follow
// flag is not forwarded — a mounted filesystem models no symlink-deref
// distinction unless its own handlers do). Absent handler: clean refusal.
func (a *aqlFileOps) Chown(path string, uid, gid int, _ bool) error {
	_, err := a.call("chown", []Value{NewPathonFromString(path), NewInteger(int64(uid)), NewInteger(int64(gid))})
	return err
}

// XattrGet bridges to an optional `xattr-get ([p name])` handler; a none
// result is the absent-attribute convention.
func (a *aqlFileOps) XattrGet(path, name string) ([]byte, error) {
	res, err := a.call("xattr-get", []Value{NewPathonFromString(path), NewString(name)})
	if err != nil {
		return nil, err
	}
	if isNoneResult(res) {
		return nil, &os.PathError{Op: "xattr-get", Path: path, Err: errors.New("attribute not found")}
	}
	if b, ok := AsBytesValue(res); ok {
		return b, nil
	}
	s, serr := res.AsConcreteString()
	if serr != nil {
		return nil, &os.PathError{Op: "xattr-get", Path: path,
			Err: fmt.Errorf("mounted xattr-get handler returned %s (want String, Bytes, or none)", res.Parent)}
	}
	return []byte(s), nil
}

// XattrSet bridges to an optional `xattr-set ([p name value])` handler
// (the value arrives as a String when valid UTF-8, Bytes otherwise —
// the same convention as the bridged write payload).
func (a *aqlFileOps) XattrSet(path, name string, value []byte) error {
	var payload Value
	if utf8.Valid(value) {
		payload = NewString(string(value))
	} else {
		payload = NewBytesValue(value)
	}
	_, err := a.call("xattr-set", []Value{NewPathonFromString(path), NewString(name), payload})
	return err
}

// XattrList bridges to an optional `xattr-list ([p])` handler returning a
// list of name strings (sorted here so backends need not bother).
func (a *aqlFileOps) XattrList(path string) ([]string, error) {
	res, err := a.call("xattr-list", []Value{NewPathonFromString(path)})
	if err != nil {
		return nil, err
	}
	lst, lerr := RequireConcreteList(res, "xattr-list")
	if lerr != nil {
		return nil, &os.PathError{Op: "xattr-list", Path: path,
			Err: fmt.Errorf("mounted xattr-list handler returned %s (want a list of names)", res.Parent)}
	}
	names := make([]string, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		if s, serr := lst.Get(i).AsConcreteString(); serr == nil {
			names = append(names, s)
		}
	}
	sort.Strings(names)
	return names, nil
}

// XattrRemove bridges to an optional `xattr-remove ([p name])` handler.
func (a *aqlFileOps) XattrRemove(path, name string) error {
	_, err := a.call("xattr-remove", []Value{NewPathonFromString(path), NewString(name)})
	return err
}

// TempFile bridges to an optional `temp ([dir pattern] → Pathon|String)`
// handler; absent that, a DEFAULT EMULATION generates a counter name and
// routes through the bridged write — mount's minimum contract already
// covers file creation, so temp needs no mandatory handler.
func (a *aqlFileOps) TempFile(dir, pattern string) (string, error) {
	if _, ok := a.handlers.Get("temp"); ok {
		res, err := a.call("temp", []Value{NewPathonFromString(dir), NewString(pattern)})
		if err != nil {
			return "", err
		}
		return extractPath(res), nil
	}
	path := a.tempEmulatedName(dir, pattern)
	if err := a.WriteFile(path, []byte{}, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// TempDir is the directory twin: the optional handler, else the counter
// name routed through the bridged mkdir.
func (a *aqlFileOps) TempDir(dir, pattern string) (string, error) {
	if _, ok := a.handlers.Get("temp"); ok {
		res, err := a.call("temp", []Value{NewPathonFromString(dir), NewString(pattern), NewBoolean(true)})
		if err != nil {
			return "", err
		}
		return extractPath(res), nil
	}
	path := a.tempEmulatedName(dir, pattern)
	if err := a.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// tempEmulatedName builds the emulation's unique name (counter token per
// the os.CreateTemp pattern contract; "" dir → "/tmp").
func (a *aqlFileOps) tempEmulatedName(dir, pattern string) string {
	if dir == "" {
		dir = "/tmp"
	}
	a.tempSeq++
	token := fmt.Sprintf("%d", a.tempSeq)
	name := pattern + token
	if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
		name = pattern[:i] + token + pattern[i+1:]
	}
	return filepath.Join(dir, name)
}

// Statfs bridges to an optional `statfs ([p] → {total free available
// bsize type}|none)` handler. none (or an absent handler) refuses.
func (a *aqlFileOps) Statfs(path string) (capabilities.FsInfo, error) {
	res, err := a.call("statfs", []Value{NewPathonFromString(path)})
	if err != nil {
		return capabilities.FsInfo{}, err
	}
	if isNoneResult(res) {
		return capabilities.FsInfo{}, &os.PathError{Op: "statfs", Path: path, Err: os.ErrNotExist}
	}
	m, merr := RequireConcreteMap(res, "statfs")
	if merr != nil {
		return capabilities.FsInfo{}, &os.PathError{Op: "statfs", Path: path,
			Err: fmt.Errorf("mounted statfs handler returned %s (want a map or none)", res.Parent)}
	}
	fs := capabilities.FsInfo{Type: "mount"}
	if n, ok := MapFieldInteger(m, "total"); ok {
		fs.TotalBytes = uint64(n)
	}
	if n, ok := MapFieldInteger(m, "free"); ok {
		fs.FreeBytes = uint64(n)
	}
	if n, ok := MapFieldInteger(m, "available"); ok {
		fs.AvailBytes = uint64(n)
	}
	if n, ok := MapFieldInteger(m, "bsize"); ok {
		fs.BlockSize = n
	}
	if s, ok := MapFieldString(m, "type"); ok {
		fs.Type = s
	}
	return fs, nil
}

func (a *aqlFileOps) Watch(path string, _ capabilities.WatchOpts) (<-chan capabilities.WatchEvent, func() error, error) {
	// AQL has no event source to bridge — a mounted filesystem is not
	// watchable. (A future mount contract could accept an emit callback.)
	return nil, nil, &os.PathError{Op: "watch", Path: path, Err: errMountUnsupported}
}

func (a *aqlFileOps) ResolvePath(path string) (string, error) {
	if _, ok := a.handlers.Get("resolve"); ok {
		res, err := a.call("resolve", []Value{NewPathonFromString(path)})
		if err != nil {
			return "", err
		}
		if s, serr := res.AsConcreteString(); serr == nil {
			return s, nil
		}
	}
	return filepath.Clean(path), nil
}

// Lock and Mmap REFUSE cleanly on a mount: an advisory lock that locks
// nothing (or a mapping over a value-based backend with no fd) is worse
// than an honest refusal.
func (a *aqlFileOps) Lock(path string, _, _ bool) (io.Closer, error) {
	return nil, &os.PathError{Op: "lock", Path: path, Err: errMountUnsupported}
}

func (a *aqlFileOps) Mmap(path string, _ int64, _ int, _ bool) (capabilities.MmapRegion, error) {
	return nil, &os.PathError{Op: "mmap", Path: path, Err: errMountUnsupported}
}

var _ capabilities.FileOps = (*aqlFileOps)(nil)

// mountPrevKey is the capability slot holding the FileOps that was
// active before IO.mount, so IO.unmount can restore it exactly.
const mountPrevKey = "engine.fileops.mountprev"

// mountedPrev is the typed wrapper for the saved slot.
type mountedPrev struct {
	ops    capabilities.FileOps // the pre-mount FileOps (nil if the slot was empty)
	hadOps bool
}

// doMountWord implements IO.mount: validate the handler map, save the
// current host FileOps, and install the bridge (policy-wrapped like any
// SetHostFileOps installation).
func doMountWord(args []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[0], "mount")
	if err != nil {
		return nil, r.AqlError("mount_error", "mount: the handler map must be a concrete map", "mount")
	}
	handlers := NewOrderedMap()
	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		if _, isFn := v.Data.(FnDefInfo); !isFn {
			return nil, r.AqlError("mount_error",
				fmt.Sprintf("mount: handler %q is %s — every handler must be a Function", key, v.Parent), "mount")
		}
		handlers.Set(key, v)
	}
	if _, ok := handlers.Get("read"); !ok {
		return nil, r.AqlErrorHint("mount_error", "mount: a mounted filesystem needs at least a `read` handler", "mount",
			"supply {read: ([p] => [...]), ...}; unhandled operations refuse cleanly")
	}
	prevOps, hadOps, _ := eng.Cap[capabilities.FileOps](r, CapFileOps)
	prev := &mountedPrev{ops: prevOps, hadOps: hadOps}
	_ = r.Capabilities.Set(mountPrevKey, prev)
	SetHostFileOps(r, &aqlFileOps{r: r, handlers: handlers})
	return nil, nil
}

// doMountZipWord implements the Pathon form of IO.mount: it reads a ZIP
// archive (engaged by a ".zip" path or an explicit {zip:true}) through the
// EFFECTIVE FileOps — so an archive staged in the mem backend is mountable,
// and policy gates the read — and installs a read-only ZipFileOps as the
// host filesystem. {writable:true} layers the archive as the read-only lower
// of a fresh in-memory overlay, giving copy-on-write edits that live only in
// memory. IO.unmount restores the previous FileOps, exactly like a handler
// mount.
func doMountZipWord(args []Value, r *Registry, opts Value) ([]Value, error) {
	target := extractPath(args[0])
	zipIntent := strings.HasSuffix(strings.ToLower(target), ".zip")
	writable := false
	if IsConcrete(opts) {
		if mapBoolOpt(opts, "zip", false) {
			zipIntent = true
		}
		writable = mapBoolOpt(opts, "writable", false)
	}
	if !zipIntent {
		return nil, r.AqlErrorHint("mount_error",
			fmt.Sprintf("mount: %q is not a .zip archive", target), "mount",
			"pass a .zip path or {zip:true} to mount an archive, or a handler map to mount an AQL filesystem")
	}
	data, err := EffectiveFileOps(r).ReadFile(target)
	if err != nil {
		return nil, r.AqlError("mount_error", fmt.Sprintf("mount: %v", err), "mount")
	}
	zfs, err := capabilities.NewZip(data)
	if err != nil {
		return nil, r.AqlError("mount_error", fmt.Sprintf("mount: not a valid zip archive: %v", err), "mount")
	}
	var ops capabilities.FileOps = zfs
	if writable {
		ops = capabilities.NewOverlay(capabilities.NewMem(), zfs)
	}
	prevOps, hadOps, _ := eng.Cap[capabilities.FileOps](r, CapFileOps)
	prev := &mountedPrev{ops: prevOps, hadOps: hadOps}
	_ = r.Capabilities.Set(mountPrevKey, prev)
	SetHostFileOps(r, ops)
	return nil, nil
}

// doUnmountWord implements IO.unmount: restore the FileOps saved by the
// most recent mount.
func doUnmountWord(_ []Value, r *Registry) ([]Value, error) {
	prev, ok, _ := eng.Cap[*mountedPrev](r, mountPrevKey)
	if !ok || prev == nil {
		return nil, r.AqlError("unmount_error", "unmount: no mounted filesystem to unmount", "unmount")
	}
	if prev.hadOps && prev.ops != nil {
		_ = r.Capabilities.Set(CapFileOps, prev.ops)
	} else {
		_, _ = r.Capabilities.Delete(CapFileOps)
	}
	_, _ = r.Capabilities.Delete(mountPrevKey)
	return nil, nil
}
