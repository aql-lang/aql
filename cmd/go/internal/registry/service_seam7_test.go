package registry

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// TestS7n_ServeReturnsNonShutdownError drives Start's final "return
// err" arm (service.go): closing the bound listener directly
// (bypassing Stop/Shutdown) makes http.Server.Serve return the raw
// "use of closed network connection" error rather than
// http.ErrServerClosed, since the doneChan Shutdown sets is never
// triggered. That path is otherwise unreachable through the public
// Stop/ctx-cancel surface, which both go through srv.Shutdown.
func TestS7n_ServeReturnsNonShutdownError(t *testing.T) {
	srv, err := NewServer(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	deadline := time.Now().Add(5 * time.Second)
	for srv.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach running state")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Reach into the unexported listener and close it directly — this
	// bypasses srv.srv.Shutdown, so Serve's Accept loop sees a closed
	// connection without the server's doneChan ever being closed.
	srv.mu.Lock()
	ln := srv.ln
	srv.mu.Unlock()
	if ln == nil {
		t.Fatal("listener not bound")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("closing the listener directly: %s", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Start() = nil, want the raw (non-ErrServerClosed) accept error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after the listener was closed directly")
	}
}

// TestS7n_StopWithCanceledContextPropagatesError drives Stop's
// "if err := s.srv.Shutdown(ctx); err != nil { return err }" arm.
// http.Server.Shutdown closes the listeners unconditionally and only
// then polls for quiescence; with zero active connections that first
// poll already succeeds and Shutdown returns nil without ever
// checking ctx. So this needs one connection open (and not yet idle)
// at Stop time to force Shutdown into its ctx-aware wait loop, where
// an already-canceled context returns ctx.Err() immediately.
func TestS7n_StopWithCanceledContextPropagatesError(t *testing.T) {
	srv, err := NewServer(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(context.Background()) }()

	deadline := time.Now().Add(5 * time.Second)
	for srv.Addr() == ":0" || srv.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach running state")
		}
		time.Sleep(5 * time.Millisecond)
	}

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial %s: %s", srv.Addr(), err)
	}
	defer conn.Close()
	// Give the server's Accept goroutine a moment to register the new
	// connection's ConnState before we call Stop.
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before Stop is even called

	if err := srv.Stop(ctx); err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("Stop(canceled ctx) = %v, want context.Canceled", err)
	}

	// Clean up: give the server a real chance to shut down so the
	// Start goroutine doesn't leak past the test.
	conn.Close()
	_ = srv.Stop(context.Background())
	select {
	case <-startErr:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return during cleanup")
	}
}
