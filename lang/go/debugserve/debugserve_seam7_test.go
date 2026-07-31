package debugserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
)

// s7dbgReg builds a fresh DefaultRegistry. When parse is true the parser is
// wired via SetParseFunc; when false the ParseFunc stays nil so the
// "parser not configured" eval arm is reachable.
func s7dbgReg(t *testing.T, parse bool) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if parse {
		reg.SetParseFunc(parser.Parse)
	}
	return reg
}

// s7dbgServer stands up a debugserve.Server (parser wired) on an
// httptest.Server and returns both so tests can reach unexported fields
// (maxEv) and issue raw requests. Cleanup closes the httptest.Server.
func s7dbgServer(t *testing.T, token string) (*Server, *httptest.Server) {
	t.Helper()
	srv := NewServer(s7dbgReg(t, true), token)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

// s7dbgErrReader is a request body whose Read always fails, driving the
// handleEval read-body error arm.
type s7dbgErrReader struct{}

func (s7dbgErrReader) Read([]byte) (int, error) { return 0, errors.New("boom read") }

// --- client.go NewRequest error arms (getJSON 97-99, postJSON 105-107) ---

func TestS7Dbg_ClientBadURL(t *testing.T) {
	// "%zz" is an invalid URL escape, so url.Parse inside http.NewRequest
	// fails before any transport — exercising both getJSON and postJSON
	// NewRequest error returns.
	c := NewClient("http://%zz", "")

	_, err := c.Words() // getJSON
	if err == nil {
		t.Fatal("Words() with a malformed base URL must return an error")
	}
	if !strings.Contains(err.Error(), "invalid URL escape") {
		t.Errorf("getJSON err = %q, want it to mention invalid URL escape", err)
	}

	_, _, err = c.Eval("1") // postJSON
	if err == nil {
		t.Fatal("Eval() with a malformed base URL must return an error")
	}
	if !strings.Contains(err.Error(), "invalid URL escape") {
		t.Errorf("postJSON err = %q, want it to mention invalid URL escape", err)
	}
}

// --- server.go net.Listen error (93-95) ---

func TestS7Dbg_ListenNetError(t *testing.T) {
	srv := NewServer(s7dbgReg(t, true), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 127.0.0.1 passes the loopback guard; port 999999 is out of range so the
	// real net.Listen fails.
	err := srv.ListenAndServe(ctx, "127.0.0.1:999999", false)
	if err == nil {
		t.Fatal("an out-of-range port must make net.Listen fail")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("err = %v, want a wrapped listen error", err)
	}
}

// --- server.go non-ErrServerClosed srv.Serve error (101-103) ---

func TestS7Dbg_ServeClosedListener(t *testing.T) {
	// Bind a real listener then close it, and hand that pre-closed listener to
	// ListenAndServe via the netListen seam. srv.Serve returns immediately with
	// a closed-conn error that is NOT http.ErrServerClosed.
	realLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := realLn.Close(); err != nil {
		t.Fatal(err)
	}
	orig := netListen
	netListen = func(_, _ string) (net.Listener, error) { return realLn, nil }
	t.Cleanup(func() { netListen = orig })

	srv := NewServer(s7dbgReg(t, true), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = srv.ListenAndServe(ctx, "127.0.0.1:0", false)
	if err == nil {
		t.Fatal("serving a pre-closed listener must return an error")
	}
	if errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("want a non-ErrServerClosed error, got %v", err)
	}
}

// --- server.go handleHealthz (118-120) ---

func TestS7Dbg_Healthz(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	resp, err := http.Get(ts.URL + "/debug/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Errorf("healthz body = %v, want {status:ok}", out)
	}
}

// --- server.go handleEval non-POST (145-148) + positive POST ---

func TestS7Dbg_EvalMethod(t *testing.T) {
	_, ts := s7dbgServer(t, "")

	// Negative: GET is refused with 405 "POST required".
	resp, err := http.Get(ts.URL + "/debug/eval")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := s7dbgReadClose(t, resp)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /debug/eval status = %d, want 405", resp.StatusCode)
	}
	if !strings.Contains(body, "POST required") {
		t.Errorf("GET /debug/eval body = %q, want POST required", body)
	}

	// Positive: POST evaluates and returns 200.
	presp, err := http.Post(ts.URL+"/debug/eval", "text/plain", strings.NewReader("1 2 add"))
	if err != nil {
		t.Fatal(err)
	}
	defer presp.Body.Close()
	pbody := s7dbgReadClose(t, presp)
	if presp.StatusCode != http.StatusOK {
		t.Errorf("POST /debug/eval status = %d, want 200 (body %q)", presp.StatusCode, pbody)
	}
	if !strings.Contains(pbody, "\"result\"") {
		t.Errorf("POST /debug/eval body = %q, want a result field", pbody)
	}
}

// --- server.go handleEval read-body error (150-153) ---

func TestS7Dbg_EvalReadBodyError(t *testing.T) {
	srv := NewServer(s7dbgReg(t, true), "")
	req := httptest.NewRequest(http.MethodPost, "/debug/eval", s7dbgErrReader{})
	rec := httptest.NewRecorder()
	srv.handleEval(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("read-body error status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "read body:") {
		t.Errorf("body = %q, want read body:", rec.Body.String())
	}
}

// --- server.go eval parser-not-configured (166-168) ---

func TestS7Dbg_EvalParserNotConfigured(t *testing.T) {
	// A registry WITHOUT SetParseFunc leaves ParseFunc nil.
	srv := NewServer(s7dbgReg(t, false), "")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	c := NewClient(ts.URL, "")
	_, evalErr, err := c.Eval("1 2 add")
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !strings.Contains(evalErr, "parser not configured") {
		t.Errorf("evalErr = %q, want parser not configured", evalErr)
	}
}

// --- server.go eval parse error (170-172) ---

func TestS7Dbg_EvalParseError(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	c := NewClient(ts.URL, "")
	// An unclosed paren is a parse error, surfaced as evalErr (not transport).
	_, evalErr, err := c.Eval("(")
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !strings.Contains(evalErr, "parse:") {
		t.Errorf("evalErr = %q, want parse:", evalErr)
	}
}

// --- server.go eval empty result (177-179) ---

func TestS7Dbg_EvalEmptyResult(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	c := NewClient(ts.URL, "")
	// Empty source parses to no tokens, leaving an empty result → "none".
	result, evalErr, err := c.Eval("")
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if evalErr != "" {
		t.Fatalf("evalErr = %q, want empty", evalErr)
	}
	if result != "none" {
		t.Errorf("result = %q, want none", result)
	}
}

// --- server.go handleEmit non-POST (184-187) + positive POST ---

func TestS7Dbg_EmitMethod(t *testing.T) {
	_, ts := s7dbgServer(t, "")

	// Negative: GET is refused with 405 "POST required".
	resp, err := http.Get(ts.URL + "/debug/emit?id=x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := s7dbgReadClose(t, resp)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /debug/emit status = %d, want 405", resp.StatusCode)
	}
	if !strings.Contains(body, "POST required") {
		t.Errorf("GET /debug/emit body = %q, want POST required", body)
	}

	// Positive: POST appends and returns 200.
	presp, err := http.Post(ts.URL+"/debug/emit?id=x", "application/json", strings.NewReader(`{"label":"l","value":"v"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer presp.Body.Close()
	pbody := s7dbgReadClose(t, presp)
	if presp.StatusCode != http.StatusOK {
		t.Errorf("POST /debug/emit status = %d, want 200 (body %q)", presp.StatusCode, pbody)
	}
}

// --- server.go handleEmit missing id (189-192) ---

func TestS7Dbg_EmitMissingID(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	resp, err := http.Post(ts.URL+"/debug/emit", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := s7dbgReadClose(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /debug/emit (no id) status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(body, "missing id") {
		t.Errorf("body = %q, want missing id", body)
	}
}

// --- server.go emit decode error (194-197) ---

func TestS7Dbg_EmitDecodeError(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	resp, err := http.Post(ts.URL+"/debug/emit?id=abc", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := s7dbgReadClose(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /debug/emit malformed body status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(body, "decode:") {
		t.Errorf("body = %q, want decode:", body)
	}
}

// --- server.go ring drop-oldest (202-204) ---

func TestS7Dbg_EmitRingDrop(t *testing.T) {
	srv, ts := s7dbgServer(t, "")
	srv.maxEv = 2 // tiny ring so overflow is easy to observe
	c := NewClient(ts.URL, "")
	for i := 0; i < 5; i++ {
		if err := c.Emit("ring", Event{Label: "l", Value: fmt.Sprintf("%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := c.Events("ring")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2 (ring cap dropped the oldest)", len(evs))
	}
	// Only the 2 most recent (values "3","4") survive; "0"-"2" were dropped.
	if evs[0].Value != "3" || evs[1].Value != "4" {
		t.Errorf("ring survivors = %q,%q, want 3,4", evs[0].Value, evs[1].Value)
	}
}

// --- server.go handleEvents missing id (212-215) ---

func TestS7Dbg_EventsMissingID(t *testing.T) {
	_, ts := s7dbgServer(t, "")
	resp, err := http.Get(ts.URL + "/debug/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := s7dbgReadClose(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET /debug/events (no id) status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(body, "missing id") {
		t.Errorf("body = %q, want missing id", body)
	}
}

// --- server.go isLoopback (234-236 SplitHostPort-error, 237-239 empty/localhost) ---

func TestS7Dbg_IsLoopback(t *testing.T) {
	cases := []struct {
		bind string
		want bool
	}{
		{"localhost", true},   // SplitHostPort errors (no port) → host=bind → "localhost"
		{":8080", true},       // empty host → loopback
		{"127.0.0.1:0", true}, // loopback IP
		{"8.8.8.8:80", false}, // public IP is not loopback
	}
	for _, tc := range cases {
		if got := isLoopback(tc.bind); got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.bind, got, tc.want)
		}
	}
}

// s7dbgReadClose drains and closes an HTTP response body, returning it as a
// string.
func s7dbgReadClose(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
