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

	"github.com/aql-lang/aql/cmd/go/internal/buildrt"
	"github.com/aql-lang/aql/cmd/go/internal/check"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	"github.com/aql-lang/aql/cmd/go/internal/permsflags"
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
	checkFirst := fs.Bool("check", false, "verbose pre-flight: print ALL check diagnostics (the pre-flight itself runs by default; this flag adds the advisory tiers to stderr)")
	noCheck := fs.Bool("no-check", false, "skip the static pre-flight check before execution (also enabled by AQL_NO_CHECK)")
	compileFlag := fs.Bool("compile", false, "execute via the bytecode compiler (now the DEFAULT; kept for compatibility); silent interpreter fallback when not compilable; AQL_NO_COMPILE disables")
	forceCompileFlag := fs.Bool("force-compile", false, "REQUIRE the bytecode compiler — abort with the refusal reason instead of falling back to the interpreter (also enabled by AQL_FORCE_COMPILE; AQL_NO_COMPILE disables)")
	optionsStr := fs.String("options", "", "engine options as jsonic (e.g. tape:initial:65536,tape:grows:9)")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: aql [options] [script.aql]\n       aql do <words...>\n       aql check [script.aql]\n       aql help [subcommand]\n       aql describe [word|module]\n       aql fmt [file.aql ...]\n       aql build <prog.aql> [-o name]\n       aql prep [dir]\n       aql pack [dir]\n       aql clean [dir]\n       aql lsp [-p <port>]\n       aql exec [-bind host:port] [-p <port>] [-r <registry>]\n       aql registry -r <folder> -p <port>\n       aql serve <svc> [flags] [+ <svc> [flags]]...\n       aql ctl [--api url] [--token tok] <op> [name]\n       aql tui [--api url] [--token tok]\n       aql install <name>-x.y.z [-r <url>]\n       aql register [-r <url>]\n       aql login [-r <url>]\n       aql publish [-r <url>] [dir]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "aql %s\n", Version)
		return 0
	}

	// Expand a leading ~ the shell left verbatim (e.g. -r=~/reg or a
	// quoted "~/script.aql") in the registry path and the script path.
	reg := pathutil.Expand(*registry)

	var source string
	var hasSource bool

	if *evalExpr != "" {
		source = *evalExpr
		hasSource = true
	} else if fs.NArg() > 0 {
		filename := pathutil.Expand(fs.Arg(0))
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		source = string(data)
		hasSource = true
	}

	if hasSource {
		// CHECK-BY-DEFAULT (completion plan Phase 5.2): every run
		// pre-flights unless --no-check / AQL_NO_CHECK. The default gate is
		// QUIET — diagnostics print only when an Error-severity finding
		// aborts the run, so a clean program's stderr stays empty and the
		// advisory tiers don't become per-run noise. An explicit --check
		// keeps the verbose behavior (all diagnostics, every severity).
		// Sequenced after the guard-narrowing legalization per the plan's
		// FP-honesty rule; `aql check --soft` remains the advisory surface.
		if !*noCheck && os.Getenv("AQL_NO_CHECK") == "" {
			if err := check.Preflight(stderr, source, reg, *seed, *checkFirst); err != nil {
				fmt.Fprintf(stderr, "%s\n", err)
				return 1
			}
		}
		pol, err := pf.Resolve()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		o := lang.Options{Registry: reg, Seed: *seed, Policy: pol}
		if *optionsStr != "" {
			m, perr := lang.ParseOptions(*optionsStr)
			if perr != nil {
				fmt.Fprintf(stderr, "error: %s\n", perr)
				return 1
			}
			if aerr := lang.ApplyOptions(&o, m); aerr != nil {
				fmt.Fprintf(stderr, "error: %s\n", aerr)
				return 1
			}
		}
		if err := EvalOptionsMode(stdout, source, o, ResolveCompileMode(*compileFlag, *forceCompileFlag)); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
	}

	// No source provided: drop into the REPL.
	fmt.Fprintf(stdout, "aql %s\n", Version)
	repl.Start(stdin, stdout, reg)
	return 0
}

// Eval runs source through lang.New(...).Run and writes the carrier
// stack to w. Exposed for the do subcommand, which builds source
// from positional args. Equivalent to EvalWithPolicy with a nil
// policy.
func Eval(w io.Writer, source string, registry string, seed int64) error {
	return EvalWithPolicy(w, source, registry, seed, nil)
}

// OptionsFor assembles the lang.Options the legacy positional CLI
// arguments map to. Exposed for sibling subcommands (do) so they
// don't import lang directly.
func OptionsFor(registry string, seed int64, pol lang.Policy) lang.Options {
	return lang.Options{Registry: registry, Seed: seed, Policy: pol}
}

// EvalWithPolicy is Eval with an explicit Policy. Pass nil for pol
// to preserve the historical default (no checks).
func EvalWithPolicy(w io.Writer, source string, registry string, seed int64, pol lang.Policy) error {
	return EvalOptions(w, source, lang.Options{Registry: registry, Seed: seed, Policy: pol})
}

// EvalOptions runs source under the full Options set (registry, seed,
// policy, tape bounds). The CLI builds Options from its flags —
// including --options — and calls this.
func EvalOptions(w io.Writer, source string, o lang.Options) error {
	return EvalOptionsMode(w, source, o, CompileOff)
}

// CompileMode selects which execution engine EvalOptionsMode drives: the
// interpreter (the default), the best-effort bytecode compiler (silent
// fallback), or the bytecode compiler in FORCE mode (error if uncompilable).
// The type and its constants live in buildrt so the standalone executable
// produced by `aql build` can reference them without importing run; run
// aliases them here to keep its public surface unchanged.
type CompileMode = buildrt.CompileMode

const (
	// CompileOff runs the interpreter — the default.
	CompileOff = buildrt.CompileOff
	// CompileTry runs the bytecode compiler when the program is compilable and
	// silently falls back to the interpreter otherwise (the `--compile` flag).
	CompileTry = buildrt.CompileTry
	// CompileForce REQUIRES the bytecode path: an uncompilable program (or a VM
	// soundness assertion) aborts with the refusal reason rather than falling
	// back (the `--force-compile` flag).
	CompileForce = buildrt.CompileForce
)

// ResolveCompileMode applies the bytecode-mode rollout contract
// (design/aql-bytecode-plan.0.md "Developer experience"; the Stage-7 flip is
// recorded in design/P7-ENDGAME.10.md): compiled mode is the DEFAULT — every
// run takes the bytecode path when the emitter can lower the program and
// silently falls back to the interpreter otherwise ("slow, not wrong"; the
// differential gates hold compile == interpret byte-identical across the
// corpus, the combination matrix, and the property fuzz). `AQL_NO_COMPILE`
// is the kill switch that wins over everything, exactly as the rollout
// contract reserved; `--force-compile` / `AQL_FORCE_COMPILE` still upgrade a
// refusal from silent fallback to a loud error; the historical `--compile` /
// `AQL_COMPILE` opt-ins remain accepted as no-ops of the new default.
func ResolveCompileMode(compile, force bool) CompileMode {
	if envEnabled("AQL_NO_COMPILE") {
		return CompileOff
	}
	if force || envEnabled("AQL_FORCE_COMPILE") {
		return CompileForce
	}
	_ = compile // accepted for compatibility; TRY is the default
	return CompileTry
}

// envEnabled reports whether an env var is set to a truthy value
// (present and not one of the empty/0/false/no forms).
func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// EvalOptionsMode is EvalOptions with the execution engine selected by mode:
// CompileOff runs the interpreter; CompileTry runs the bytecode path when the
// emitter can lower the program and silently falls back otherwise; CompileForce
// REQUIRES the bytecode path and errors (with the refusal reason) when the
// program is not compilable. CompileTry results are identical to the
// interpreter — the flag is opt-in performance, never semantics
// (design/aql-bytecode-plan.0.md, ground rules).
func EvalOptionsMode(w io.Writer, source string, o lang.Options, mode CompileMode) error {
	return buildrt.Eval(w, source, o, mode)
}
