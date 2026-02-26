package repl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chzyer/readline"

	"github.com/metsitaba/voxgig-exp/hay/internal/evaluator"
	"github.com/metsitaba/voxgig-exp/hay/internal/lexer"
	"github.com/metsitaba/voxgig-exp/hay/internal/parser"
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

	for {
		line, err := rl.Readline()
		if err != nil { // EOF or interrupt
			return
		}

		if line == "" {
			continue
		}

		l := lexer.New(line)
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) > 0 {
			printParserErrors(out, p.Errors())
			continue
		}

		result := evaluator.Eval(program)
		if result != nil {
			fmt.Fprintln(out, result.Inspect())
		}
	}
}

func printParserErrors(out io.Writer, errors []string) {
	for _, msg := range errors {
		fmt.Fprintf(out, "\tparse error: %s\n", msg)
	}
}

func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hay_history")
}

// toReadCloser wraps an io.Reader in an io.ReadCloser if needed.
func toReadCloser(r io.Reader) io.ReadCloser {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc
	}
	return io.NopCloser(r)
}
