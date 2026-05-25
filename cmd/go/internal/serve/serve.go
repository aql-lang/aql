// Package serve implements `aql serve <svc> [flags] + <svc> [flags] ...`:
// the umbrella command that supervises multiple AQL services in one
// process. Each segment between `+` tokens is parsed by the named
// service's factory, then all services run concurrently under a
// single SIGINT/SIGTERM-driven graceful shutdown.
//
// A Unix-socket control plane can be enabled with --ctl=<path>, which
// is what `aql ctl` connects to for ad-hoc status/pause/resume/stop.
//
// Stdio conflicts (two services that both want stdin/stdout) are
// rejected up front rather than allowed to interleave.
package serve

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/service"
)

type cmd struct{}

// New returns the serve subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "serve" }
func (*cmd) Synopsis() string { return "run one or more services in one process" }

// Run handles `aql serve [--ctl[=path]] [-c file] <svc> [flags] + <svc> [flags] ...`.
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Pull the serve-level flags off the head of argv before we
	// hand the rest to splitSegments. We use a manual walk because
	// the flag package would consume the service segments too.
	var ctlEnabled bool
	var ctlPath string
	var configPath string
	tail := args
	for len(tail) > 0 {
		a := tail[0]
		switch {
		case a == "--ctl":
			ctlEnabled = true
			tail = tail[1:]
		case len(a) > len("--ctl=") && a[:len("--ctl=")] == "--ctl=":
			ctlEnabled = true
			ctlPath = a[len("--ctl="):]
			tail = tail[1:]
		case a == "-c" || a == "--config":
			if len(tail) < 2 {
				fmt.Fprintf(stderr, "serve: %s requires a path\n", a)
				return 1
			}
			configPath = tail[1]
			tail = tail[2:]
		case len(a) > len("--config=") && a[:len("--config=")] == "--config=":
			configPath = a[len("--config="):]
			tail = tail[1:]
		case a == "-h" || a == "--help":
			printUsage(stdout)
			return 0
		default:
			// First non-flag token: rest is segment grammar.
			goto parseSegments
		}
	}
parseSegments:

	var segments [][]string
	if configPath != "" {
		loaded, err := loadConfig(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "serve: %s\n", err)
			return 1
		}
		segments = loaded
		if len(tail) > 0 {
			fmt.Fprintf(stderr, "serve: -c is exclusive with inline service segments\n")
			return 1
		}
	} else {
		segments = splitSegments(tail)
	}

	if len(segments) == 0 {
		fmt.Fprintf(stderr, "serve: at least one service segment is required\n")
		printUsage(stderr)
		return 1
	}

	services, err := buildServices(segments, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "serve: %s\n", err)
		return 1
	}

	if err := checkStdioConflict(services); err != nil {
		fmt.Fprintf(stderr, "serve: %s\n", err)
		return 1
	}

	if err := checkDuplicateNames(services); err != nil {
		fmt.Fprintf(stderr, "serve: %s\n", err)
		return 1
	}

	sup := newSupervisor(services, stdout, stderr)
	if ctlEnabled {
		if ctlPath == "" {
			ctlPath = defaultCtlPath()
		}
		if err := sup.startControlSocket(ctlPath); err != nil {
			fmt.Fprintf(stderr, "serve: ctl: %s\n", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return sup.run(ctx)
}

// buildServices runs each segment through its factory and returns
// the resulting Service instances in order.
func buildServices(segments [][]string, stdin io.Reader, stdout, stderr io.Writer) ([]service.Service, error) {
	out := make([]service.Service, 0, len(segments))
	for _, seg := range segments {
		name := seg[0]
		f, ok := factories[name]
		if !ok {
			return nil, fmt.Errorf("unknown service %q (known: %v)", name, factoryOrder)
		}
		svc, err := f(seg[1:], stdin, stdout, stderr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, svc)
	}
	return out, nil
}

// checkStdioConflict rejects combinations where more than one
// service wants to own stdin/stdout (e.g. `repl + lsp` without
// `-p` on lsp).
func checkStdioConflict(svcs []service.Service) error {
	var users []string
	for _, s := range svcs {
		if u, ok := s.(service.StdioUser); ok && u.UsesStdio() {
			users = append(users, s.Name())
		}
	}
	if len(users) > 1 {
		sort.Strings(users)
		return fmt.Errorf("stdio conflict: %v all want stdin/stdout; give lsp a -p <port> or drop one", users)
	}
	return nil
}

// checkDuplicateNames rejects argv like `lsp -p 9000 + lsp -p 9001`.
// Same service name twice would collide in the ctl namespace and
// confuse status output; if you really want two instances, write a
// config file with explicit aliases (future work).
func checkDuplicateNames(svcs []service.Service) error {
	seen := make(map[string]bool)
	for _, s := range svcs {
		if seen[s.Name()] {
			return fmt.Errorf("service %q listed twice; only one instance per name", s.Name())
		}
		seen[s.Name()] = true
	}
	return nil
}

// printUsage writes the serve subcommand help.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: aql serve [--ctl[=path]] <svc> [flags] [+ <svc> [flags]]...")
	fmt.Fprintln(w, "       aql serve [--ctl[=path]] -c <file>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Compose multiple services in one process under a single supervisor.")
	fmt.Fprintln(w, "Segments are separated by a bare '+' token; each segment uses the")
	fmt.Fprintln(w, "named service's own flag set.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Services:")
	for _, name := range factoryOrder {
		fmt.Fprintf(w, "  %s\n", name)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --ctl[=path]   open the control socket (default: $TMPDIR/aql-serve.sock)")
	fmt.Fprintln(w, "  -c <file>      load service list from a jsonic config file")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  aql serve registry -r ./mods -p 8080 + lsp -p 9000")
	fmt.Fprintln(w, "  aql serve --ctl repl + registry -r ./mods")
	fmt.Fprintln(w, "  aql serve -c services.jsonic")
}

// supervisor owns the running services and the optional control
// socket. One supervisor per `aql serve` invocation.
type supervisor struct {
	services []service.Service
	byName   map[string]service.Service
	stdout   io.Writer
	stderr   io.Writer

	mu        sync.Mutex
	cancels   map[string]context.CancelFunc
	ctlServer *controlServer // nil if --ctl not given
	wg        sync.WaitGroup
}

func newSupervisor(svcs []service.Service, stdout, stderr io.Writer) *supervisor {
	byName := make(map[string]service.Service, len(svcs))
	for _, s := range svcs {
		byName[s.Name()] = s
	}
	return &supervisor{
		services: svcs,
		byName:   byName,
		stdout:   stdout,
		stderr:   stderr,
		cancels:  make(map[string]context.CancelFunc),
	}
}

// run launches every service in a goroutine, waits for ctx to cancel
// (signal received) or all services to exit on their own, then drives
// graceful shutdown. Returns 0 on clean shutdown, 1 if any service
// returned an error.
func (sup *supervisor) run(ctx context.Context) int {
	allDone := make(chan struct{})
	var hadErr atomic.Bool

	for _, svc := range sup.services {
		svc := svc
		sctx, cancel := context.WithCancel(ctx)
		sup.mu.Lock()
		sup.cancels[svc.Name()] = cancel
		sup.mu.Unlock()
		sup.wg.Add(1)
		go func() {
			defer sup.wg.Done()
			if err := svc.Start(sctx); err != nil && err != context.Canceled {
				fmt.Fprintf(sup.stderr, "%s: %s\n", svc.Name(), err)
				hadErr.Store(true)
			}
		}()
	}

	go func() {
		sup.wg.Wait()
		close(allDone)
	}()

	// Print a brief startup line so the user knows what's running.
	names := make([]string, 0, len(sup.services))
	for _, s := range sup.services {
		names = append(names, s.Name())
	}
	fmt.Fprintf(sup.stdout, "aql serve: running %v\n", names)
	if sup.ctlServer != nil {
		fmt.Fprintf(sup.stdout, "aql serve: ctl on %s\n", sup.ctlServer.path)
	}

	select {
	case <-ctx.Done():
		// Signal received: cancel all per-service contexts, then
		// wait for the services to unwind.
		sup.mu.Lock()
		for _, c := range sup.cancels {
			c()
		}
		sup.mu.Unlock()
		<-allDone
	case <-allDone:
		// All services exited on their own.
	}

	if sup.ctlServer != nil {
		_ = sup.ctlServer.close()
	}

	if hadErr.Load() {
		return 1
	}
	return 0
}
