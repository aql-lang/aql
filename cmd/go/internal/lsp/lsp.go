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
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the lsp subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "lsp" }
func (*cmd) Synopsis() string { return "run a Language Server Protocol server on stdio or TCP" }

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

	var srv *Server
	if *port == 0 {
		srv = NewStdioServer(stdin, stdout, stderr)
	} else {
		srv = NewTCPServer(*port, stderr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "lsp: %s\n", err)
		return 1
	}
	return srv.ExitCode()
}
