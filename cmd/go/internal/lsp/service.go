// service.go provides the Service-shaped wrapper around the LSP
// server: stdio mode (1 client, the calling editor) or TCP mode
// (1 client per accepted connection). Pause is intentionally not
// implemented — the LSP protocol assumes a continuously connected
// client.

package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// Server is the lifecycle-managed LSP server. Construct with
// NewStdioServer for stdio mode or NewTCPServer for TCP mode.
type Server struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	usesStdio bool

	tcpAddr string
	ln      net.Listener
	connMu  sync.Mutex
	conn    net.Conn // current accepted connection, if any

	state    atomic.Int32 // service.State
	exitCode atomic.Int32 // last LSP-protocol exit code (0 clean / 1 dirty)
}

// NewStdioServer constructs an LSP server that speaks the LSP base
// protocol over the given stdin/stdout (typically the process's own
// streams when invoked as a child of an editor).
func NewStdioServer(stdin io.Reader, stdout, stderr io.Writer) *Server {
	s := &Server{
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		usesStdio: true,
	}
	s.state.Store(int32(service.StateStopped))
	return s
}

// NewTCPServer constructs an LSP server that binds a TCP listener on
// the given port and serves the first accepted connection. Diagnostic
// logging goes to stderr; the listener stays open across the
// connection lifetime so the next client can reconnect after the
// current one closes.
func NewTCPServer(port int, stderr io.Writer) *Server {
	s := &Server{
		stderr:  stderr,
		tcpAddr: fmt.Sprintf(":%d", port),
	}
	s.state.Store(int32(service.StateStopped))
	return s
}

// Name returns "lsp".
func (s *Server) Name() string { return "lsp" }

// Status returns the current lifecycle state.
func (s *Server) Status() service.State {
	return service.State(s.state.Load())
}

// UsesStdio reports whether this server is bound to process stdio.
// True for stdio mode, false for TCP mode.
func (s *Server) UsesStdio() bool { return s.usesStdio }

// Addr returns the TCP listen address (empty for stdio mode).
func (s *Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.tcpAddr
}

// Start runs the LSP server until ctx is canceled or the underlying
// transport closes. In stdio mode it serves a single session and
// returns when stdin EOFs. In TCP mode it accepts one connection,
// serves it, returns when the connection closes (LSP servers are
// single-client by convention).
func (s *Server) Start(ctx context.Context) error {
	s.state.Store(int32(service.StateStarting))

	if s.usesStdio {
		s.state.Store(int32(service.StateRunning))
		defer s.state.Store(int32(service.StateStopped))

		// Close stdin on cancel so the blocking readMessage unblocks.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				if c, ok := s.stdin.(io.Closer); ok {
					_ = c.Close()
				}
			case <-done:
			}
		}()
		defer close(done)

		s.exitCode.Store(int32(newServer(s.stdin, s.stdout, s.stderr).run()))
		return nil
	}

	ln, err := net.Listen("tcp", s.tcpAddr)
	if err != nil {
		s.state.Store(int32(service.StateStopped))
		return fmt.Errorf("listen %s: %w", s.tcpAddr, err)
	}
	s.ln = ln
	defer func() {
		_ = ln.Close()
		s.state.Store(int32(service.StateStopped))
	}()

	fmt.Fprintf(s.stderr, "aql lsp listening on %s\n", ln.Addr().String())

	// Close the listener on cancel so Accept unblocks.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
			s.connMu.Lock()
			if s.conn != nil {
				_ = s.conn.Close()
			}
			s.connMu.Unlock()
		case <-done:
		}
	}()
	defer close(done)

	s.state.Store(int32(service.StateRunning))
	conn, err := ln.Accept()
	if err != nil {
		// ctx canceled or Stop closed the listener: clean exit.
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return fmt.Errorf("accept: %w", err)
	}
	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()
	defer conn.Close()

	s.exitCode.Store(int32(newServer(conn, conn, s.stderr).run()))
	return nil
}

// ExitCode returns the LSP protocol exit code from the last completed
// session: 0 if the shutdown/exit handshake was followed, 1 otherwise
// (LSP convention). Meaningful only after Start has returned.
func (s *Server) ExitCode() int { return int(s.exitCode.Load()) }

// Stop closes the listener and active connection, which makes Start
// return promptly.
func (s *Server) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.connMu.Lock()
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.connMu.Unlock()
	if s.usesStdio {
		if c, ok := s.stdin.(io.Closer); ok {
			_ = c.Close()
		}
	}
	return nil
}

// Metadata returns the LSP server's observable runtime state.
func (s *Server) Metadata() map[string]string {
	mode := "tcp"
	if s.usesStdio {
		mode = "stdio"
	}
	m := map[string]string{"mode": mode}
	if !s.usesStdio {
		m["addr"] = s.Addr()
	}
	return m
}

// compile-time interface checks. Note: Pausable is deliberately not
// implemented (see file header).
var (
	_ service.Service      = (*Server)(nil)
	_ service.StdioUser    = (*Server)(nil)
	_ service.WithMetadata = (*Server)(nil)
)
