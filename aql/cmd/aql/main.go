package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
	"github.com/metsitaba/voxgig-exp/aql/internal/repl"
)

// Version is set at build time via ldflags.
var Version = "0.1.0-dev"

func main() {
	os.Exit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// execute runs the CLI logic and returns an exit code.
func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("aql", flag.ContinueOnError)
	fs.SetOutput(stderr)

	evalExpr := fs.String("e", "", "evaluate expression")
	showVersion := fs.Bool("version", false, "print version and exit")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "aql %s\n", Version)
		return 0
	}

	// Determine the source code to process.
	var source string
	var hasSource bool

	if *evalExpr != "" {
		source = *evalExpr
		hasSource = true
	} else if fs.NArg() > 0 {
		filename := fs.Arg(0)
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		source = string(data)
		hasSource = true
	}

	if hasSource {
		if err := run(stdout, source); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
	}

	// No source provided: start the REPL.
	fmt.Fprintf(stdout, "aql %s\n", Version)
	repl.Start(stdin, stdout)
	return 0
}

func run(w io.Writer, source string) error {
	values, err := parser.Parse(source)
	if err != nil {
		return fmt.Errorf("parse error: %s", err)
	}

	eng := engine.New(engine.DefaultRegistry())
	result, err := eng.Run(values)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}

	if len(result) > 0 {
		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = v.String()
		}
		fmt.Fprintln(w, strings.Join(parts, " "))
	}
	return nil
}
