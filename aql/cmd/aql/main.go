package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	aql "github.com/metsitaba/voxgig-exp/aql"
	jsonic "github.com/jsonicjs/jsonic/go"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
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
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n       aql help [word]\n       aql prep [dir]\n\nOptions:\n")
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

	// Handle "help" subcommand: aql help [word]
	if len(args) > 0 && args[0] == "help" {
		return runHelp(args[1:], stdout)
	}

	// Handle "prep" subcommand: aql prep [dir]
	if len(args) > 0 && args[0] == "prep" {
		return runPrep(args[1:], stdout, stderr)
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
	repl.Start(stdin, stdout, *registry)
	return 0
}

// runHelp handles `aql help` and `aql help <word>`.
func runHelp(args []string, w io.Writer) int {
	if len(args) == 0 {
		// No word specified: list all available words sorted.
		words := help.Words()
		sort.Strings(words)

		fmt.Fprintln(w, "Available words:")
		for _, word := range words {
			entry := help.Lookup(word)
			fmt.Fprintf(w, "  %-16s %s\n", word, entry.Summary)
		}
		fmt.Fprintln(w, "\nUse 'aql help <word>' for detailed help on a specific word.")
		return 0
	}

	name := args[0]
	entry := help.Lookup(name)
	if entry == nil {
		fmt.Fprintf(w, "help: no help available for %q\n", name)
		return 1
	}
	fmt.Fprint(w, help.Format(entry))
	return 0
}

// runPrep handles `aql prep [dir]`: parse aql.jsonic and write .aql/aql.json.
func runPrep(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	src := filepath.Join(dir, "aql.jsonic")
	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	j := jsonic.Make()
	parsed, err := j.Parse(string(data))
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid jsonic: %s\n", err)
		return 1
	}

	out, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	dst := filepath.Join(dir, ".aql", "aql.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if err := os.WriteFile(dst, append(out, '\n'), 0644); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s\n", dst)
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
