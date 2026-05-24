// Package aql is the top-level dispatcher for the AQL command-line
// tool. It owns the Version constant (rewritten by `make publish`)
// and one short execute() function that routes args[0] to the
// matching subcommand package under internal/. Everything else
// lives in its own package.
package aql

import (
	"fmt"
	"io"
	"os"

	"github.com/aql-lang/aql/cmd/go/internal/check"
	"github.com/aql-lang/aql/cmd/go/internal/clean"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/do"
	aqlfmt "github.com/aql-lang/aql/cmd/go/internal/fmt"
	"github.com/aql-lang/aql/cmd/go/internal/help"
	"github.com/aql-lang/aql/cmd/go/internal/install"
	"github.com/aql-lang/aql/cmd/go/internal/login"
	"github.com/aql-lang/aql/cmd/go/internal/lsp"
	"github.com/aql-lang/aql/cmd/go/internal/pack"
	"github.com/aql-lang/aql/cmd/go/internal/prep"
	"github.com/aql-lang/aql/cmd/go/internal/publish"
	"github.com/aql-lang/aql/cmd/go/internal/register"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
	"github.com/aql-lang/aql/cmd/go/internal/repl"
	"github.com/aql-lang/aql/cmd/go/internal/run"
)

// Version is the aql CLI version. It is rewritten by the publish
// target before tagging, and may also be overridden at build time
// with `-ldflags "-X github.com/aql-lang/aql/cmd/go.Version=x.y.z"`.
var Version = "0.1.0-dev"

// Run is the binary entrypoint. The thin main package at
// cmd/go/aql calls this so the installed binary is named `aql`
// rather than `go`.
func Run() {
	os.Exit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// execute resolves args[0] to a Command and runs it. If args[0]
// is not a registered subcommand (or args is empty), the call
// falls through to the run subcommand, which owns the legacy
// `aql [-e expr] [script.aql]` shape and the no-args REPL drop-in.
func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	run.SetVersion(Version)
	reg := buildRegistry()

	if len(args) > 0 {
		if c, ok := reg.Lookup(args[0]); ok {
			return c.Run(args[1:], stdin, stdout, stderr)
		}
	}

	// Legacy fallthrough: aql, aql -e EXPR, aql script.aql, aql -version.
	fallback, _ := reg.Lookup("run")
	return fallback.Run(args, stdin, stdout, stderr)
}

// buildRegistry registers every subcommand. The order here drives
// the Usage listing — single-pass commands first (grouped by
// purpose), then server commands at the end.
func buildRegistry() *command.Registry {
	r := command.New()
	// Single-pass: language execution.
	r.Register(run.New())
	r.Register(do.New())
	r.Register(check.New())
	r.Register(help.New())
	r.Register(aqlfmt.New())
	// Single-pass: project lifecycle.
	r.Register(prep.New())
	r.Register(pack.New())
	r.Register(clean.New())
	// Single-pass: registry client.
	r.Register(install.New())
	r.Register(register.New())
	r.Register(login.New())
	r.Register(publish.New())
	// Server: long-running input loops.
	r.Register(repl.New())
	r.Register(registry.New())
	r.Register(lsp.New())
	return r
}

// Usage prints a short overview of every registered subcommand to
// w, grouped by Mode. Exposed for tooling that wants to render
// help without invoking the CLI.
func Usage(w io.Writer) {
	reg := buildRegistry()
	fmt.Fprintln(w, "Usage: aql [options] [script.aql]")
	fmt.Fprintln(w, "       aql <subcommand> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Single-pass subcommands:")
	for _, c := range reg.Commands() {
		if c.Mode() == command.ModeSinglePass {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Server subcommands:")
	for _, c := range reg.Commands() {
		if c.Mode() == command.ModeServer {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
}
