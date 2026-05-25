// Package tui implements the `tui` service: a bubbletea-based
// terminal UI that polls the api service and lets the user drive
// state transitions (pause/resume/stop) without leaving the
// terminal.
//
// The tui talks to the api over HTTP, so it does NOT need to be
// composed under the same supervisor — `aql tui` against an already-
// running supervisor is the common case. When run under a
// supervisor (via `aql serve ... + tui`) it takes over stdio and
// can't be combined with other stdio services.
package tui

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aql-lang/aql/cmd/go/internal/api"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/service"
)

type cmdImpl struct{}

// New returns the tui subcommand (callable directly via `aql tui`).
func New() command.Command { return &cmdImpl{} }

func (*cmdImpl) Name() string     { return "tui" }
func (*cmdImpl) Synopsis() string { return "interactive terminal UI driven by the api service" }

// Run handles `aql tui [--api url] [--token tok]`.
func (*cmdImpl) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	apiURL, token, err := parseFlags(args, stderr)
	if err != nil {
		return 1
	}
	if apiURL == "" || token == "" {
		url, tok, _, err := api.ReadDiscoveryFile()
		if err != nil {
			fmt.Fprintf(stderr, "tui: %s\n", err)
			return 1
		}
		if apiURL == "" {
			apiURL = url
		}
		if token == "" {
			token = tok
		}
	}

	cli := newAPIClient(apiURL, token)
	model := newModel(cli)
	prog := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout))
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(stderr, "tui: %s\n", err)
		return 1
	}
	return 0
}

// parseFlags shares its flag set with the Service factory.
func parseFlags(args []string, stderr io.Writer) (apiURL, token string, err error) {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	a := fs.String("api", "", "api base URL (default: read from discovery file)")
	t := fs.String("token", "", "bearer token (default: read from discovery file)")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	return *a, *t, nil
}

// Server is the lifecycle-managed wrapper for use under `aql serve`.
type Server struct {
	apiURL string
	token  string
	in     io.Reader
	out    io.Writer
	stderr io.Writer

	state  atomic.Int32
	cancel context.CancelFunc
	prog   *tea.Program
}

// NewServer constructs a tui Server. apiURL and token may be empty;
// if so the server reads them from the discovery file on Start.
func NewServer(apiURL, token string, in io.Reader, out, stderr io.Writer) *Server {
	s := &Server{
		apiURL: apiURL,
		token:  token,
		in:     in,
		out:    out,
		stderr: stderr,
	}
	s.state.Store(int32(service.StateStopped))
	return s
}

// Name returns "tui".
func (s *Server) Name() string { return "tui" }

// Status returns the lifecycle state.
func (s *Server) Status() service.State { return service.State(s.state.Load()) }

// UsesStdio is always true: the tui needs the terminal.
func (s *Server) UsesStdio() bool { return true }

// Metadata returns the configured api URL.
func (s *Server) Metadata() map[string]string {
	return map[string]string{"api": s.apiURL}
}

// Start runs the bubbletea program until quit or ctx cancel.
func (s *Server) Start(ctx context.Context) error {
	s.state.Store(int32(service.StateStarting))
	defer s.state.Store(int32(service.StateStopped))

	if s.apiURL == "" || s.token == "" {
		url, tok, _, err := api.ReadDiscoveryFile()
		if err != nil {
			return err
		}
		if s.apiURL == "" {
			s.apiURL = url
		}
		if s.token == "" {
			s.token = tok
		}
	}

	cli := newAPIClient(s.apiURL, s.token)
	model := newModel(cli)
	s.prog = tea.NewProgram(model, tea.WithInput(s.in), tea.WithOutput(s.out))

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	go func() {
		<-ctx.Done()
		if s.prog != nil {
			s.prog.Quit()
		}
	}()

	s.state.Store(int32(service.StateRunning))
	_, err := s.prog.Run()
	return err
}

// Stop signals the bubbletea program to quit.
func (s *Server) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	if s.prog != nil {
		s.prog.Quit()
	}
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// compile-time interface checks.
var (
	_ service.Service      = (*Server)(nil)
	_ service.StdioUser    = (*Server)(nil)
	_ service.WithMetadata = (*Server)(nil)
)
