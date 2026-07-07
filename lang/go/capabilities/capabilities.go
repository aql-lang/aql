// Package capabilities provides the abstraction for host-side capabilities
// the AQL engine needs to talk to the outside world. At present it covers
// file-system access (the FileOps interface and its OS-backed and
// in-memory implementations); future host capabilities (network,
// process spawn, …) go here too. All dangerous I/O routes through these
// interfaces so it can be replaced for testing or sandboxing without
// touching the Go os/net/etc. packages directly.
package capabilities

import (
	"os"
	"path/filepath"
	"time"
)

// Clock is the host capability that supplies "the current time" to AQL's
// temporal words (`now`, the aql:time `time-now*` family) and to the
// default seed of aql:rand. The default implementation reads the wall
// clock; a FixedClock can be installed for deterministic tests/specs so
// `now` and time-dependent output are reproducible.
type Clock interface {
	Now() time.Time
}

// WallClock is the default Clock: it returns the real system time.
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }

// FixedClock is a Clock frozen at a single instant — used by the spec
// runner and tests so temporal words produce deterministic results.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }

// StepAction is what a StepController decides to do at a paused step.
type StepAction int

const (
	// StepInto runs the next single step, then pauses again.
	StepInto StepAction = iota
	// StepContinue resumes until the next breakpoint (or completion).
	StepContinue
	// StepQuit abandons the stepped run.
	StepQuit
)

// StepFrame is the rendered state handed to a StepController at a pause.
// It carries only stdlib types so this package stays engine-agnostic — the
// stack is pre-rendered to strings by the caller.
type StepFrame struct {
	Step    int      // 0-based step index
	Pointer int      // index of the value about to execute
	Stack   []string // the tape, rendered, one entry per slot
	AtBreak bool     // true when paused on a Debug.break marker
}

// StepController is the host capability that drives interactive stepping
// (Debug.step). At each pause the engine renders the frame and calls
// OnStep; the returned StepAction decides what happens next. A TTY host
// reads a keypress; a test supplies a scripted list of actions; a
// non-interactive host returns StepContinue to run straight through.
type StepController interface {
	OnStep(frame StepFrame) StepAction
}

// DebugOps is the host capability backing the effectful parts of
// aql:debug that the in-process module cannot supply itself: the
// interactive step controller (and, in future, runtime hooks). Absent a
// DebugOps, Debug.step falls back to printing the trace.
type DebugOps interface {
	Controller() StepController
}

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
	// MemFileOps.ResolvePath never fails (both of its arms return a nil
	// error), so there is no error to propagate here or in WriteFile /
	// MkdirAll — see design/TEST-SEAMS.10.md on provably-dead guards.
	resolved, _ := m.ResolvePath(path)
	data, ok := m.Files[resolved]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return data, nil
}

func (m *MemFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	resolved, _ := m.ResolvePath(path) // never fails — see ReadFile
	m.Files[resolved] = make([]byte, len(data))
	copy(m.Files[resolved], data)
	return nil
}

// MkdirAll records a directory path. Idempotent.
func (m *MemFileOps) MkdirAll(path string, perm os.FileMode) error {
	resolved, _ := m.ResolvePath(path) // never fails — see ReadFile
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
