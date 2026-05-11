package engine

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/lang/internal/fileops"
)

// DefaultRegistry returns a registry populated with built-in primitives
// plus any additional provider functions passed in. Each provider is a
// function that registers words (e.g. engine.Register, native.Register).
// Called with no providers, it registers only engine's built-in core words.
//
// This is the entry point that wires the host-side capabilities
// (FileOps, format registry, SQLite store) onto the bare
// eng.Registry returned by eng.NewRegistry. aqleng itself knows
// nothing about these — it just stores them in opaque capability slots
// for the host's word handlers to retrieve.
func DefaultRegistry(providers ...func(*Registry)) (*Registry, error) {
	r, err := NewRegistry()
	if err != nil {
		return nil, err
	}

	// Default file operations: OS-backed.
	ops := fileops.NewDefault()
	SetHostFileOps(r, ops)

	// Default format registry, with the jsonic resolver pointed at
	// the active fileops so @"path" references resolve.
	formats := DefaultFormats()
	if jf, ok := formats["jsonic"].(*JsonicFormat); ok {
		jf.Resolver = MakeFileOpsResolver(ops)
	}
	SetHostFormats(r, formats)

	// Default SQLite store.
	sqlStore, err := NewSQLiteStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}
	SetHostSQLite(r, sqlStore)

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
