// Tests for the Service-shaped LSP server wrapper (service.go) and
// the `boru lsp` subcommand (lsp.go). Stdio sessions are driven with
// pre-framed input buffers (fully deterministic); TCP sessions dial
// the listener that Start binds. Blocking-transport tests use io.Pipe
// plus ctx-cancel/Stop to prove Start unwinds promptly.

package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// syncBuffer is a mutex-guarded bytes.Buffer safe to share between
// the server goroutine and the test goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
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

// frameBytes encodes payload as one LSP base-protocol framed message.
func frameBytes(t *testing.T, payload any) []byte {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}

// cleanSessionInput is the canonical initialize → shutdown → exit
// handshake, pre-framed.
func cleanSessionInput(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}}))
	buf.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 2, Method: "shutdown"}))
	buf.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "exit"}))
	return buf.Bytes()
}

// waitStatus polls srv.Status until it reports want or the deadline
// passes. Status is atomic-backed, so polling is race-free.
func waitStatus(t *testing.T, srv *Server, want service.State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if srv.Status() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("server never reached state %s (currently %s)", want, srv.Status())
}

// waitListenAddr polls the stderr log for the "listening on <addr>"
// line the TCP server prints once bound, and returns the address.
func waitListenAddr(t *testing.T, buf *syncBuffer) string {
	t.Helper()
	const marker = "listening on "
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := buf.String()
		if i := strings.Index(s, marker); i >= 0 {
			rest := s[i+len(marker):]
			if j := strings.IndexByte(rest, '\n'); j >= 0 {
				return strings.TrimSpace(rest[:j])
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("server never reported a listening address")
	return ""
}

// --- stdio mode ---

func TestStdioServerCleanSession(t *testing.T) {
	var out, stderr bytes.Buffer
	srv := NewStdioServer(bytes.NewReader(cleanSessionInput(t)), &out, &stderr)

	if srv.Name() != "lsp" {
		t.Errorf("Name() = %q, want %q", srv.Name(), "lsp")
	}
	if !srv.UsesStdio() {
		t.Error("UsesStdio() = false, want true for stdio server")
	}
	if srv.Status() != service.StateStopped {
		t.Errorf("initial Status() = %s, want stopped", srv.Status())
	}
	if got := srv.Addr(); got != "" {
		t.Errorf("Addr() = %q, want empty for stdio mode", got)
	}

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %s", err)
	}
	if code := srv.ExitCode(); code != 0 {
		t.Errorf("ExitCode() = %d, want 0 after clean shutdown/exit", code)
	}
	if srv.Status() != service.StateStopped {
		t.Errorf("post-Start Status() = %s, want stopped", srv.Status())
	}
	if !strings.Contains(out.String(), "boru-lsp") {
		t.Errorf("expected initialize response naming boru-lsp, got %q", out.String())
	}

	md := srv.Metadata()
	if md["mode"] != "stdio" {
		t.Errorf("Metadata mode = %q, want stdio", md["mode"])
	}
	if _, ok := md["addr"]; ok {
		t.Error("stdio Metadata should not carry an addr key")
	}
}

func TestStdioServerEOFWithoutShutdownIsDirtyExit(t *testing.T) {
	srv := NewStdioServer(strings.NewReader(""), io.Discard, io.Discard)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %s", err)
	}
	if code := srv.ExitCode(); code != 1 {
		t.Errorf("ExitCode() = %d, want 1 for EOF without shutdown", code)
	}
}

func TestStdioServerCancelUnblocksStart(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	srv := NewStdioServer(stdinR, io.Discard, io.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	waitStatus(t, srv, service.StateRunning)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after cancel = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
	if code := srv.ExitCode(); code != 1 {
		t.Errorf("ExitCode() = %d, want 1 for canceled session", code)
	}
}

func TestStdioServerStopUnblocksStart(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	srv := NewStdioServer(stdinR, io.Discard, io.Discard)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	waitStatus(t, srv, service.StateRunning)
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after Stop = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

// --- TCP mode ---

func TestTCPServerCleanSession(t *testing.T) {
	stderr := &syncBuffer{}
	srv := NewTCPServer("127.0.0.1", 0, stderr)

	if srv.UsesStdio() {
		t.Error("UsesStdio() = true, want false for TCP server")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	addr := waitListenAddr(t, stderr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %s", addr, err)
	}
	defer conn.Close()

	br := bufio.NewReader(conn)

	if _, err := conn.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})); err != nil {
		t.Fatalf("write initialize: %s", err)
	}
	data, err := readFramedMessage(br)
	if err != nil {
		t.Fatalf("read initialize response: %s", err)
	}
	if !strings.Contains(string(data), "boru-lsp") {
		t.Errorf("initialize response = %s, want to contain boru-lsp", data)
	}

	if _, err := conn.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 2, Method: "shutdown"})); err != nil {
		t.Fatalf("write shutdown: %s", err)
	}
	if _, err := readFramedMessage(br); err != nil {
		t.Fatalf("read shutdown response: %s", err)
	}
	if _, err := conn.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "exit"})); err != nil {
		t.Fatalf("write exit: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after exit")
	}
	if code := srv.ExitCode(); code != 0 {
		t.Errorf("ExitCode() = %d, want 0", code)
	}
	if srv.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped", srv.Status())
	}

	md := srv.Metadata()
	if md["mode"] != "tcp" {
		t.Errorf("Metadata mode = %q, want tcp", md["mode"])
	}
	if md["addr"] == "" {
		t.Error("Metadata addr should be set for TCP mode")
	}
	if got := srv.Addr(); got != addr {
		t.Errorf("Addr() = %q, want bound address %q", got, addr)
	}
}

func TestTCPServerCancelBeforeAccept(t *testing.T) {
	stderr := &syncBuffer{}
	srv := NewTCPServer("127.0.0.1", 0, stderr)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	waitListenAddr(t, stderr)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after cancel = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
	if srv.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped", srv.Status())
	}
}

func TestTCPServerStopBeforeAccept(t *testing.T) {
	stderr := &syncBuffer{}
	srv := NewTCPServer("127.0.0.1", 0, stderr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	waitListenAddr(t, stderr)
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after Stop = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestTCPServerStopClosesActiveConnection(t *testing.T) {
	stderr := &syncBuffer{}
	srv := NewTCPServer("127.0.0.1", 0, stderr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	addr := waitListenAddr(t, stderr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %s", err)
	}
	defer conn.Close()

	// Complete one round-trip so the connection is definitely the one
	// the server is serving before we yank it away.
	br := bufio.NewReader(conn)
	if _, err := conn.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})); err != nil {
		t.Fatalf("write initialize: %s", err)
	}
	if _, err := readFramedMessage(br); err != nil {
		t.Fatalf("read initialize response: %s", err)
	}

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after Stop = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop closed the connection")
	}
	// Connection was severed without shutdown/exit: dirty exit code.
	if code := srv.ExitCode(); code != 1 {
		t.Errorf("ExitCode() = %d, want 1 for severed connection", code)
	}
}

func TestTCPServerListenErrorSurfaced(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	srv := NewTCPServer("127.0.0.1", port, io.Discard)
	startErr := srv.Start(context.Background())
	if startErr == nil {
		t.Fatal("Start on an occupied port should fail")
	}
	if !strings.Contains(startErr.Error(), "listen") {
		t.Errorf("error = %q, want to mention listen", startErr)
	}
	if srv.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped after failed Start", srv.Status())
	}
}

func TestTCPServerEmptyHostDefaultsToLoopback(t *testing.T) {
	srv := NewTCPServer("", 8123, io.Discard)
	if got := srv.Addr(); got != "127.0.0.1:8123" {
		t.Errorf("Addr() = %q, want 127.0.0.1:8123 (empty host must not bind all interfaces)", got)
	}
}

// --- `boru lsp` subcommand (lsp.go) ---

func TestLSPCommandMetadata(t *testing.T) {
	c := New()
	if c.Name() != "lsp" {
		t.Errorf("Name() = %q, want lsp", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() should not be empty")
	}
}

func TestLSPCommandRunRejectsBadFlag(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run([]string{"-definitely-not-a-flag"}, strings.NewReader(""), &out, &stderr)
	if code != 1 {
		t.Errorf("Run with bad flag = %d, want 1", code)
	}
}

func TestLSPCommandRunStdioCleanSession(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run(nil, bytes.NewReader(cleanSessionInput(t)), &out, &stderr)
	if code != 0 {
		t.Errorf("Run = %d, want 0 for clean handshake (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(out.String(), "boru-lsp") {
		t.Errorf("expected initialize response on stdout, got %q", out.String())
	}
}

func TestLSPCommandRunStdioDirtyExitCode(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run(nil, strings.NewReader(""), &out, &stderr)
	if code != 1 {
		t.Errorf("Run = %d, want 1 when stdin EOFs before shutdown", code)
	}
}

func TestLSPCommandRunTCPListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var out, stderr bytes.Buffer
	code := New().Run([]string{"-p", strconv.Itoa(port)}, strings.NewReader(""), &out, &stderr)
	if code != 1 {
		t.Errorf("Run = %d, want 1 when the TCP port is occupied", code)
	}
	if !strings.Contains(stderr.String(), "lsp:") {
		t.Errorf("stderr = %q, want the lsp: error prefix", stderr.String())
	}
}
