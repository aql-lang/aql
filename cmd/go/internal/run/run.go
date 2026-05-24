// Package run is both the explicit `aql run` subcommand and the
// fallback path the top-level dispatcher takes when no recognised
// subcommand matches: parse the legacy flags (-e, -r, -s, --check,
// -version), and either execute a one-shot script/-e expression or
// drop into the REPL when there is nothing to execute.
package run

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/check"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/repl"
	lang "github.com/aql-lang/aql/lang/go"
)

// Version is the aql CLI version string, populated by the top-level
// package via SetVersion before any Run call. Holding it here (rather
// than reading from cmd/go directly) keeps the import direction one-way:
// cmd/go → internal/run, never the other way.
var Version = "0.1.0-dev"

// SetVersion lets the top-level package inject its Version into the
// run subcommand so -version prints the right value.
func SetVersion(v string) { Version = v }

type cmd struct{}

// New returns the run subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "run" }
func (*cmd) Synopsis() string { return "execute a script or expression (or start the REPL)" }
func (*cmd) Mode() command.Mode {
	return command.ModeSinglePass
}
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Execute(args, stdin, stdout, stderr)
}

// Execute is the legacy CLI body. It owns the flag set for the
// no-subcommand invocation form (aql [-e expr] [script.aql]). When
// no source is provided it starts the REPL, preserving the original
// CLI's default behaviour.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("aql", flag.ContinueOnError)
	fs.SetOutput(stderr)

	evalExpr := fs.String("e", "", "evaluate expression")
	registry := fs.String("r", "", "registry path")
	seed := fs.Int64("s", 0, "random seed for ID generation (default: current time)")
	showVersion := fs.Bool("version", false, "print version and exit")
	checkFirst := fs.Bool("check", false, "run static type-check before execution; abort on error")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n       aql check [script.aql]\n       aql help [word]\n       aql fmt [file.aql ...]\n       aql prep [dir]\n       aql pack [dir]\n       aql clean [dir]\n       aql lsp [-p <port>]\n       aql registry -r <folder> -p <port>\n       aql install <name>-x.y.z [-r <url>]\n       aql register [-r <url>]\n       aql login [-r <url>]\n       aql publish [-r <url>] [dir]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "aql %s\n", Version)
		return 0
	}

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
			if err := check.Run(stdout, stderr, source, *registry, *seed, false, true); err != nil {
				fmt.Fprintf(stderr, "%s\n", err)
				return 1
			}
		}
		if err := Eval(stdout, source, *registry, *seed); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
	}

	// No source provided: drop into the REPL.
	fmt.Fprintf(stdout, "aql %s\n", Version)
	repl.Start(stdin, stdout, *registry)
	return 0
}

// Eval runs source through lang.New(...).Run and writes the carrier
// stack to w. Exposed for the do subcommand, which builds source
// from positional args.
func Eval(w io.Writer, source string, registry string, seed int64) error {
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
