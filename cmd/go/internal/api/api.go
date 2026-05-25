// Package api implements the `api` service: a REST HTTP server that
// exposes the supervisor's state and accepts state-transition
// actions on managed services. The contract is defined in
// openapi.yaml, embedded into the binary and served at
// /openapi.yaml.
//
// Routes (all under /v1 except where noted):
//
//	GET    /v1/server                    supervisor info
//	GET    /v1/services                  list services
//	GET    /v1/services/{name}           one service
//	POST   /v1/services/{name}/actions   {action: pause|resume|stop}
//	GET    /openapi.yaml                 contract
//	GET    /healthz                      liveness (no auth)
//
// Auth: optional bearer token via --token. With no token, requests
// are accepted unauthenticated (default bind is 127.0.0.1 so the
// trust boundary is the loopback interface).
//
// The api service writes a discovery file at $TMPDIR/aql-api.json
// (mode 0600) containing {url, token, pid} so ctl/tui can find it
// without arguments. The file is removed on Stop.
package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

//go:embed openapi.yaml
var openAPISpec []byte

// Server is the lifecycle-managed api HTTP server.
type Server struct {
	bind   string
	token  string
	stderr io.Writer

	insp  service.Inspector
	ln    net.Listener
	srv   *http.Server
	state atomic.Int32

	mu           sync.Mutex
	startTime    time.Time
	discoverPath string
}

// NewServer constructs an api server bound to bind ("host:port"). If
// token is non-empty, requests must carry a matching Bearer token.
func NewServer(bind, token string, stderr io.Writer) *Server {
	s := &Server{
		bind:   bind,
		token:  token,
		stderr: stderr,
	}
	s.state.Store(int32(service.StateStopped))
	return s
}

// Name returns "api".
func (s *Server) Name() string { return "api" }

// Status returns the current lifecycle state.
func (s *Server) Status() service.State { return service.State(s.state.Load()) }

// UsesStdio always returns false.
func (s *Server) UsesStdio() bool { return false }

// Bind receives the supervisor's Inspector before Start. Required for
// /v1/services to return real data.
func (s *Server) Bind(insp service.Inspector) {
	s.insp = insp
}

// Addr returns the bound listen address (resolved after Start).
func (s *Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.bind
}

// Metadata returns the api server's runtime info.
func (s *Server) Metadata() map[string]string {
	m := map[string]string{"addr": s.Addr()}
	if s.token != "" {
		m["auth"] = "bearer"
	} else {
		m["auth"] = "none"
	}
	return m
}

// Start binds the listener and serves until ctx is canceled. Writes
// the discovery file on bind and removes it on shutdown.
func (s *Server) Start(ctx context.Context) error {
	if s.insp == nil {
		return errors.New("api: not bound to a supervisor (missing Bind call)")
	}
	s.state.Store(int32(service.StateStarting))

	ln, err := net.Listen("tcp", s.bind)
	if err != nil {
		s.state.Store(int32(service.StateStopped))
		return fmt.Errorf("api: listen %s: %w", s.bind, err)
	}
	s.ln = ln
	s.startTime = time.Now()
	s.srv = &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := s.writeDiscoveryFile(); err != nil {
		fmt.Fprintf(s.stderr, "api: discovery file: %s\n", err)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.srv.Shutdown(shutdownCtx)
		case <-done:
		}
	}()

	s.state.Store(int32(service.StateRunning))
	err = s.srv.Serve(ln)
	close(done)
	s.state.Store(int32(service.StateStopped))
	s.removeDiscoveryFile()

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop requests a graceful HTTP shutdown.
func (s *Server) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// compile-time interface checks.
var (
	_ service.Service         = (*Server)(nil)
	_ service.StdioUser       = (*Server)(nil)
	_ service.SupervisorBound = (*Server)(nil)
	_ service.WithMetadata    = (*Server)(nil)
)
