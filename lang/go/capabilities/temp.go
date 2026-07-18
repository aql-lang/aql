package capabilities

import "os"

// temp.go — the OS backend's unique-file/directory creation, thin
// wrappers over os.CreateTemp / os.MkdirTemp (which own the uniqueness
// retry loop). dir "" falls to os.TempDir, per the os contract.

// closeTempFile is the syscall seam for TempFile's post-create Close.
// A freshly-created temp file's Close failing is a real (if rare)
// deferred-write edge (NFS, disk quota); the branch is reached in
// tests by overriding this seam (design/TEST-SEAMS.10.md).
var closeTempFile = (*os.File).Close

// TempFile creates a unique file (0600) and returns its path.
func (o *OSFileOps) TempFile(dir, pattern string) (string, error) {
	if dir != "" {
		resolved, err := o.ResolvePath(dir)
		if err != nil {
			return "", err
		}
		dir = resolved
	}
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := closeTempFile(f); err != nil {
		return "", err
	}
	return name, nil
}

// TempDir creates a unique directory (0700) and returns its path.
func (o *OSFileOps) TempDir(dir, pattern string) (string, error) {
	if dir != "" {
		resolved, err := o.ResolvePath(dir)
		if err != nil {
			return "", err
		}
		dir = resolved
	}
	return os.MkdirTemp(dir, pattern)
}

// Statfs reports the filesystem holding path (platform impl in
// statfs_*.go behind the resolve step).
func (o *OSFileOps) Statfs(path string) (FsInfo, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return FsInfo{}, err
	}
	return osStatfs(resolved)
}
