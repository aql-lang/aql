package debugserve

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// attachHarness stands up a debugserve.Server over a fresh registry on an
// httptest.Server (real HTTP, no real socket bind) and returns a Client
// attached to it — a genuine cross-process round trip in-test.
func attachHarness(t *testing.T, token string) (*native.Registry, *Client, func()) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	ts := httptest.NewServer(NewServer(reg, token).Handler())
	return reg, NewClient(ts.URL, token), ts.Close
}

func TestAttachWordsAndEval(t *testing.T) {
	reg, c, closeFn := attachHarness(t, "")
	defer closeFn()

	// A def made on the server is observable through the attached client.
	if _, err := native.NewTop(reg).Run(mustParse(t, "def remote-secret 1234")); err != nil {
		t.Fatal(err)
	}

	words, err := c.Words()
	if err != nil || len(words) == 0 {
		t.Fatalf("Words(): %v (n=%d)", err, len(words))
	}

	defs, err := c.Defs()
	if err != nil {
		t.Fatalf("Defs(): %v", err)
	}
	if defs["remote-secret"] != "1234" {
		t.Errorf("attached client should see the server's def; got %q", defs["remote-secret"])
	}

	// Remote eval runs ON the attached registry, so it sees remote-secret.
	result, evalErr, err := c.Eval("remote-secret 1 add")
	if err != nil {
		t.Fatalf("Eval transport error: %v", err)
	}
	if evalErr != "" {
		t.Fatalf("Eval runtime error: %s", evalErr)
	}
	if result != "1235" {
		t.Errorf("remote eval = %q, want 1235", result)
	}
}

func TestAttachEvalReportsRuntimeError(t *testing.T) {
	_, c, closeFn := attachHarness(t, "")
	defer closeFn()
	// A runtime error in evaluated source comes back as evalErr, not a
	// transport error — the attach contract distinguishes the two.
	_, evalErr, err := c.Eval("1 div 0")
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if evalErr == "" {
		t.Error("dividing by zero should surface as a runtime eval error")
	}
}

func TestAttachHeap(t *testing.T) {
	_, c, closeFn := attachHarness(t, "")
	defer closeFn()
	h, err := c.Heap()
	if err != nil {
		t.Fatalf("Heap(): %v", err)
	}
	if h.Alloc == 0 {
		t.Error("heap alloc should be non-zero")
	}
}

func TestAttachBearerAuth(t *testing.T) {
	_, c, closeFn := attachHarness(t, "s3cret")
	defer closeFn()
	// Correct token works.
	if _, err := c.Words(); err != nil {
		t.Errorf("authorized request should succeed: %v", err)
	}
	// Wrong token is rejected (401).
	bad := NewClient(c.baseURL, "wrong")
	if _, err := bad.Words(); err == nil {
		t.Error("a wrong bearer token must be rejected")
	}
	// Missing token is rejected.
	none := NewClient(c.baseURL, "")
	if _, err := none.Words(); err == nil {
		t.Error("a missing bearer token must be rejected when one is required")
	}
}

func TestServerlessChannelEmitAndInterrogate(t *testing.T) {
	_, c, closeFn := attachHarness(t, "")
	defer closeFn()

	// The "function" emits events for its invocation id; the "interrogator"
	// reads them back by id — the §7.3 relay, out-of-band and post-hoc.
	const inv = "req-abc"
	if err := c.Emit(inv, Event{Label: "input", Value: "42", UnixSec: 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.Emit(inv, Event{Label: "result", Value: "84", UnixSec: 2}); err != nil {
		t.Fatal(err)
	}
	// A different invocation's channel is isolated.
	if err := c.Emit("other", Event{Label: "x", Value: "1"}); err != nil {
		t.Fatal(err)
	}

	evs, err := c.Events(inv)
	if err != nil {
		t.Fatalf("Events(): %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 events for %s, got %d", inv, len(evs))
	}
	if evs[0].Label != "input" || evs[1].Value != "84" {
		t.Errorf("events out of order/wrong: %+v", evs)
	}
	if evs[0].Seq != 0 || evs[1].Seq != 1 {
		t.Errorf("events should be sequenced 0,1; got %d,%d", evs[0].Seq, evs[1].Seq)
	}
}

func TestListenAndServeRefusesPublicByDefault(t *testing.T) {
	reg, _ := native.DefaultRegistry()
	s := NewServer(reg, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A non-loopback bind without allowPublic must be refused loudly.
	err := s.ListenAndServe(ctx, "0.0.0.0:0", false)
	if err == nil {
		t.Error("non-loopback bind without allowPublic must be refused")
	}
}

func TestListenAndServeLoopbackRoundTrip(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	s := NewServer(reg, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- s.ListenAndServe(ctx, "127.0.0.1:0", false) }()
	// The loopback path is accepted (no immediate refusal); cancel shuts it
	// down cleanly with a nil error.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("loopback serve should shut down cleanly, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down within 2s of ctx cancel")
	}
}

func mustParse(t *testing.T, src string) []native.Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return toks
}
