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

// Start runs the REPL loop, reading from in and writing to out.
func Start(in io.Reader, out io.Writer) {
	rl, err := readline.NewEx(&readline.Config{
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
	registry := engine.DefaultRegistry()
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

func historyFile() string {
	home, err := os.UserHomeDir()
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
