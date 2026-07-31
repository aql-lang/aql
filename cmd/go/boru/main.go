// Command boru is the thin binary entrypoint for the BORU CLI.
//
// The CLI implementation lives in the parent package
// (github.com/boru-lang/boru/cmd/go) as a library so it can be imported
// and tested. This file only exists so `go install` produces a binary
// named `boru` instead of `go` (Go's install rule takes the binary name
// from the main package's directory).
//
// Install:
//
//	go install github.com/boru-lang/boru/cmd/go/boru@latest
package main

import boru "github.com/boru-lang/boru/cmd/go"

// runCLI is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// observe the dispatch to boru.Run without running the real CLI, which
// would exit the test process.
var runCLI = boru.Run

func main() {
	runCLI()
}
