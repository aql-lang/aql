package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/metsitaba/voxgig-exp/hay/internal/evaluator"
	"github.com/metsitaba/voxgig-exp/hay/internal/lexer"
	"github.com/metsitaba/voxgig-exp/hay/internal/parser"
	"github.com/metsitaba/voxgig-exp/hay/internal/repl"
)

// Version is set at build time via ldflags.
var Version = "0.1.0-dev"

func main() {
	evalExpr := flag.String("e", "", "evaluate expression")
	showVersion := flag.Bool("version", false, "print version and exit")
	showAST := flag.Bool("ast", false, "print AST and exit")
	showTokens := flag.Bool("tokens", false, "print tokens and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: hay [options] [script.hay]\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("hay %s\n", Version)
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
		run(source, *showTokens, *showAST)
		return
	}

	// No source provided: start the REPL.
	fmt.Printf("hay %s\n", Version)
	repl.Start(os.Stdin, os.Stdout)
}

func run(source string, showTokens bool, showAST bool) {
	if showTokens {
		l := lexer.New(source)
		tokens := l.Tokenize()
		for _, tok := range tokens {
			fmt.Printf("%+v\n", tok)
		}
		return
	}

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		for _, msg := range p.Errors() {
			fmt.Fprintf(os.Stderr, "parse error: %s\n", msg)
		}
		os.Exit(1)
	}

	if showAST {
		fmt.Println(program.String())
		return
	}

	result := evaluator.Eval(program)
	if result != nil {
		fmt.Println(result.Inspect())
	}
}
