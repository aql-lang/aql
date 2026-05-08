package engine

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// DefaultRegistry returns a registry populated with built-in primitives
// plus any additional provider functions passed in. Each provider is a
// function that registers words (e.g. engine.Register, native.Register).
// Called with no providers, it registers only engine's built-in core words.
//
// This is the entry point that wires the host-side concerns (SQLite,
// formats, file system access) into the bare aqleng.Registry returned
// by aqleng.NewRegistry.
func DefaultRegistry(providers ...func(*Registry)) (*Registry, error) {
	r, err := NewRegistry()
	if err != nil {
		return nil, err
	}

	// File operations: OS-backed by default, with the in-memory variant
	// available on demand via __sys.fs.mem.
	ops := fileops.NewDefault()
	r.FileOps = ops
	r.MemOpsFactory = func() FileOps { return fileops.NewMem() }

	// Format registry: the read/write words look formats up by name.
	// Re-wire the jsonic format's multisource resolver whenever the
	// host swaps FileOps.
	r.Formats = DefaultFormats()
	if jf, ok := r.Formats["jsonic"].(*JsonicFormat); ok {
		jf.Resolver = MakeFileOpsResolver(ops)
	}
	r.OnSetFileOps = func(newOps FileOps) {
		if jf, ok := r.Formats["jsonic"].(*JsonicFormat); ok {
			if real, ok2 := newOps.(fileops.FileOps); ok2 {
				jf.Resolver = MakeFileOpsResolver(real)
			}
		}
	}

	// SQLite store: used by the table/query words.
	sqlStore, err := NewSQLiteStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}
	r.SQLite = sqlStore

	// Register engine-bundled words and any caller-supplied providers.
	Register(r)
	for _, p := range providers {
		p(r)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	r.InitRootContext()
	return r, nil
}

// sqliteStore returns the registry's SQLite store, or nil if none is
// installed. Words that need SQLite access call this rather than
// type-asserting r.SQLite repeatedly.
func sqliteStore(r *Registry) *SQLiteStore {
	if r == nil || r.SQLite == nil {
		return nil
	}
	if s, ok := r.SQLite.(*SQLiteStore); ok {
		return s
	}
	return nil
}
