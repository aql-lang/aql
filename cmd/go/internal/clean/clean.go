// Package clean implements `boru clean [dir]` — delete everything in
// .boru/ except dotfiles.
package clean

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
)

// osRemoveAll is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive Run's removal-failure arm, which cannot be forced via directory
// permissions when the suite runs as root.
var osRemoveAll = os.RemoveAll

type cmd struct{}

// New returns the clean subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "clean" }
func (*cmd) Synopsis() string { return "delete .boru/* except dotfiles" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run handles `boru clean [dir]`.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		// Expand a leading ~ the shell left verbatim (e.g. a quoted dir).
		dir = pathutil.Expand(args[0])
	}

	boruDir := filepath.Join(dir, ".boru")
	entries, err := os.ReadDir(boruDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(boruDir, e.Name())
		if err := osRemoveAll(path); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "cleaned %s\n", boruDir)
	return 0
}
