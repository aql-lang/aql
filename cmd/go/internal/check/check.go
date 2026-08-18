// Package check implements `boru check [--json] [--soft] [--strict] [script.boru]`
// — run the static type-checker over a boru source file or -e
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

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	lang "github.com/boru-lang/boru/lang/go"
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
	pedantic := false
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
		case "--pedantic", "-pedantic":
			pedantic = true
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
		fmt.Fprintf(stderr, "error: boru check requires a script file or -e expression\n")
		return 1
	}

	var source string
	if args[0] == "-e" {
		if len(args) < 2 {
			fmt.Fprintf(stderr, "error: boru check -e requires an expression\n")
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
	opts := Opts{
		JSON:     jsonOut,
		Soft:     soft,
		Strict:   strict,
		Pedantic: pedantic,
		Color:    lang.ResolveColor(nil, stderr, colorMode),
	}
	if err := RunWith(stdout, stderr, source, opts); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}

// Emit runs the bytecode recording pass over source and prints the
// Program disassembly to stdout, or the precise refusal reason when
// the emitter cannot lower the program (debug/tooling surface —
// design/boru-bytecode-plan.0.md, Stage 1 gate and the DX section).
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

// writeSiteReport prints the compile report (design/boru-bytecode-plan.0.md
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
	return RunWith(stdout, stderr, source, Opts{
		Registry: registry,
		Seed:     seed,
		JSON:     jsonOut,
		Soft:     soft,
		Strict:   strict,
		Color:    color,
	})
}

// Opts carries the check pass's options. It exists so new tiers can be
// added without growing RunColor's positional parameter list; Run and
// RunColor remain the fixed-shape wrappers their callers already use.
type Opts struct {
	Registry string
	Seed     int64
	JSON     bool
	Soft     bool
	Strict   bool
	// Pedantic promotes the advisory tiers: with it set, warning- and
	// info-severity diagnostics gate the exit code the way errors
	// already do. It composes UNDER Soft — `--soft` still means "never
	// gate", so `--soft --pedantic` exits 0 — which keeps each flag's
	// documented meaning intact (design/ROC-ADOPTION-PLAN.0.md, A3).
	Pedantic bool
	Color    bool
}

// gates reports whether sum should drive a non-zero exit, returning the error
// that names why. Soft short-circuits every tier; otherwise errors
// always gate and the advisory tiers gate only under Pedantic. Shared by
// the JSON and text paths so `--json --pedantic` cannot disagree with
// `--pedantic`.
func (o Opts) gates(sum lang.CheckSummary) error {
	if o.Soft {
		return nil
	}
	if sum.Errors > 0 {
		return fmt.Errorf("check failed: %d error(s)", sum.Errors)
	}
	if o.Pedantic && sum.Warnings+sum.Infos > 0 {
		return fmt.Errorf("check failed: %d warning(s), %d info (--pedantic)",
			sum.Warnings, sum.Infos)
	}
	return nil
}

// RunWith is Run with every option carried in a struct.
func RunWith(stdout, stderr io.Writer, source string, opts Opts) error {
	jsonOut, strict, color := opts.JSON, opts.Strict, opts.Color
	a, err := langNew(lang.Options{Registry: opts.Registry, Seed: opts.Seed})
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
		return opts.gates(res.Summary)
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
	return opts.gates(res.Summary)
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
// `boru run --check`: it prints any diagnostics to stderr and returns a
// non-nil error when an Error-severity diagnostic is present, so the
// caller aborts before executing. Unlike Run it prints no summary or
// result-stack line — stdout is left entirely for the program.
func Preflight(stderr io.Writer, source, registry string, seed int64, verbose bool) error {
	return PreflightColor(stderr, source, registry, seed, verbose, lang.ResolveColor(nil, stderr, "auto"))
}

// PreflightColor is Preflight with the color decision resolved by the
// caller (the run subcommand's --color flag). Relative file imports
// resolve against the process cwd — exactly how the run that follows
// will resolve them.
func PreflightColor(stderr io.Writer, source, registry string, seed int64, verbose, color bool) error {
	return PreflightColorAt(stderr, source, registry, seed, verbose, color, "")
}

// PreflightColorAt is PreflightColor with relative file imports anchored
// to baseDir instead of the process cwd. `boru build` (NUR044) needs the
// anchor: a built binary resolves `import "./lib.boru"` against the
// build-time entry directory (buildrt.Main sets NativeRegistry().BaseDir
// to cfg.EntryDir), so its pre-flight must ask the same question or a
// perfectly buildable multi-file program invoked from a foreign cwd
// would be refused. An empty baseDir keeps the cwd behaviour run/debug
// want — for them, cwd IS how the subsequent execution resolves imports.
func PreflightColorAt(stderr io.Writer, source, registry string, seed int64, verbose, color bool, baseDir string) error {
	a, err := langNew(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}
	if baseDir != "" {
		a.NativeRegistry().BaseDir = baseDir
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
