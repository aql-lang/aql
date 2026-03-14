package repl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// PROMPT is the REPL prompt string.
const PROMPT = ">> "

// newReadline and newRegistry are package-level vars for testability.
var newReadline = func(cfg *readline.Config) (readliner, error) {
	return readline.NewEx(cfg)
}

var newRegistry = func() (*engine.Registry, error) {
	return engine.DefaultRegistry()
}

// readliner abstracts the readline interface for testing.
type readliner interface {
	Readline() (string, error)
	Close() error
}

// Start runs the REPL loop, reading from in and writing to out.
func Start(in io.Reader, out io.Writer) {
	rl, err := newReadline(&readline.Config{
		Prompt:          PROMPT,
		HistoryFile:     historyFile(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdin:           toReadCloser(in),
		Stdout:          out,
	})
	if err != nil {
		fmt.Fprintf(out, "readline error: %s\n", err)
		return
	}
	defer rl.Close()

	// Shared registry so set/get state persists across REPL lines.
	registry, regErr := newRegistry()
	if regErr != nil {
		fmt.Fprintf(out, "init error: %s\n", regErr)
		return
	}
	registry.SetParseFunc(parser.Parse)
	registry.Output = out

	for {
		line, err := rl.Readline()
		if err != nil { // EOF or interrupt
			return
		}

		if line == "" {
			continue
		}

		values, err := parser.Parse(line)
		if err != nil {
			fmt.Fprintf(out, "  parse error: %s\n", err)
			continue
		}

		eng := engine.NewTop(registry)
		result, err := eng.Run(values)
		if err != nil {
			fmt.Fprintf(out, "  error: %s\n", err)
			continue
		}

		if len(result) > 0 {
			parts := make([]string, len(result))
			for i, v := range result {
				parts[i] = v.String()
			}
			fmt.Fprintln(out, strings.Join(parts, " "))
		}
	}
}

// userHomeDir is a package-level var for testability.
var userHomeDir = os.UserHomeDir

func historyFile() string {
	home, err := userHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aql_history")
}

// toReadCloser wraps an io.Reader in an io.ReadCloser if needed.
func toReadCloser(r io.Reader) io.ReadCloser {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc
	}
	return io.NopCloser(r)
}
