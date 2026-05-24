// cmd.go adds the command.Command wrapper for the repl subcommand
// without touching the existing repl.Start implementation in repl.go.

package repl

import (
	"flag"
	"fmt"
	"io"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the repl subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "repl" }
func (*cmd) Synopsis() string   { return "start the interactive read-eval-print loop" }
func (*cmd) Mode() command.Mode { return command.ModeServer }

// Run handles `aql repl [-r <registry>]`. With no flags it starts a
// REPL on stdio; -r passes a registry path to native.DefaultRegistry.
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("repl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryPath := fs.String("r", "", "registry path passed to the UniversalManager")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Print a small banner so explicit `aql repl` matches the
	// no-arg invocation, which prints the version line.
	fmt.Fprintf(stdout, "aql repl\n")
	Start(stdin, stdout, *registryPath)
	return 0
}
