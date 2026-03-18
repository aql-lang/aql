package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	aql "github.com/metsitaba/voxgig-exp/aql"
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
	registry := fs.String("r", "", "registry path")
	seed := fs.Int64("s", 0, "random seed for ID generation (default: current time)")
	showVersion := fs.Bool("version", false, "print version and exit")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n\nOptions:\n")
		fs.PrintDefaults()
	}

	// Handle "do" subcommand before flag parsing: aql do <words...>
	if len(args) > 0 && args[0] == "do" {
		doSource := strings.Join(args[1:], " ")
		if doSource == "" {
			fmt.Fprintf(stderr, "error: aql do requires an expression\n")
			return 1
		}
		if err := run(stdout, doSource, "", 0); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
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
		if err := run(stdout, source, *registry, *seed); err != nil {
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

func run(w io.Writer, source string, registry string, seed int64) error {
	a, err := aql.New(aql.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}

	result, err := a.Run(source)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}

	if len(result) > 0 {
		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = fmt.Sprint(v)
		}
		fmt.Fprintln(w, strings.Join(parts, " "))
	}
	return nil
}
