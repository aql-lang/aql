// Package debugcmd implements the `aql debug` subcommand: the user-facing
// front door to cross-process debugging (design/DEBUG-MODULE.0.md §7.2/§7.3).
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
// Transport is plain HTTP with an optional Bearer token (a static token or
// a vault capability id), the same posture as the api service. This is the
// host-level realization of the attach/serverless surfaces while the AQL
// Service model (SERVICES.0.md) and a language-level socket primitive are
// still RFC-only.
package debugcmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/debugserve"
)

type cmdImpl struct{}

// New returns the debug subcommand.
func New() command.Command { return &cmdImpl{} }

func (*cmdImpl) Name() string { return "debug" }
func (*cmdImpl) Synopsis() string {
	return "cross-process debugging: serve a runtime's introspection, or attach to one"
}

func (*cmdImpl) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: aql debug <serve|attach> ...")
		return 1
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "attach":
		return runAttach(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "aql debug: unknown subcommand %q (want serve|attach)\n", args[0])
		return 1
	}
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

	a, err := lang.New()
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
	data, err := json.Marshal(d)
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
