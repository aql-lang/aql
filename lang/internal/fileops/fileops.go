// Package fileops provides an internal abstraction over file system operations.
// All dangerous file I/O goes through this interface so it can be replaced
// for testing or sandboxing without touching the Go os package directly.
package fileops

import (
	"os"
	"path/filepath"
)

// FileOps defines the file operations that AQL's read/write words use.
// The default implementation delegates to the os package.
// Replace with a custom implementation for testing or sandboxing.
type FileOps interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
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

// NewDefault returns the default OS-backed file operations.
func NewDefault() FileOps {
	return &OSFileOps{}
}

// MemFileOps is an in-memory implementation for testing.
type MemFileOps struct {
	Files map[string][]byte
	Dirs  map[string]bool // tracked directories
	Cwd   string          // simulated working directory; defaults to "." if empty
}

func NewMem() *MemFileOps {
	return &MemFileOps{
		Files: make(map[string][]byte),
		Dirs:  make(map[string]bool),
	}
}

func (m *MemFileOps) ReadFile(path string) ([]byte, error) {
	resolved, err := m.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	data, ok := m.Files[resolved]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return data, nil
}

func (m *MemFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	resolved, err := m.ResolvePath(path)
	if err != nil {
		return err
	}
	m.Files[resolved] = make([]byte, len(data))
	copy(m.Files[resolved], data)
	return nil
}

// MkdirAll records a directory path. Idempotent.
func (m *MemFileOps) MkdirAll(path string, perm os.FileMode) error {
	resolved, err := m.ResolvePath(path)
	if err != nil {
		return err
	}
	m.Dirs[resolved] = true
	// Also record parent dirs.
	for d := filepath.Dir(resolved); d != resolved && d != "." && d != "/"; d = filepath.Dir(d) {
		m.Dirs[d] = true
		resolved = d
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
