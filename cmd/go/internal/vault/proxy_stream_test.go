package vault

// proxy_stream_test.go — the broker's streaming pass-through: each
// upstream chunk must be flushed to the client as it arrives (the SSE
// token-stream shape), not parked in the response buffer until the
// handler returns.

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestProxyFlushesStreamedChunks holds the upstream open after its
// first SSE chunk and asserts the chunk reaches the client while the
// upstream is still mid-response. Without the per-chunk flush the read
// would stall until the whole body completed — the regression this test
// exists to catch (it times out rather than deadlocks if that returns).
func TestProxyFlushesStreamedChunks(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake-stream", fu, "bearer")

	release := make(chan struct{})
	fu.respond = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release: // the client saw chunk one
		case <-r.Context().Done(): // client gave up — don't wedge Close
			return
		}
		_, _ = io.WriteString(w, "data: two\n\n")
	}

	if code, _, errOut := runVault(t, "sse-secret\n", "add",
		"--from-stdin", "--provider=fake-stream", "k"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	tok := grantOK(t, "k", nil, nil)
	base := startProxy(t)

	// The whole exchange runs under a deadline: if the proxy buffers
	// instead of flushing, the first read below blocks until the context
	// cancels the request and the Read fails — a clear failure, not a
	// wedged test.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/k/stream", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b := make([]byte, 256)
	n, err := resp.Body.Read(b)
	if err != nil {
		t.Fatalf("first SSE chunk was not flushed through the proxy while the upstream was still streaming: %v", err)
	}
	if !strings.Contains(string(b[:n]), "data: one") {
		t.Fatalf("first read = %q; want the first SSE chunk", b[:n])
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(rest), "data: two") {
		t.Errorf("tail of the stream lost: %q", rest)
	}
}

// nonFlushingRW is an http.ResponseWriter with no Flush method, so
// flushingWriter's cannot-flush arm is exercised.
type nonFlushingRW struct {
	hdr  http.Header
	body strings.Builder
}

func (n *nonFlushingRW) Header() http.Header         { return n.hdr }
func (n *nonFlushingRW) Write(p []byte) (int, error) { return n.body.Write(p) }
func (n *nonFlushingRW) WriteHeader(int)             {}

// TestFlushingWriter pins the wrapper's contract: a Flusher-capable
// writer is flushed once per non-empty write and on an explicit flush(),
// an empty write does not flush, and a writer that cannot flush yields a
// usable wrapper whose flush() and Write() are safe no-op-flush passes.
func TestFlushingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	fw := flushingWriter(rec)
	if _, err := fw.Write(nil); err != nil {
		t.Fatal(err)
	}
	if rec.Flushed {
		t.Error("an empty write must not flush")
	}
	fw.flush()
	if !rec.Flushed {
		t.Error("explicit flush() must flush a Flusher-capable writer")
	}
	rec.Flushed = false
	if _, err := fw.Write([]byte("data: x\n\n")); err != nil {
		t.Fatal(err)
	}
	if !rec.Flushed {
		t.Error("a non-empty write must flush")
	}
	if rec.Body.String() != "data: x\n\n" {
		t.Errorf("body = %q", rec.Body.String())
	}

	// A non-Flusher writer still yields a working wrapper; flush() and a
	// non-empty write are both safe (the flush is a no-op).
	nf := &nonFlushingRW{hdr: http.Header{}}
	nfw := flushingWriter(nf)
	nfw.flush()
	if _, err := nfw.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if nf.body.String() != "payload" {
		t.Errorf("non-flusher wrapper lost the write: %q", nf.body.String())
	}
}

// TestProxyFlushesHeadersForIdleStream: an upstream that opens the
// stream (200 + headers) then idles before its first event must not
// leave the client blocked awaiting headers — the proxy flushes the
// status line immediately after WriteHeader.
func TestProxyFlushesHeadersForIdleStream(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake-idle", fu, "bearer")

	release := make(chan struct{})
	fu.respond = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}
	if code, _, errOut := runVault(t, "idle-secret\n", "add",
		"--from-stdin", "--provider=fake-idle", "k"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	tok := grantOK(t, "k", nil, nil)
	base := startProxy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/k/stream", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	// Do returns once response headers arrive; if the proxy withheld them
	// until the (never-arriving) first body chunk, this blocks until the
	// context deadline and Do fails.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("headers were not flushed before the first body chunk: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}
	close(release)
}

// TestProxyLogsStreamInterrupted: when the upstream aborts the response
// after the status is committed, the copy errors and the broker records
// outcome=stream-interrupted rather than a clean ok.
func TestProxyLogsStreamInterrupted(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake-abort", fu, "bearer")

	fu.respond = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		// Abort the connection without a proper terminating chunk; the
		// proxy's read of the body then fails mid-stream.
		panic(http.ErrAbortHandler)
	}
	if code, _, errOut := runVault(t, "abort-secret\n", "add",
		"--from-stdin", "--provider=fake-abort", "k"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	tok := grantOK(t, "k", nil, nil)

	// Capture the proxy's access log to observe the outcome tag.
	var logBuf syncBuffer
	homeDir := os.Getenv(EnvHome)
	_ = home
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	p := NewProxy(addr, homeDir, os.Getenv(EnvPassphrase), &logBuf, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Serve(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitForProxy(t, addr)

	req, _ := http.NewRequest("GET", "http://"+addr+"/k/stream", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body) // drains until the aborted connection errors
	resp.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logBuf.String(), "outcome=stream-interrupted") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected outcome=stream-interrupted in the access log, got:\n%s", logBuf.String())
}

// syncBuffer is a goroutine-safe io.Writer for capturing the proxy's
// access log, which is written from the request goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForProxy blocks until addr accepts a connection or the test times
// out.
func waitForProxy(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond); err == nil {
			c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("proxy did not start in time")
}
