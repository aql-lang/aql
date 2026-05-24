// Package check implements `aql check [--json] [--soft] [script.aql]`
// — run the static type-checker over an AQL source file or -e
// expression and report diagnostics.
//
// Without --soft, the presence of any Error-severity diagnostic
// causes the command to exit non-zero (the default mode used by
// CI). --soft downgrades every diagnostic to advisory.
package check

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	lang "github.com/aql-lang/aql/lang/go"
)

type cmd struct{}

// New returns the check subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "check" }
func (*cmd) Synopsis() string   { return "static type-check a script or expression" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return RunCLI(args, stdout, stderr)
}

// RunCLI is the entry point for the check subcommand, parsing flags
// from args.
func RunCLI(args []string, stdout, stderr io.Writer) int {
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

	if err := Run(stdout, stderr, source, "", 0, jsonOut, soft); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}

// Run executes the static type-checker over source and writes the
// carrier stack and diagnostics to the provided writers. It returns
// an error for a parse/execution failure; diagnostics on their own
// do not fail the run (they're printed to stderr).
//
// When jsonOut is true, the entire CheckResult is emitted to stdout
// as a single JSON object suitable for editor / tooling integration.
//
// When soft is false (the default), any Error-severity diagnostic
// causes a non-nil error to be returned so the caller propagates a
// non-zero exit code. Passing soft=true downgrades every diagnostic
// to advisory: Run returns nil as long as the underlying analysis
// completes.
func Run(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft bool) error {
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
