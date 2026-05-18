package aql

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

	"github.com/aql-lang/aql/cmd/go/internal/repl"
	"github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/formatter"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/native/help"
	jsonic "github.com/jsonicjs/jsonic/go"
)

// Version is the aql CLI version. It is rewritten by the publish
// target before tagging, and may also be overridden at build time
// with `-ldflags "-X github.com/aql-lang/aql/cmd/go.Version=x.y.z"`.
var Version = "0.1.0-dev"

// Run is the binary entrypoint. It reads os.Args / os.Stdin etc.,
// runs the CLI, and exits with the appropriate status. The thin
// main package at cmd/go/aql calls this so the installed binary is
// named `aql` rather than `go`.
func Run() {
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
	checkFirst := fs.Bool("check", false, "run static type-check before execution; abort on error")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n       aql check [script.aql]\n       aql help [word]\n       aql fmt [file.aql ...]\n       aql prep [dir]\n       aql pack [dir]\n       aql clean [dir]\n       aql registry -r <folder> -p <port>\n       aql install <name>-x.y.z [-r <url>]\n       aql register [-r <url>]\n       aql login [-r <url>]\n       aql publish [-r <url>] [dir]\n\nOptions:\n")
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

	// Handle "check" subcommand: aql check [script.aql]
	if len(args) > 0 && args[0] == "check" {
		return runCheck(args[1:], stdout, stderr)
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

	// Handle "register" subcommand: aql register [-r <url>]
	if len(args) > 0 && args[0] == "register" {
		return runRegister(args[1:], stdin, stdout, stderr)
	}

	// Handle "login" subcommand: aql login [-r <url>]
	if len(args) > 0 && args[0] == "login" {
		return runLogin(args[1:], stdin, stdout, stderr)
	}

	// Handle "publish" subcommand: aql publish [-r <url>] [dir]
	if len(args) > 0 && args[0] == "publish" {
		return runPublish(args[1:], stdin, stdout, stderr)
	}

	// Handle "clean" subcommand: aql clean [dir]
	if len(args) > 0 && args[0] == "clean" {
		return runClean(args[1:], stdout, stderr)
	}

	// Handle "fmt" subcommand: aql fmt [file.aql ...]
	if len(args) > 0 && args[0] == "fmt" {
		return runFmt(args[1:], stdout, stderr)
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
		if *checkFirst {
			// --check before execution: always soft (don't block run).
			if err := check(stdout, stderr, source, *registry, *seed, false, true); err != nil {
				fmt.Fprintf(stderr, "%s\n", err)
				return 1
			}
		}
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

	// Build a registry to get dynamic signature data.
	reg, err := native.DefaultRegistry()
	if err == nil {
		if info := native.BuildFuncInfo(reg, name); info != nil {
			fmt.Fprint(w, help.FormatDynamic(*info))
			return 0
		}
	}

	// Fallback to static help if registry unavailable.
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
	packDir := filepath.Join(dir, ".aql", "_pack")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	zipPath := filepath.Join(packDir, zipName)

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
	a, err := lang.New(lang.Options{Registry: registry, Seed: seed})
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

// check runs the static type-checker over source and writes the carrier
// result stack and any diagnostics to the provided writers. It returns
// an error for a parse/execution failure; diagnostics on their own do
// not fail the run (they're printed to stderr).
//
// When jsonOut is true, the entire CheckResult is emitted to stdout as
// a single JSON object suitable for editor / tooling integration.
//
// When soft is false (the default), the presence of any Error-severity
// diagnostic causes a non-nil error to be returned so the caller
// propagates a non-zero exit code. Passing soft=true downgrades every
// diagnostic to advisory: check returns nil as long as the underlying
// analysis completes.
func check(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft bool) error {
	a, err := lang.New(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}

	res, err := a.Check(source)
	if jsonOut {
		out, jerr := json.MarshalIndent(res, "", "  ")
		if jerr != nil {
			return fmt.Errorf("json marshal: %s", jerr)
		}
		fmt.Fprintln(stdout, string(out))
		if err != nil {
			return fmt.Errorf("check error: %s", err)
		}
		if !soft && res.Summary.Errors > 0 {
			return fmt.Errorf("check failed: %d error(s)", res.Summary.Errors)
		}
		return nil
	}

	for _, d := range res.Diagnostics {
		sev := string(d.Severity)
		if sev == "" {
			sev = "info"
		}
		if d.Row > 0 {
			fmt.Fprintf(stderr, "check: %d:%d: [%s] %s: %s\n", d.Row, d.Col, sev, d.Code, d.Detail)
		} else {
			fmt.Fprintf(stderr, "check: [%s] %s: %s\n", sev, d.Code, d.Detail)
		}
	}
	if err != nil {
		return fmt.Errorf("check error: %s", err)
	}

	fmt.Fprintf(stderr, "check: %d error(s), %d warning(s), %d info\n",
		res.Summary.Errors, res.Summary.Warnings, res.Summary.Infos)

	if len(res.Stack) > 0 {
		fmt.Fprintln(stdout, "check: "+strings.Join(res.Stack, " "))
	} else {
		fmt.Fprintln(stdout, "check: (empty stack)")
	}
	if !soft && res.Summary.Errors > 0 {
		return fmt.Errorf("check failed: %d error(s)", res.Summary.Errors)
	}
	return nil
}

// runCheck implements the `aql check [--json] [--soft] [script.aql]`
// subcommand. By default the command exits non-zero when any
// Error-severity diagnostic was recorded. Pass --soft to report
// diagnostics while always exiting zero (advisory mode for CI).
func runCheck(args []string, stdout, stderr io.Writer) int {
	jsonOut := false
	soft := false
	for len(args) > 0 {
		switch args[0] {
		case "--json", "-json":
			jsonOut = true
			args = args[1:]
		case "--soft", "-soft":
			soft = true
			args = args[1:]
		default:
			goto done
		}
	}
done:
	if len(args) == 0 {
		fmt.Fprintf(stderr, "error: aql check requires a script file or -e expression\n")
		return 1
	}

	var source string
	if args[0] == "-e" {
		if len(args) < 2 {
			fmt.Fprintf(stderr, "error: aql check -e requires an expression\n")
			return 1
		}
		source = args[1]
	} else {
		data, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		source = string(data)
	}

	if err := check(stdout, stderr, source, "", 0, jsonOut, soft); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}

// runClean handles `aql clean [dir]`: delete everything in .aql/ except dotfiles.
func runClean(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	aqlDir := filepath.Join(dir, ".aql")
	entries, err := os.ReadDir(aqlDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(aqlDir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "cleaned %s\n", aqlDir)
	return 0
}

// runFmt handles `aql fmt [file.aql ...]`: format AQL source files in place.
// With no arguments, formats all .aql files in the current directory tree.
func runFmt(args []string, stdout, stderr io.Writer) int {
	var files []string
	if len(args) == 0 {
		// Find all .aql files in current directory tree.
		err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && info.Name() == ".aql" {
				return filepath.SkipDir
			}
			if !info.IsDir() && strings.HasSuffix(path, ".aql") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	} else {
		files = args
	}

	if len(files) == 0 {
		fmt.Fprintln(stdout, "no .aql files found")
		return 0
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		formatted := formatter.Format(string(data))
		if string(data) != formatted {
			if err := os.WriteFile(path, []byte(formatted), 0644); err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			fmt.Fprintln(stdout, path)
		}
	}
	return 0
}
