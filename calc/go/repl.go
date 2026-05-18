package calc

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// REPL runs a read-eval-print loop against c, reading lines from in
// and writing prompts/results/errors to out.
//
// Lines beginning with `:` are meta-commands rather than calc source:
//
//	:stack          show the current stack
//	:clear          drop everything on the stack
//	:words          list every registered word
//	:help           short reminder of the meta-command set
//	:quit / :exit   leave the REPL
//
// The empty line is ignored. Parse and runtime errors are reported
// inline; the stack is unchanged so the user can correct and retry.
//
// Returns when in reaches EOF or the user types :quit / :exit.
func REPL(c *Calc, in io.Reader, out io.Writer, prompt string) {
	if prompt == "" {
		prompt = "calc> "
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Fprint(out, prompt)
		if !scanner.Scan() {
			fmt.Fprintln(out)
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			if quit := runMeta(c, out, line); quit {
				return
			}
			continue
		}
		stk, err := c.Eval(line)
		if err != nil {
			fmt.Fprintf(out, "  error: %s\n", err)
			continue
		}
		if len(stk) > 0 {
			fmt.Fprintln(out, FormatStack(stk))
		}
	}
}

func runMeta(c *Calc, out io.Writer, line string) (quit bool) {
	cmd := strings.TrimPrefix(line, ":")
	cmd = strings.TrimSpace(cmd)
	switch cmd {
	case "quit", "exit", "q":
		return true
	case "stack", "s":
		fmt.Fprintln(out, FormatStack(c.Stack()))
	case "clear", "c":
		c.Reset()
	case "words", "w":
		names := c.Registry.Defs.Names()
		fmt.Fprintln(out, strings.Join(names, " "))
	case "help", "h", "?":
		fmt.Fprintln(out, "calc meta-commands:")
		fmt.Fprintln(out, "  :stack | :s     show the stack")
		fmt.Fprintln(out, "  :clear | :c     clear the stack")
		fmt.Fprintln(out, "  :words | :w     list registered words")
		fmt.Fprintln(out, "  :quit  | :q     exit")
		fmt.Fprintln(out, "  :help  | :h     this message")
	default:
		fmt.Fprintf(out, "  unknown meta-command: %s (try :help)\n", cmd)
	}
	return false
}
