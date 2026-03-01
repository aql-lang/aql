package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
	"github.com/metsitaba/voxgig-exp/aql/internal/repl"
)

// Version is set at build time via ldflags.
var Version = "0.1.0-dev"

func main() {
	evalExpr := flag.String("e", "", "evaluate expression")
	showVersion := flag.Bool("version", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: aql [options] [script.aql]\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("aql %s\n", Version)
		return
	}

	// Determine the source code to process.
	var source string
	var hasSource bool

	if *evalExpr != "" {
		source = *evalExpr
		hasSource = true
	} else if flag.NArg() > 0 {
		filename := flag.Arg(0)
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		source = string(data)
		hasSource = true
	}

	if hasSource {
		run(source)
		return
	}

	// No source provided: start the REPL.
	fmt.Printf("aql %s\n", Version)
	repl.Start(os.Stdin, os.Stdout)
}

func run(source string) {
	values, err := parser.Parse(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %s\n", err)
		os.Exit(1)
	}

	eng := engine.New(engine.DefaultRegistry())
	result, err := eng.Run(values)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if len(result) > 0 {
		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = v.String()
		}
		fmt.Println(strings.Join(parts, " "))
	}
}
