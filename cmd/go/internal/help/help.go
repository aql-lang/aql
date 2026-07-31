// Package help implements `boru help` — an overview of the boru
// command-line tool itself: its usage forms and the subcommands it
// dispatches. `boru help <subcommand>` prints a one-line summary and
// points at that subcommand's own -h flags. Documentation for the BORU
// *language* (words and modules) lives under `boru describe`.
package help

import (
	"fmt"
	"io"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	helppkg "github.com/boru-lang/boru/lang/go/native/help"
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

// writeOverview prints the top-level introduction: usage forms, the two ways
// to get help (`help` for the CLI, `describe` for the language), the one-shot
// commands, the long-running services, and pointers at the deeper help.
func writeOverview(w io.Writer, reg *command.Registry, services map[string]bool) {
	fmt.Fprintln(w, "boru — command-line tool for the BORU query language.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  boru [options] [script.boru]   Run a script, an -e expression, or the REPL.")
	fmt.Fprintln(w, "  boru <subcommand> [args...]   Run one of the subcommands below.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Two kinds of help — pick by what you're asking about:")
	fmt.Fprintln(w, "  help      drives the boru tool      — its subcommands and their flags.")
	fmt.Fprintln(w, "  describe  documents the language   — its words, categories, and modules.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  boru help                List the subcommands below (this screen).")
	fmt.Fprintln(w, "  boru help <subcommand>   Summary and flags for one subcommand, e.g. boru help vault.")
	fmt.Fprintln(w, "  boru describe            A categorised guide to every word and module.")
	fmt.Fprintln(w, "  boru describe <word>     Full docs for one word, e.g. boru describe add.")
	fmt.Fprintln(w, "  boru describe <category> The words in one category, e.g. boru describe math.")
	fmt.Fprintln(w, "  boru describe boru:<module>[:<word>]   A module, or one of its words.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range reg.Commands() {
		if !services[c.Name()] {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Services (long-running; composable under `boru serve`):")
	for _, c := range reg.Commands() {
		if services[c.Name()] {
			fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Synopsis())
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Docs: "+helppkg.RepoURL)
}

// helpCommand prints the summary for a single subcommand, or a hint if
// the name is not a known subcommand.
func helpCommand(w io.Writer, reg *command.Registry, name string) int {
	c, ok := reg.Lookup(name)
	if !ok {
		fmt.Fprintf(w, "boru help: unknown command %q.\n", name)
		fmt.Fprintf(w, "Run 'boru help' for the command list, or 'boru describe %s' for a language word.\n", name)
		return 1
	}
	fmt.Fprintf(w, "boru %s — %s\n", c.Name(), c.Synopsis())
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run 'boru %s -h' for its options.\n", c.Name())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Docs: "+helppkg.RepoURL+"/blob/main/CLI.md")
	return 0
}
