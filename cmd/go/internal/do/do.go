// Package do implements `aql do <words...>` — join the remaining
// args with spaces, run as an AQL expression, print the result.
package do

import (
	"fmt"
	"io"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/run"
)

type cmd struct{}

// New returns the do subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "do" }
func (*cmd) Synopsis() string { return "evaluate args as an AQL expression" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	source := strings.Join(args, " ")
	if source == "" {
		fmt.Fprintf(stderr, "error: aql do requires an expression\n")
		return 1
	}
	if err := run.Eval(stdout, source, "", 0); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}
