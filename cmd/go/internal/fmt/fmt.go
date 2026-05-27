// Package fmt implements `aql fmt [file.aql ...]` — format AQL
// source files in place via lang/go/formatter.Format.
//
// With no arguments, formats every .aql file in the current
// directory tree (skipping anything inside .aql/).
package fmt

import (
	stdfmt "fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/lang/go/formatter"
)

type cmd struct{}

// New returns the fmt subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "fmt" }
func (*cmd) Synopsis() string { return "format .aql source files in place" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run handles `aql fmt [file.aql ...]`.
func Run(args []string, stdout, stderr io.Writer) int {
	var files []string
	if len(args) == 0 {
		err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && info.Name() == ".aql" {
				return filepath.SkipDir
			}
			if !info.IsDir() && strings.HasSuffix(path, ".aql") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			stdfmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	} else {
		files = args
	}

	if len(files) == 0 {
		stdfmt.Fprintln(stdout, "no .aql files found")
		return 0
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			stdfmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		formatted := formatter.Format(string(data))
		if string(data) != formatted {
			if err := os.WriteFile(path, []byte(formatted), 0644); err != nil {
				stdfmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			stdfmt.Fprintln(stdout, path)
		}
	}
	return 0
}
