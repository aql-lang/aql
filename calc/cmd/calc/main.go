// Command calc is a small CLI / REPL calculator built on the AQL
// engine kernel.
//
// Usage:
//
//	calc                            # interactive REPL
//	calc -e "add 2 3"               # evaluate a one-shot expression
//	calc add 2 3                    # positional args are joined with spaces
//
// Without -e or positional args, calc starts a REPL that reads one
// expression per line from stdin and prints the resulting stack. Type
// :help inside the REPL for the meta-command list.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aql-lang/aql/calc"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point. It parses args from the supplied
// slice (excluding the program name) and reads from / writes to the
// supplied streams. Returns the desired process exit code.
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("calc", flag.ContinueOnError)
	fs.SetOutput(errOut)
	expr := fs.String("e", "", "evaluate this expression and exit")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "Usage: calc [-e EXPR] [WORDS...]")
		fmt.Fprintln(errOut, "  No args:      start a REPL on stdin/stdout")
		fmt.Fprintln(errOut, "  -e EXPR:      evaluate EXPR and print the stack")
		fmt.Fprintln(errOut, "  WORDS...:     joined with spaces and evaluated as one program")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Example: calc 10 sub 3   # 7")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	c, err := calc.New(out)
	if err != nil {
		fmt.Fprintf(errOut, "calc: %s\n", err)
		return 1
	}

	if *expr != "" {
		return evalOnce(c, *expr, out, errOut)
	}
	if rest := fs.Args(); len(rest) > 0 {
		return evalOnce(c, strings.Join(rest, " "), out, errOut)
	}
	calc.REPL(c, in, out, "")
	return 0
}

func evalOnce(c *calc.Calc, src string, out, errOut io.Writer) int {
	stk, err := c.Eval(src)
	if err != nil {
		fmt.Fprintf(errOut, "calc: %s\n", err)
		return 1
	}
	if len(stk) > 0 {
		fmt.Fprintln(out, calc.FormatStack(stk))
	}
	return 0
}
