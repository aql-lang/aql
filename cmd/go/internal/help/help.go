// Package help implements `aql help [word]` — list available words
// or print dynamic help for a single word backed by the native
// registry.
package help

import (
	"fmt"
	"io"
	"sort"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/lang/go/native"
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

type cmd struct{}

// New returns the help subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "help" }
func (*cmd) Synopsis() string   { return "list available words or describe one" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, _ io.Reader, stdout, _ io.Writer) int {
	return Run(args, stdout)
}

// Run handles `aql help` and `aql help <word>`.
func Run(args []string, w io.Writer) int {
	if len(args) == 0 {
		words := helppkg.Words()
		sort.Strings(words)

		fmt.Fprintln(w, "Available words:")
		for _, word := range words {
			entry := helppkg.Lookup(word)
			fmt.Fprintf(w, "  %-16s %s\n", word, entry.Summary)
		}
		fmt.Fprintln(w, "\nUse 'aql help <word>' for detailed help on a specific word.")
		return 0
	}

	name := args[0]

	reg, err := native.DefaultRegistry()
	if err == nil {
		if info := native.BuildFuncInfo(reg, name); info != nil {
			fmt.Fprint(w, helppkg.FormatDynamic(*info))
			return 0
		}
	}

	entry := helppkg.Lookup(name)
	if entry == nil {
		fmt.Fprintf(w, "help: no help available for %q\n", name)
		return 1
	}
	fmt.Fprint(w, helppkg.Format(entry))
	return 0
}
