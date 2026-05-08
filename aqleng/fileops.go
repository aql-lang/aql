package aqleng

import "os"

// FileOps abstracts the file-system operations used by the engine.
//
// The engine treats this as an opaque dependency: it only forwards a
// FileOps value to handlers that need it, never instantiates one
// directly. The hosting package wires up a concrete implementation
// (OS-backed, in-memory, sandboxed, etc.) via Registry.SetFileOps
// before running user code.
type FileOps interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	ResolvePath(path string) (string, error)
}
