package native

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// notInstalledFileOps is the sentinel FileOps returned by the
// EffectiveFileOps / HostFileOps fallbacks when the host policy has
// uninstalled the fileops capability. Every method returns a
// permission-denied-shaped error so consumer sites that forgot to
// nil-check still get a clean error instead of a nil deref.
//
// This is the "structural denial" pattern from PERMISSIONS.10 — the
// only FileOps reachable to handlers either does I/O or refuses; it
// never crashes.
type notInstalledFileOps struct{}

func (notInstalledFileOps) ReadFile(path string) ([]byte, error) {
	return nil, capabilityNotInstalled("fileops", "read", path)
}

func (notInstalledFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	return capabilityNotInstalled("fileops", "write", path)
}

func (notInstalledFileOps) MkdirAll(path string, perm os.FileMode) error {
	return capabilityNotInstalled("fileops", "mkdir", path)
}

func (notInstalledFileOps) Stat(path string, follow bool) (capabilities.FileInfo, error) {
	return capabilities.FileInfo{}, capabilityNotInstalled("fileops", "stat", path)
}

func (notInstalledFileOps) ReadDir(path string) ([]capabilities.FileInfo, error) {
	return nil, capabilityNotInstalled("fileops", "list", path)
}

func (notInstalledFileOps) Remove(path string, recursive bool) error {
	return capabilityNotInstalled("fileops", "remove", path)
}

func (notInstalledFileOps) Rename(oldPath, newPath string) error {
	return capabilityNotInstalled("fileops", "rename", oldPath)
}

func (notInstalledFileOps) Symlink(target, linkPath string) error {
	return capabilityNotInstalled("fileops", "link", linkPath)
}

func (notInstalledFileOps) Link(target, linkPath string) error {
	return capabilityNotInstalled("fileops", "link", linkPath)
}

func (notInstalledFileOps) Chmod(path string, mode os.FileMode) error {
	return capabilityNotInstalled("fileops", "chmod", path)
}

func (notInstalledFileOps) Chtimes(path string, atime, mtime time.Time) error {
	return capabilityNotInstalled("fileops", "chmod", path)
}

func (notInstalledFileOps) Chown(path string, uid, gid int, follow bool) error {
	return capabilityNotInstalled("fileops", "chown", path)
}

func (notInstalledFileOps) XattrGet(path, name string) ([]byte, error) {
	return nil, capabilityNotInstalled("fileops", "xattr-get", path)
}

func (notInstalledFileOps) XattrSet(path, name string, value []byte) error {
	return capabilityNotInstalled("fileops", "xattr-set", path)
}

func (notInstalledFileOps) XattrList(path string) ([]string, error) {
	return nil, capabilityNotInstalled("fileops", "xattr-list", path)
}

func (notInstalledFileOps) XattrRemove(path, name string) error {
	return capabilityNotInstalled("fileops", "xattr-remove", path)
}

func (notInstalledFileOps) TempFile(dir, pattern string) (string, error) {
	return "", capabilityNotInstalled("fileops", "temp", dir)
}

func (notInstalledFileOps) TempDir(dir, pattern string) (string, error) {
	return "", capabilityNotInstalled("fileops", "temp", dir)
}

func (notInstalledFileOps) Statfs(path string) (capabilities.FsInfo, error) {
	return capabilities.FsInfo{}, capabilityNotInstalled("fileops", "statfs", path)
}

func (notInstalledFileOps) Truncate(path string, size int64) error {
	return capabilityNotInstalled("fileops", "write", path)
}

func (notInstalledFileOps) ResolvePath(path string) (string, error) {
	// ResolvePath is pure path manipulation in real fileops; the
	// stub returns the input unchanged so callers that resolve a
	// path before deciding what to do don't trip up. The actual
	// read/write/mkdir still refuses.
	return path, nil
}

// capabilityNotInstalled builds the standard error returned by every
// stub method. The shape matches policy.Denied so consumer code that
// expects to see a permission_denied chain reads consistently.
func capabilityNotInstalled(scope, op, path string) error {
	return fmt.Errorf("[aql/capability_not_installed]: %s.%s denied: %s capability not installed (policy uninstalled it; path=%q)",
		scope, op, scope, path)
}

// compile-time interface check.
func (notInstalledFileOps) Watch(path string, _ capabilities.WatchOpts) (<-chan capabilities.WatchEvent, func() error, error) {
	return nil, nil, capabilityNotInstalled("fileops", "watch", path)
}

func (notInstalledFileOps) Open(path string, _ capabilities.OpenOpts) (capabilities.FileHandle, error) {
	return nil, capabilityNotInstalled("fileops", "open", path)
}

func (notInstalledFileOps) Lock(path string, _, _ bool) (io.Closer, error) {
	return nil, capabilityNotInstalled("fileops", "lock", path)
}

func (notInstalledFileOps) Mmap(path string, _ int64, _ int, _ bool) (capabilities.MmapRegion, error) {
	return nil, capabilityNotInstalled("fileops", "mmap", path)
}

var _ capabilities.FileOps = notInstalledFileOps{}
