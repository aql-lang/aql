// Command aql is the thin binary entrypoint for the AQL CLI.
//
// The CLI implementation lives in the parent package
// (github.com/aql-lang/aql/cmd/go) as a library so it can be imported
// and tested. This file only exists so `go install` produces a binary
// named `aql` instead of `go` (Go's install rule takes the binary name
// from the main package's directory).
//
// Install:
//
//	go install github.com/aql-lang/aql/cmd/go/aql@latest
package main

import aql "github.com/aql-lang/aql/cmd/go"

func main() {
	aql.Run()
}
