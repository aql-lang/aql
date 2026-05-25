// service.go provides the Service-shaped wrapper around the vault
// proxy so it can be composed under `aql serve` and controlled via
// the api service. The pre-existing Proxy type (and its ServeHTTP
// method) is reused unchanged — this file only adds lifecycle, a
// pause gate, and the metadata accessor.

package vault

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// ProxyService is the lifecycle-managed wrapper around Proxy.
type ProxyService struct {
	proxy *Proxy

	mu     sync.Mutex
	ln     net.Listener
	srv    *http.Server
	paused atomic.Bool
	state  atomic.Int32 // service.State
	addr   string       // resolved bound address after Start
}

// NewProxyService constructs a vault-proxy Service ready to Start.
// homeDir defaults to the standard $HOME (honoring AQL_HOME) if
// empty. defaultPass is the file-keyring passphrase, typically read
// from AQL_VAULT_PASSPHRASE; pass "" to skip.
func NewProxyService(listen, homeDirArg, defaultPass string) (*ProxyService, error) {
	if listen == "" {
		listen = "127.0.0.1:8787"
	}
	if homeDirArg == "" {
		h, err := homeDir()
		if err != nil {
			return nil, fmt.Errorf("vault-proxy: %w", err)
		}
		homeDirArg = h
	}
	s, err := requireStore(homeDirArg)
	if err != nil {
		return nil, fmt.Errorf("vault-proxy: %w", err)
	}
	if s.Locked {
		return nil, errors.New("vault-proxy: vault is locked; run `aql vault unlock`")
	}

	ps := &ProxyService{
		proxy: NewProxy(listen, homeDirArg, defaultPass, nopWriter{}, nopWriter{}),
	}
	ps.state.Store(int32(service.StateStopped))
	return ps, nil
}

// Name returns "vault-proxy".
func (s *ProxyService) Name() string { return "vault-proxy" }

// Status returns the lifecycle state.
func (s *ProxyService) Status() service.State { return service.State(s.state.Load()) }

// UsesStdio is always false: the proxy speaks HTTP.
func (s *ProxyService) UsesStdio() bool { return false }

// Addr returns the bound listen address (resolved after Start).
func (s *ProxyService) Addr() string {
	if s.addr != "" {
		return s.addr
	}
	return s.proxy.listen
}

// Metadata surfaces the proxy's runtime config for the api service.
func (s *ProxyService) Metadata() map[string]string {
	return map[string]string{
		"addr": s.Addr(),
		"home": s.proxy.homeDir,
	}
}

// Start binds the listener and serves requests until ctx is canceled.
func (s *ProxyService) Start(ctx context.Context) error {
	s.state.Store(int32(service.StateStarting))

	ln, err := net.Listen("tcp", s.proxy.listen)
	if err != nil {
		s.state.Store(int32(service.StateStopped))
		return fmt.Errorf("vault-proxy: listen %s: %w", s.proxy.listen, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.addr = ln.Addr().String()
	s.srv = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Unlock()

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

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop requests a graceful HTTP shutdown.
func (s *ProxyService) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Pause swaps the live handler for a 503 so callers get a clear
// "revoked" signal during emergency response; existing connections
// stay open so they don't get connection-refused errors.
func (s *ProxyService) Pause(_ context.Context) error {
	s.paused.Store(true)
	s.state.Store(int32(service.StatePaused))
	return nil
}

// Resume restores normal request handling.
func (s *ProxyService) Resume(_ context.Context) error {
	s.paused.Store(false)
	s.state.Store(int32(service.StateRunning))
	return nil
}

// serveHTTP is the always-installed handler: short-circuits to 503
// while paused, otherwise delegates to the underlying Proxy.
func (s *ProxyService) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if s.paused.Load() {
		http.Error(w, "vault proxy paused (revoked)", http.StatusServiceUnavailable)
		return
	}
	s.proxy.ServeHTTP(w, r)
}

// nopWriter discards writes; used so the underlying Proxy doesn't
// chatter on stdout/stderr when run as a Service (output goes via
// the supervisor's stdout/stderr instead).
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// compile-time interface checks.
var (
	_ service.Service      = (*ProxyService)(nil)
	_ service.Pausable     = (*ProxyService)(nil)
	_ service.StdioUser    = (*ProxyService)(nil)
	_ service.WithMetadata = (*ProxyService)(nil)
)
