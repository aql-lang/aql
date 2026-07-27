// Package debugcmd implements the `aql debug` subcommand: the interactive
// debugger (design/AQL-DEBUGGER.0.md) and the front door to cross-process
// debugging (design/DEBUG-MODULE.0.md §7.2/§7.3).
//
//	aql debug [--script F] [--no-check] [--color M] <file.aql> [args...]
//	    Launch file.aql under the interactive debugger: pause between
//	    source lines, stop on Debug.break, inspect the stack / scope /
//	    backtrace, and evaluate expressions at a pause. --script reads
//	    debugger commands from a file (batch/CI mode). Runs on the
//	    interpreter (the trace does not fire on the compiled VM path).
//
//	aql debug serve [--bind 127.0.0.1:7777] [--token T] [file.aql]
//	    Load file.aql (if given) into a runtime, then serve its registry's
//	    debug introspection over HTTP and block until interrupted. Writes a
//	    discovery file ($TMPDIR/aql-debug.json) so `attach` can find it.
//
//	aql debug attach [--url U] [--token T] <words|defs|heap|eval|events> [arg]
//	    Connect to a running `aql debug serve` (via the discovery file or an
//	    explicit --url) and interrogate it.
//
// Serve/attach transport is plain HTTP with an optional Bearer token (a
// static token or a vault capability id), the same posture as the api
// service — the host-level realization of the attach/serverless surfaces
// while the AQL Service model (SERVICES.0.md) and a language-level socket
// primitive are still RFC-only.
package debugcmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/aql-lang/aql/cmd/go/internal/check"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/debugger"
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	"github.com/aql-lang/aql/eng/go/parser"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/debugserve"
	"github.com/aql-lang/aql/lang/go/native"
)

// langNew is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive runServe's init-error arm — lang.New only fails on registry
// construction errors that nothing here can provoke.
var langNew = lang.New

// jsonMarshal is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscovery's marshal-error arm, which is unreachable with
// the plain discovery shape.
var jsonMarshal = json.Marshal

type cmdImpl struct{}

// New returns the debug subcommand.
func New() command.Command { return &cmdImpl{} }

func (*cmdImpl) Name() string { return "debug" }
func (*cmdImpl) Synopsis() string {
	return "interactive debugger for a program; or serve/attach cross-process introspection"
}

func (*cmdImpl) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: aql debug [flags] <file.aql> | serve | attach ...")
		return 1
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "attach":
		return runAttach(args[1:], stdout, stderr)
	default:
		// Anything else is an interactive launch: aql debug [flags] <file.aql>
		// (design/AQL-DEBUGGER.0.md §4).
		return runLaunch(args, stdin, stdout, stderr)
	}
}

// runLaunch is the interactive-debugger entry: read + preflight the file
// exactly as `aql run` does, wire a registry, and run the program's
// tokens under a debugger.Session (design/AQL-DEBUGGER.0.md §4-§5).
func runLaunch(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	script := fs.String("script", "", "read debugger commands from this file instead of stdin (batch/CI mode)")
	noCheck := fs.Bool("no-check", false, "skip the static pre-flight check before debugging (also enabled by AQL_NO_CHECK)")
	colorMode := fs.String("color", "auto", "diagnostic color: auto (terminal-only, honors NO_COLOR), always, never")
	postMortem := fs.Bool("post-mortem", false, "on an uncaught error, open an inspection prompt over the fault state before exiting")
	breakOnError := fs.Bool("break-on-error", false, "pause at every raise BEFORE it unwinds — including errors a do-handler will catch")
	var breaks []string
	fs.Func("break", "set a breakpoint before the run starts: a source line ('12', 'file:12') or a word name ('add'); repeatable", func(v string) error {
		breaks = append(breaks, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return 1
	}
	file := fs.Arg(0)
	if file == "" {
		fmt.Fprintln(stderr, "usage: aql debug [--script F] [--no-check] [--color M] <file.aql> [args...]")
		return 1
	}
	path := pathutil.Expand(file)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "aql debug: read %s: %s\n", file, err)
		return 1
	}
	source := string(data)

	// Check-by-default, mirroring `aql run` (run.Execute): quiet gate,
	// --no-check / AQL_NO_CHECK to skip.
	color := lang.ResolveColor(stderr, *colorMode)
	if !*noCheck && os.Getenv("AQL_NO_CHECK") == "" {
		if cerr := check.PreflightColor(stderr, source, "", 0, false, color); cerr != nil {
			fmt.Fprintf(stderr, "%s\n", cerr)
			return 1
		}
	}

	a, err := langNew()
	if err != nil {
		fmt.Fprintf(stderr, "aql debug: init: %s\n", err)
		return 1
	}
	reg := a.NativeRegistry()
	reg.Output = stdout
	reg.BaseFile = path
	if *script != "" {
		// Batch mode: commands come from the --script file, so the launch
		// stdin belongs entirely to the PROGRAM (IO.stdin). Interactive
		// mode leaves reg.Input alone — the prompt's reader and a
		// stdin-reading program cannot safely share one descriptor (the
		// prompt's Scanner reads ahead), a documented Phase-1 limit.
		reg.Input = stdin
	}
	if extra := fs.Args()[1:]; len(extra) > 0 {
		// Positionals after the script path reach the program as IO.args,
		// like `aql run`.
		native.SetHostScriptArgs(reg, extra)
	}

	tokens, perr := parser.Parse(source)
	if perr != nil {
		fmt.Fprintf(stderr, "aql debug: parse %s: %s\n", file, perr)
		return 1
	}

	cmdIn := stdin
	if *script != "" {
		f, oerr := os.Open(pathutil.Expand(*script))
		if oerr != nil {
			fmt.Fprintf(stderr, "aql debug: script: %s\n", oerr)
			return 1
		}
		defer func() { _ = f.Close() }()
		cmdIn = f
	}

	sess := debugger.New(reg, debugger.Config{
		In:  cmdIn,
		Out: stdout,
		// The expanded path, matching reg.BaseFile, so pause locations
		// and error attribution name one canonical file string.
		File:         path,
		Source:       source,
		Echo:         *script != "",
		BreakOnError: *breakOnError,
	})
	fmt.Fprintf(stdout, "aql debug: %s — type 'help' for commands\n", file)
	for _, b := range breaks {
		fmt.Fprintln(stdout, sess.SetBreak(b))
	}
	res, rerr := sess.RunProgram(tokens, source)
	if rerr != nil {
		if *postMortem {
			sess.PostMortem(rerr)
		}
		var ae *lang.AqlError
		if color && errors.As(rerr, &ae) {
			fmt.Fprintf(stderr, "error: %s\n", ae.Render(lang.RenderOpts{Color: true}))
		} else {
			fmt.Fprintf(stderr, "error: %s\n", rerr)
		}
		return 1
	}
	if len(res) > 0 {
		// Residuals render print-style (FormatForPrint), matching `aql run`
		// and the session's own stack/defs renderers.
		parts := make([]string, len(res))
		for i, v := range res {
			parts[i] = native.FormatForPrint(v)
		}
		fmt.Fprintln(stdout, strings.Join(parts, " "))
	}
	fmt.Fprintln(stdout, "(program exited)")
	return 0
}

// discoveryPath is where `serve` advertises its URL+token for `attach`.
func discoveryPath() string {
	return filepath.Join(os.TempDir(), "aql-debug.json")
}

type discovery struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bind := fs.String("bind", "127.0.0.1:7777", "loopback address to serve on")
	token := fs.String("token", "", "require this Bearer token (a static token or vault capability id)")
	allowPublic := fs.Bool("allow-public", false, "permit a non-loopback bind (exposes introspection to the network)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	a, err := langNew()
	if err != nil {
		fmt.Fprintf(stderr, "debug serve: init: %s\n", err)
		return 1
	}

	// Load the program (if given) so its defs populate the registry that
	// `attach` will introspect.
	if file := fs.Arg(0); file != "" {
		src, rerr := os.ReadFile(file)
		if rerr != nil {
			fmt.Fprintf(stderr, "debug serve: read %s: %s\n", file, rerr)
			return 1
		}
		if _, runErr := a.Run(string(src)); runErr != nil {
			fmt.Fprintf(stderr, "debug serve: run %s: %s\n", file, runErr)
			return 1
		}
	}

	srv := debugserve.NewServer(a.NativeRegistry(), *token)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Advertise for `attach`. Best-effort; removed on shutdown.
	dp := discoveryPath()
	writeDiscovery(dp, *bind, *token)
	defer func() { _ = os.Remove(dp) }()

	fmt.Fprintf(stdout, "aql debug: serving introspection on http://%s (token: %v)\n", *bind, *token != "")
	if err := srv.ListenAndServe(ctx, *bind, *allowPublic); err != nil {
		fmt.Fprintf(stderr, "debug serve: %s\n", err)
		return 1
	}
	return 0
}

func writeDiscovery(path, bind, token string) {
	d := discovery{URL: "http://" + bind, Token: token, PID: os.Getpid()}
	data, err := jsonMarshal(d)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func readDiscovery(path string) (url, token string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var d discovery
	if err := json.Unmarshal(data, &d); err != nil {
		return "", "", err
	}
	return d.URL, d.Token, nil
}

func runAttach(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("url", "", "debug server URL (default: read from discovery file)")
	token := fs.String("token", "", "Bearer token (default: read from discovery file)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *url == "" {
		u, t, err := readDiscovery(discoveryPath())
		if err != nil {
			fmt.Fprintf(stderr, "debug attach: no --url and no discovery file: %s\n", err)
			return 1
		}
		*url = u
		if *token == "" {
			*token = t
		}
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "usage: aql debug attach [--url U] [--token T] <words|defs|heap|eval SRC|events ID>")
		return 1
	}
	c := debugserve.NewClient(*url, *token)
	return attachVerb(c, rest, stdout, stderr)
}

// attachVerb dispatches one attach query. Split out so it is unit-testable
// against an in-process httptest server.
func attachVerb(c *debugserve.Client, rest []string, stdout, stderr io.Writer) int {
	switch rest[0] {
	case "words":
		words, err := c.Words()
		if err != nil {
			return fail(stderr, err)
		}
		for _, w := range words {
			fmt.Fprintln(stdout, w)
		}
	case "defs":
		defs, err := c.Defs()
		if err != nil {
			return fail(stderr, err)
		}
		names := make([]string, 0, len(defs))
		for n := range defs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(stdout, "%s = %s\n", n, defs[n])
		}
	case "heap":
		h, err := c.Heap()
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "alloc=%d total-alloc=%d heap-objects=%d num-gc=%d\n",
			h.Alloc, h.TotalAlloc, h.HeapObjects, h.NumGC)
	case "eval":
		src := strings.Join(rest[1:], " ")
		result, evalErr, err := c.Eval(src)
		if err != nil {
			return fail(stderr, err)
		}
		if evalErr != "" {
			fmt.Fprintf(stderr, "eval error: %s\n", evalErr)
			return 1
		}
		fmt.Fprintln(stdout, result)
	case "events":
		if len(rest) < 2 {
			fmt.Fprintln(stderr, "usage: aql debug attach events <invocation-id>")
			return 1
		}
		evs, err := c.Events(rest[1])
		if err != nil {
			return fail(stderr, err)
		}
		for _, e := range evs {
			fmt.Fprintf(stdout, "[%d] %s = %s\n", e.Seq, e.Label, e.Value)
		}
	default:
		fmt.Fprintf(stderr, "aql debug attach: unknown query %q\n", rest[0])
		return 1
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "debug attach: %s\n", err)
	return 1
}
