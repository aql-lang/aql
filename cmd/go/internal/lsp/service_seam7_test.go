package lsp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestS7n_TCPCancelAfterAcceptClosesConnection drives the ctx.Done()
// cancel goroutine's s.conn != nil branch inside Start (service.go):
// once a connection has been accepted, canceling ctx directly (not
// via Stop) must close that live connection too.
func TestS7n_TCPCancelAfterAcceptClosesConnection(t *testing.T) {
	stderr := &syncBuffer{}
	srv := NewTCPServer("127.0.0.1", 0, stderr)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	addr := waitListenAddr(t, stderr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %s", addr, err)
	}
	defer conn.Close()

	// Complete one round-trip so s.conn is definitely assigned before
	// we cancel, exercising the "conn != nil" branch rather than racing
	// past it.
	br := bufio.NewReader(conn)
	if _, err := conn.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})); err != nil {
		t.Fatalf("write initialize: %s", err)
	}
	if _, err := readFramedMessage(br); err != nil {
		t.Fatalf("read initialize response: %s", err)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after ctx cancel with an active connection = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel with an active connection")
	}
}

// fakeListener is a net.Listener whose Accept always returns a fixed
// error, used to drive Start's "accept failed for a reason other than
// ctx-cancel or a closed listener" arm — a real listener only ever
// fails that way for ctx-cancel/Stop (both produce net.ErrClosed), so
// this needs a seam.
type fakeListener struct {
	err error
}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, f.err }
func (f *fakeListener) Close() error              { return nil }
func (f *fakeListener) Addr() net.Addr            { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "tcp" }
func (fakeAddr) String() string  { return "fake:0" }

// TestS7n_TCPAcceptOtherErrorSurfaced swaps the netListen seam so
// ln.Accept() fails with an error that is neither ctx-cancellation nor
// net.ErrClosed; Start must surface it wrapped as "accept: ...".
func TestS7n_TCPAcceptOtherErrorSurfaced(t *testing.T) {
	orig := netListen
	t.Cleanup(func() { netListen = orig })
	netListen = func(network, addr string) (net.Listener, error) {
		return &fakeListener{err: errors.New("boom-accept")}, nil
	}

	srv := NewTCPServer("127.0.0.1", 0, io.Discard)
	err := srv.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "accept: boom-accept") {
		t.Errorf("Start() = %v, want an \"accept: boom-accept\" error", err)
	}
}
