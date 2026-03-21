package main

import (
	"archive/zip"
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
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n       aql help [word]\n       aql prep [dir]\n       aql pack [dir]\n       aql registry -r <folder> -p <port>\n       aql install <name>-x.y.z [-r <url>]\n\nOptions:\n")
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

	// Handle "pack" subcommand: aql pack [dir]
	if len(args) > 0 && args[0] == "pack" {
		return runPack(args[1:], stdout, stderr)
	}

	// Handle "registry" subcommand: aql registry -r <folder> -p <port>
	if len(args) > 0 && args[0] == "registry" {
		return runRegistry(args[1:], stdout, stderr)
	}

	// Handle "install" subcommand: aql install <name>-x.y.z [-r <url>]
	if len(args) > 0 && args[0] == "install" {
		return runInstall(args[1:], stdout, stderr)
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

// doPrep parses aql.jsonic and writes .aql/aql.json. Returns the parsed map.
func doPrep(dir string) (map[string]any, error) {
	src := filepath.Join(dir, "aql.jsonic")
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}

	j := jsonic.Make()
	parsed, err := j.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}

	m, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("aql.jsonic must be a map")
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}

	dst := filepath.Join(dir, ".aql", "aql.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(dst, append(out, '\n'), 0644); err != nil {
		return nil, err
	}

	return m, nil
}

// runPrep handles `aql prep [dir]`: parse aql.jsonic and write .aql/aql.json.
func runPrep(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	if _, err := doPrep(dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s\n", filepath.Join(dir, ".aql", "aql.json"))
	return 0
}

// runPack handles `aql pack [dir]`: prep then zip listed files + aql.jsonic.
func runPack(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	m, err := doPrep(dir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	name, _ := m["name"].(string)
	if name == "" {
		fmt.Fprintf(stderr, "error: aql.jsonic missing name\n")
		return 1
	}

	major, _ := m["major"].(float64)
	minor, _ := m["minor"].(float64)
	patch, _ := m["patch"].(float64)
	version := fmt.Sprintf("%d.%d.%d", int(major), int(minor), int(patch))

	rawFiles, ok := m["files"].([]any)
	if !ok {
		fmt.Fprintf(stderr, "error: aql.jsonic missing files list\n")
		return 1
	}

	// Collect files: explicit list + implicit aql.jsonic.
	files := []string{"aql.jsonic"}
	for _, f := range rawFiles {
		if s, ok := f.(string); ok {
			files = append(files, s)
		}
	}

	zipName := fmt.Sprintf("%s-%s.zip", name, version)
	zipPath := filepath.Join(dir, ".aql", zipName)

	zf, err := os.Create(zipPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer zf.Close()

	zw := zip.NewWriter(zf)
	defer zw.Close()

	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		w, err := zw.Create(f)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if _, err := w.Write(data); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "%s\n", zipPath)
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
