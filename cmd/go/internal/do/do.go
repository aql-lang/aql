// Package do implements `aql do <words...>` — join the remaining
// args with spaces, run as an AQL expression, print the result.
package do

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/permsflags"
	"github.com/aql-lang/aql/cmd/go/internal/run"
)

type cmd struct{}

// New returns the do subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "do" }
func (*cmd) Synopsis() string { return "evaluate args as an AQL expression" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	// Separate permission flags from positional words. We accept
	// every --perms* / --allow / --deny / --no-install / --install
	// flag at the head of the argv, then the remainder forms the
	// expression. flag.FlagSet stops at the first non-flag token,
	// so this works as long as users put flags first.
	fs := flag.NewFlagSet("do", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	source := strings.Join(fs.Args(), " ")
	if source == "" {
		fmt.Fprintf(stderr, "error: aql do requires an expression\n")
		return 1
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if err := run.EvalWithPolicy(stdout, source, "", 0, pol); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}
