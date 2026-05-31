// Package aql is the top-level dispatcher for the AQL command-line
// tool. It owns the Version constant (rewritten by `make publish`)
// and one short execute() function that routes args[0] to the
// matching subcommand package under internal/. Everything else
// lives in its own package.
package aql

import (
	"io"
	"os"

	"github.com/aql-lang/aql/cmd/go/internal/api"
	"github.com/aql-lang/aql/cmd/go/internal/check"
	"github.com/aql-lang/aql/cmd/go/internal/clean"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/ctl"
	"github.com/aql-lang/aql/cmd/go/internal/describe"
	"github.com/aql-lang/aql/cmd/go/internal/do"
	"github.com/aql-lang/aql/cmd/go/internal/exec"
	aqlfmt "github.com/aql-lang/aql/cmd/go/internal/fmt"
	"github.com/aql-lang/aql/cmd/go/internal/help"
	"github.com/aql-lang/aql/cmd/go/internal/install"
	"github.com/aql-lang/aql/cmd/go/internal/login"
	"github.com/aql-lang/aql/cmd/go/internal/lsp"
	"github.com/aql-lang/aql/cmd/go/internal/pack"
	"github.com/aql-lang/aql/cmd/go/internal/policy"
	"github.com/aql-lang/aql/cmd/go/internal/prep"
	"github.com/aql-lang/aql/cmd/go/internal/publish"
	"github.com/aql-lang/aql/cmd/go/internal/register"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
	"github.com/aql-lang/aql/cmd/go/internal/repl"
	"github.com/aql-lang/aql/cmd/go/internal/run"
	"github.com/aql-lang/aql/cmd/go/internal/serve"
	"github.com/aql-lang/aql/cmd/go/internal/tui"
	"github.com/aql-lang/aql/cmd/go/internal/vault"
)

// Version is the aql CLI version. It is rewritten by the publish
// target before tagging, and may also be overridden at build time
// with `-ldflags "-X github.com/aql-lang/aql/cmd/go.Version=x.y.z"`.
var Version = "0.1.0-dev"

// Run is the binary entrypoint. The thin main package at
// cmd/go/aql calls this so the installed binary is named `aql`
// rather than `go`.
func Run() {
	// Publish the version constant to the api package so the
	// /v1/server endpoint can report it without an import cycle.
	api.SetSupervisorVersion(Version)
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
// the Usage listing — one-shot Commands first (grouped by purpose),
// then long-running Services at the end.
func buildRegistry() *command.Registry {
	r := command.New()
	// provide hands the help command the live registry + service
	// classification at Run time. r is captured by reference and fully
	// populated before the closure is ever called, so the cycle that
	// would arise from importing the top-level package is avoided.
	provide := func() (*command.Registry, map[string]bool) { return r, serviceNames }
	// Commands: language execution.
	r.Register(run.New())
	r.Register(do.New())
	r.Register(check.New())
	r.Register(help.New(provide))
	r.Register(describe.New())
	r.Register(aqlfmt.New())
	// Commands: project lifecycle.
	r.Register(prep.New())
	r.Register(pack.New())
	r.Register(clean.New())
	// Commands: registry client.
	r.Register(install.New())
	r.Register(register.New())
	r.Register(login.New())
	r.Register(publish.New())
	// Commands: local secret management.
	r.Register(vault.New())
	// Commands: permission profiles.
	r.Register(policy.New())
	// Commands: supervisor control plane client.
	r.Register(ctl.New())
	// Services: long-running input loops.
	r.Register(repl.New())
	r.Register(registry.New())
	r.Register(lsp.New())
	r.Register(exec.New())
	r.Register(serve.New())
	r.Register(tui.New())
	return r
}

// serviceNames is the set of Commands that are also long-running
// services (composable under `aql serve`). Used by the help command to
// group them separately from one-shot commands.
var serviceNames = map[string]bool{
	"repl":     true,
	"registry": true,
	"lsp":      true,
	"exec":     true,
	"serve":    true,
	"tui":      true,
}
