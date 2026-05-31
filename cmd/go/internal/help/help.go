// Package help implements `aql help` — an overview of the aql
// command-line tool itself: its usage forms and the subcommands it
// dispatches. `aql help <subcommand>` prints a one-line summary and
// points at that subcommand's own -h flags. Documentation for the AQL
// *language* (words and modules) lives under `aql describe`.
package help

import (
	"fmt"
	"io"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

// Provider yields the live command registry and the set of service
// names at Run time. The top-level package supplies this so the help
// command can describe every registered subcommand without importing
// that package (which would be an import cycle).
type Provider func() (*command.Registry, map[string]bool)

type cmd struct{ provide Provider }

// New returns the help subcommand backed by provide.
func New(provide Provider) command.Command { return &cmd{provide: provide} }

func (*cmd) Name() string     { return "help" }
func (*cmd) Synopsis() string { return "show CLI usage, or help for a subcommand" }
func (c *cmd) Run(args []string, _ io.Reader, stdout, _ io.Writer) int {
	reg, services := c.provide()
	if len(args) == 0 {
		writeOverview(stdout, reg, services)
		return 0
	}
	return helpCommand(stdout, reg, args[0])
}

// writeOverview prints the top-level command overview: usage forms,
// the one-shot commands, the long-running services, and pointers at
// per-subcommand and per-word help.
func writeOverview(w io.Writer, reg *command.Registry, services map[string]bool) {
	fmt.Fprintln(w, "aql — command-line tool for the AQL query language.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  aql [options] [script.aql]   Run a script, an -e expression, or the REPL.")
	fmt.Fprintln(w, "  aql <subcommand> [args...]   Run one of the subcommands below.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range reg.Commands() {
		if !services[c.Name()] {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Services (long-running; composable under `aql serve`):")
	for _, c := range reg.Commands() {
		if services[c.Name()] {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Getting more help:")
	fmt.Fprintln(w, "  aql help <subcommand>   Summary and flags for one subcommand.")
	fmt.Fprintln(w, "  aql describe <word>     Documentation for an AQL language word.")
	fmt.Fprintln(w, "  aql describe            List language words and built-in modules.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Docs: "+helppkg.RepoURL)
}

// helpCommand prints the summary for a single subcommand, or a hint if
// the name is not a known subcommand.
func helpCommand(w io.Writer, reg *command.Registry, name string) int {
	c, ok := reg.Lookup(name)
	if !ok {
		fmt.Fprintf(w, "aql help: unknown command %q.\n", name)
		fmt.Fprintf(w, "Run 'aql help' for the command list, or 'aql describe %s' for a language word.\n", name)
		return 1
	}
	fmt.Fprintf(w, "aql %s — %s\n", c.Name(), c.Synopsis())
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run 'aql %s -h' for its options.\n", c.Name())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Docs: "+helppkg.RepoURL+"/blob/main/CLI.md")
	return 0
}
