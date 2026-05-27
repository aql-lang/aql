package native

import (
	"fmt"
	"os"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// notInstalledFileOps is the sentinel FileOps returned by the
// EffectiveFileOps / HostFileOps fallbacks when the host policy has
// uninstalled the fileops capability. Every method returns a
// permission-denied-shaped error so consumer sites that forgot to
// nil-check still get a clean error instead of a nil deref.
//
// This is the "structural denial" pattern from PERMISSIONS.0 — the
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
var _ capabilities.FileOps = notInstalledFileOps{}
