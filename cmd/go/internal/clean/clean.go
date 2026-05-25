// Package clean implements `aql clean [dir]` — delete everything in
// .aql/ except dotfiles.
package clean

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the clean subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "clean" }
func (*cmd) Synopsis() string { return "delete .aql/* except dotfiles" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run handles `aql clean [dir]`.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	aqlDir := filepath.Join(dir, ".aql")
	entries, err := os.ReadDir(aqlDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(aqlDir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "cleaned %s\n", aqlDir)
	return 0
}
