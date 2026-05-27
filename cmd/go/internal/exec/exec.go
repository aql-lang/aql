// Package exec is the aql exec HTTP server subcommand —
// `aql exec -p <port>`.
//
// The server exposes a REST API for executing AQL source code. Each
// request creates a fresh AQL instance so requests are stateless and
// safe to handle concurrently (the underlying lang.AQL instance is
// not safe for concurrent use).
//
// Routes:
//
//	POST /v1/exec     run AQL code; returns last stack value as result
//	GET  /healthz     liveness probe
package exec

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/permsflags"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

// cmd is the Command implementation for `aql exec`.
type cmd struct{}

// New returns the exec subcommand.
func New() command.Command { return &cmd{} }

// Name returns "exec".
func (*cmd) Name() string { return "exec" }

// Synopsis returns the one-line help text.
func (*cmd) Synopsis() string {
	return "serve AQL code execution over HTTP"
}

// Run handles `aql exec -p <port> [-r <registry>]`.
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr)
}

// run parses flags and drives the Server lifecycle with a
// SIGINT/SIGTERM-driven graceful shutdown.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(stderr)

	bind := fs.String("bind", "127.0.0.1:8091", "host:port to bind the exec HTTP server")
	port := fs.Int("p", 0, "port to listen on (overrides -bind host:port if >0)")
	registry := fs.String("r", "", "registry path passed to AQL instances")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	addr := *bind
	if *port > 0 {
		addr = fmt.Sprintf(":%d", *port)
	}

	srv, err := NewServer(addr, *registry, pol)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimPrefix(err.Error(), "exec: "))
		return 1
	}

	fmt.Fprintf(stdout, "aql exec serving on %s\n", srv.Addr())
	if pol != nil {
		fmt.Fprintf(stdout, "aql exec policy: %s\n", pol.Name())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// execRequest is the JSON body accepted by POST /v1/exec.
type execRequest struct {
	Code string `json:"code"`
}

// execResponse is the JSON body returned by POST /v1/exec. Result
// is the last value on the stack (null when the stack is empty);
// Stack is the full residual stack (empty when there is nothing
// left).
type execResponse struct {
	Result any    `json:"result"`
	Stack  []any  `json:"stack"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Handler builds the http.Handler for the exec service. registry is
// forwarded to each AQL instance so user code can `use` modules from
// a local directory. pol (may be nil) is the policy applied to every
// per-request AQL instance — it is bound at server construction and
// cannot be overridden by request bodies. Exposed so tests can spin
// up an httptest server.
func Handler(registry string, pol policy.Policy) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1/exec", func(w http.ResponseWriter, r *http.Request) {
		handleExec(registry, pol, w, r)
	})

	return mux
}

// handleExec runs the submitted code in a fresh AQL instance and
// writes the JSON response. Errors are reported in the response body
// with HTTP 200 so clients can distinguish transport errors from AQL
// errors (parse / type / runtime).
func handleExec(registry string, pol policy.Policy, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, execResponse{Error: "invalid request body: " + err.Error()})
		return
	}
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, execResponse{Error: "code is required"})
		return
	}

	a, err := lang.New(lang.Options{Registry: registry, Policy: pol})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, execResponse{Error: "init error: " + err.Error()})
		return
	}

	var outBuf strings.Builder
	a.SetOutput(&outBuf)

	stack, runErr := a.Run(req.Code)
	resp := execResponse{Output: outBuf.String()}
	if runErr != nil {
		resp.Error = runErr.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Stack = make([]any, len(stack))
	copy(resp.Stack, stack)
	if len(stack) > 0 {
		resp.Result = stack[len(stack)-1]
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeJSON serialises body as JSON and writes it with the given status.
func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
