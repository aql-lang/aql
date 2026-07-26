package vault

// proxy_stream_test.go — the broker's streaming pass-through: each
// upstream chunk must be flushed to the client as it arrives (the SSE
// token-stream shape), not parked in the response buffer until the
// handler returns.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
// writer is wrapped and flushed once per non-empty write, an empty
// write does not flush, and a writer that cannot flush is returned
// unwrapped rather than losing the ability to write at all.
func TestFlushingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	fw := flushingWriter(rec)
	if _, ok := fw.(*flushWriter); !ok {
		t.Fatalf("a Flusher-capable writer should be wrapped, got %T", fw)
	}
	if _, err := fw.Write(nil); err != nil {
		t.Fatal(err)
	}
	if rec.Flushed {
		t.Error("an empty write must not flush")
	}
	if _, err := fw.Write([]byte("data: x\n\n")); err != nil {
		t.Fatal(err)
	}
	if !rec.Flushed {
		t.Error("a non-empty write must flush")
	}
	if rec.Body.String() != "data: x\n\n" {
		t.Errorf("body = %q", rec.Body.String())
	}

	nf := &nonFlushingRW{hdr: http.Header{}}
	if got := flushingWriter(nf); got != io.Writer(nf) {
		t.Errorf("a non-Flusher must be returned unwrapped, got %T", got)
	}
}
