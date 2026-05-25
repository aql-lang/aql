// service.go provides the Service-shaped wrapper around the REPL
// loop: lifecycle-managed start/stop and pause/resume control. The
// pre-existing Start() function in repl.go is reused by the standalone
// CLI path and delegates to the Service.Start internals.

package repl

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// Server is the lifecycle-managed REPL. Construct with NewServer,
// then call Start. Pause stops processing input (the line buffer
// silently accumulates and is dropped on Resume); Resume restores
// normal eval.
type Server struct {
	in           io.Reader
	out          io.Writer
	registryPath string

	state  atomic.Int32 // service.State
	paused atomic.Bool
	cancel context.CancelFunc
}

// NewServer constructs a REPL server bound to in/out. registryPath
// is the module-registry path forwarded to the UniversalManager
// (empty means use the user default).
func NewServer(in io.Reader, out io.Writer, registryPath string) *Server {
	s := &Server{
		in:           in,
		out:          out,
		registryPath: registryPath,
	}
	s.state.Store(int32(service.StateStopped))
	return s
}

// Name returns "repl".
func (s *Server) Name() string { return "repl" }

// Status returns the current lifecycle state.
func (s *Server) Status() service.State {
	return service.State(s.state.Load())
}

// UsesStdio always returns true: the REPL reads/writes the streams
// it was constructed with, which for `aql repl` standalone are the
// process's own stdin/stdout.
func (s *Server) UsesStdio() bool { return true }

// Start runs the REPL loop until ctx is canceled or the input EOFs.
// Returns nil on EOF or cancel.
func (s *Server) Start(ctx context.Context) error {
	s.state.Store(int32(service.StateStarting))

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	// Close stdin on cancel so the blocking Readline unblocks. The
	// REPL's readline loop returns on Read error and Start unwinds.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if c, ok := s.in.(io.Closer); ok {
				_ = c.Close()
			}
		case <-done:
		}
	}()
	defer close(done)

	s.state.Store(int32(service.StateRunning))
	startWithPauseGate(s.in, s.out, s.registryPath, &s.paused)
	s.state.Store(int32(service.StateStopped))
	return nil
}

// Stop cancels the context driving Start, which closes the input
// stream and lets the readline loop unwind.
func (s *Server) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// Pause stops the REPL from evaluating new lines. The readline loop
// keeps reading (so the prompt is still responsive), but each entered
// line is discarded with a "paused" notice. Resume returns to normal.
func (s *Server) Pause(ctx context.Context) error {
	s.paused.Store(true)
	s.state.Store(int32(service.StatePaused))
	return nil
}

// Resume re-enables evaluation.
func (s *Server) Resume(ctx context.Context) error {
	s.paused.Store(false)
	s.state.Store(int32(service.StateRunning))
	return nil
}

// Metadata returns observable REPL state for the api service.
func (s *Server) Metadata() map[string]string {
	m := map[string]string{}
	if s.registryPath != "" {
		m["registry"] = s.registryPath
	}
	return m
}

// compile-time interface checks.
var (
	_ service.Service      = (*Server)(nil)
	_ service.Pausable     = (*Server)(nil)
	_ service.StdioUser    = (*Server)(nil)
	_ service.WithMetadata = (*Server)(nil)
)
