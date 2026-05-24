// Package lsp implements `aql lsp` — a Language Server Protocol
// server backed by the existing lang.Check, formatter.Format, and
// help.Lookup APIs.
//
// By default the server speaks stdio (the LSP convention). Pass
// -p <port> to listen on TCP instead; LSP servers are single-
// client, so the first accepted connection drives the server until
// it closes.
//
// Methods implemented: initialize, initialized, shutdown, exit,
// textDocument/didOpen, didChange, didClose, hover, completion,
// formatting (full-document). Anything else returns "method not
// found".
package lsp

import (
	"flag"
	"fmt"
	"io"
	"net"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the lsp subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "lsp" }
func (*cmd) Synopsis() string   { return "run a Language Server Protocol server on stdio or TCP" }
func (*cmd) Mode() command.Mode { return command.ModeServer }

// Run handles `aql lsp [-p <port>]`. With no flags the server
// reads/writes stdio. Passing -p binds a TCP listener on the given
// port and serves the first connection (LSP servers are single-
// client).
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("p", 0, "TCP port to listen on (0 = stdio mode)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *port == 0 {
		return newServer(stdin, stdout, stderr).run()
	}

	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "lsp: listen %s: %s\n", addr, err)
		return 1
	}
	defer ln.Close()

	fmt.Fprintf(stderr, "aql lsp listening on %s\n", addr)
	conn, err := ln.Accept()
	if err != nil {
		fmt.Fprintf(stderr, "lsp: accept: %s\n", err)
		return 1
	}
	defer conn.Close()

	return newServer(conn, conn, stderr).run()
}
