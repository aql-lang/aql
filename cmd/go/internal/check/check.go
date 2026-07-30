// Package check implements `aql check [--json] [--soft] [--strict] [script.aql]`
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
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	lang "github.com/aql-lang/aql/lang/go"
)

// langNew is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive the init-error arms of Emit/Run/Preflight — lang.New only fails
// on registry construction errors that no Options value can provoke.
var langNew = lang.New

// jsonMarshalIndent is a test seam (design/TEST-SEAMS.10.md); tests swap
// it to drive Run's marshal-error arm, which is unreachable with the
// plain CheckResult shape.
var jsonMarshalIndent = json.MarshalIndent

type cmd struct{}

// New returns the check subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "check" }
func (*cmd) Synopsis() string { return "static type-check a script or expression" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return RunCLI(args, stdout, stderr)
}

// RunCLI is the entry point for the check subcommand, parsing flags
// from args.
func RunCLI(args []string, stdout, stderr io.Writer) int {
	jsonOut := false
	soft := false
	emit := false
	strict := false
	colorMode := "auto"
	for len(args) > 0 {
		switch args[0] {
		case "--color", "-color":
			if len(args) > 1 {
				colorMode = args[1]
				args = args[2:]
				continue
			}
			args = args[1:]
		case "--json", "-json":
			jsonOut = true
			args = args[1:]
		case "--soft", "-soft":
			soft = true
			args = args[1:]
		case "--strict", "-strict":
			strict = true
			args = args[1:]
		case "--emit", "-emit":
			emit = true
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
		// Expand a leading ~ the shell left verbatim (e.g. a quoted path).
		data, err := os.ReadFile(pathutil.Expand(args[0]))
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		source = string(data)
	}

	if emit {
		if err := Emit(stdout, stderr, source); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
	}
	if err := RunColor(stdout, stderr, source, "", 0, jsonOut, soft, strict, lang.ResolveColor(nil, stderr, colorMode)); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}

// Emit runs the bytecode recording pass over source and prints the
// Program disassembly to stdout, or the precise refusal reason when
// the emitter cannot lower the program (debug/tooling surface —
// design/aql-bytecode-plan.0.md, Stage 1 gate and the DX section).
func Emit(stdout, stderr io.Writer, source string) error {
	a, err := langNew()
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}
	prog, reason, res, err := a.CompileCheck(source)
	for i := range res.Diagnostics {
		d := &res.Diagnostics[i]
		fmt.Fprintf(stderr, "check: %s [%s] %s: %s\n", atPos(d.Row, d.Col), d.Severity, d.Code, d.Detail)
	}
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}
	if prog == nil {
		fmt.Fprintf(stdout, "uncompilable: %s\n", reason)
		writeSiteReport(stdout, res.SiteCounts, nil)
		return nil
	}
	fmt.Fprint(stdout, prog.Disassemble())
	islands := make([]string, len(prog.Fallbacks))
	for i, fb := range prog.Fallbacks {
		islands[i] = fb.Desc
	}
	writeSiteReport(stdout, res.SiteCounts, islands)
	return nil
}

// writeSiteReport prints the compile report (design/aql-bytecode-plan.0.md
// DX section): the per-class dispatch-site tally that answers "why didn't
// this compile to a single path?", plus the interpreter islands a
// compiled program falls back into for each fallback span.
func writeSiteReport(w io.Writer, counts map[string]int, islands []string) {
	if len(counts) > 0 {
		fmt.Fprintf(w, "; sites: mono=%d poly=%d dynamic=%d meta=%d\n",
			counts["mono"], counts["poly"], counts["dynamic"], counts["meta"])
	}
	if len(islands) > 0 {
		fmt.Fprintf(w, "; islands: %s\n", strings.Join(islands, ", "))
	}
}

func atPos(row, col int) string {
	if row == 0 {
		return "-"
	}
	return fmt.Sprintf("%d:%d", row, col)
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
func Run(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft, strict bool) error {
	return RunColor(stdout, stderr, source, registry, seed, jsonOut, soft, strict, false)
}

// RunColor is Run with the color decision resolved by the caller
// (lang.ResolveColor); the rich per-diagnostic blocks render through
// the shared diagnostic renderer either way.
func RunColor(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft, strict, color bool) error {
	a, err := langNew(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}

	if strict {
		a.SetStrictCheck(true)
	}
	res, err := a.Check(source)
	if jsonOut {
		out, jerr := jsonMarshalIndent(res, "", "  ")
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

	printDiagnostics(stderr, res.Diagnostics, source, color)
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

// printDiagnostics writes each diagnostic to w in the `check: row:col:
// [sev] code: detail` form shared by Run and Preflight. The one-liner
// is a parsing contract (editors and CI scripts key on it); the RICH
// block underneath — source excerpt, notes, suggestions — is additive
// (design/DIAGNOSTICS.0.md, phase 6).
func printDiagnostics(w io.Writer, diags []lang.CheckDiagnostic, source string, color bool) {
	for _, d := range diags {
		sev := string(d.Severity)
		if sev == "" {
			sev = "info"
		}
		if d.Row > 0 {
			fmt.Fprintf(w, "check: %d:%d: [%s] %s: %s\n", d.Row, d.Col, sev, d.Code, d.Detail)
		} else {
			fmt.Fprintf(w, "check: [%s] %s: %s\n", sev, d.Code, d.Detail)
		}
		if block := lang.RenderCheckDiagnostic(d, source, "", lang.RenderOpts{Color: color}); block != "" {
			fmt.Fprintln(w, block)
		}
	}
}

// Preflight runs the static checker as a pre-execution gate for
// `aql run --check`: it prints any diagnostics to stderr and returns a
// non-nil error when an Error-severity diagnostic is present, so the
// caller aborts before executing. Unlike Run it prints no summary or
// result-stack line — stdout is left entirely for the program.
func Preflight(stderr io.Writer, source, registry string, seed int64, verbose bool) error {
	return PreflightColor(stderr, source, registry, seed, verbose, lang.ResolveColor(nil, stderr, "auto"))
}

// PreflightColor is Preflight with the color decision resolved by the
// caller (the run subcommand's --color flag).
func PreflightColor(stderr io.Writer, source, registry string, seed int64, verbose, color bool) error {
	a, err := langNew(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}
	res, cerr := a.Check(source)
	// Quiet by default (check-by-default runs this on EVERY execution):
	// diagnostics surface only when the run is about to abort, or when
	// the caller asked for the verbose pre-flight (--check).
	if verbose || cerr != nil || res.Summary.Errors > 0 {
		printDiagnostics(stderr, res.Diagnostics, source, color)
	}
	if cerr != nil {
		return fmt.Errorf("check error: %s", cerr)
	}
	if res.Summary.Errors > 0 {
		return fmt.Errorf("check failed: %d error(s)", res.Summary.Errors)
	}
	return nil
}
