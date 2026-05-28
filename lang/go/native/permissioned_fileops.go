package native

import (
	"os"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/policy"
)

// permissionedFileOps wraps a FileOps implementation, gating each
// method through a Policy. The wrapper is installed automatically
// by SetHostFileOps when a Policy is present on the registry.
//
// Each method:
//  1. Consults the relevant global hard cap (disk.read / disk.write).
//  2. Consults the fileops scope rule for the op with the path arg.
//  3. Delegates to the inner FileOps on allow.
//
// Returns the *policy.Denied error verbatim on refusal so callers
// can pattern-match on Code.
type permissionedFileOps struct {
	inner  capabilities.FileOps
	policy policy.Policy
}

// NewPermissionedFileOps wraps ops with a permissions gate. If pol
// is nil, the input is returned unchanged.
func NewPermissionedFileOps(ops capabilities.FileOps, pol policy.Policy) capabilities.FileOps {
	if pol == nil {
		return ops
	}
	return &permissionedFileOps{inner: ops, policy: pol}
}

// ReadFile gates on global.disk.read + fileops.read{path}.
func (p *permissionedFileOps) ReadFile(path string) ([]byte, error) {
	if err := p.policy.Check("fileops", "read", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.ReadFile(path)
}

// WriteFile gates on global.disk.write + fileops.write{path,bytes}.
func (p *permissionedFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := p.policy.Check("fileops", "write", policy.Args{
		"path":  path,
		"bytes": int64(len(data)),
	}); err != nil {
		return err
	}
	return p.inner.WriteFile(path, data, perm)
}

// MkdirAll gates on global.disk.write + fileops.mkdir{path}.
func (p *permissionedFileOps) MkdirAll(path string, perm os.FileMode) error {
	if err := p.policy.Check("fileops", "mkdir", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.MkdirAll(path, perm)
}

// ResolvePath is pure path manipulation — no I/O, no policy check.
// Resolution must succeed even for paths that read/write would
// later deny, so consumers can compute the canonical form before
// hitting a gated method.
func (p *permissionedFileOps) ResolvePath(path string) (string, error) {
	return p.inner.ResolvePath(path)
}

// compile-time interface check.
var _ capabilities.FileOps = (*permissionedFileOps)(nil)
