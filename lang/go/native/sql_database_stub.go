//go:build js

package native

import "errors"

// External database connectivity relies on database/sql drivers that
// are not available in the wasm build, so the browser build offers only
// the embedded SQLite engine. These stubs keep the aql:db module
// compiling under GOOS=js with a clear runtime error.

// OpenDBConn always fails in the wasm build.
func OpenDBConn(_, _ string) (*DBConn, error) {
	return nil, errors.New("external database connections are not supported in the wasm build")
}

// CanonicalDBKind reports no external kinds in the wasm build.
func CanonicalDBKind(string) (string, bool) { return "", false }
