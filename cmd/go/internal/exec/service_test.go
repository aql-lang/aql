// Tests for the Service-shaped exec HTTP server (service.go): the
// Start/Stop lifecycle, Pause/Resume 503 gating, and the metadata
// surface. State is polled through the atomic-backed Status(), which
// also orders the test's reads of Addr() after Start's write.

package exec

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
	"github.com/aql-lang/aql/lang/go/policy"
)

// waitExecState polls Status until it reports want or the deadline
// passes.
func waitExecState(t *testing.T, s *Server, want service.State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.Status() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("server never reached state %s (currently %s)", want, s.Status())
}

// getStatus GETs url and returns the status code and body.
func getStatus(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %s", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestServerLifecycleServesPausesAndStops(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", "", nil)
	if err != nil {
		t.Fatalf("NewServer: %s", err)
	}
	if s.Name() != "exec" {
		t.Errorf("Name() = %q, want exec", s.Name())
	}
	if s.UsesStdio() {
		t.Error("UsesStdio() = true, want false")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("initial Status() = %s, want stopped", s.Status())
	}
	if s.Registry() != "" {
		t.Errorf("Registry() = %q, want empty", s.Registry())
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()
	waitExecState(t, s, service.StateRunning)

	base := "http://" + s.Addr()

	// Live: healthz answers and code executes.
	if code, _ := getStatus(t, base+"/healthz"); code != http.StatusOK {
		t.Errorf("healthz = %d, want 200", code)
	}
	resp, err := http.Post(base+"/v1/exec", "application/json", strings.NewReader(`{"code":"1 add 2"}`))
	if err != nil {
		t.Fatalf("POST /v1/exec: %s", err)
	}
	var got execResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	resp.Body.Close()
	if got.Error != "" {
		t.Fatalf("exec error: %s", got.Error)
	}
	if n, ok := got.Result.(float64); !ok || n != 3 {
		t.Errorf("result = %v, want 3", got.Result)
	}

	// Paused: everything answers 503 but the listener stays bound.
	if err := s.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %s", err)
	}
	if s.Status() != service.StatePaused {
		t.Errorf("Status() after Pause = %s, want paused", s.Status())
	}
	if code, body := getStatus(t, base+"/healthz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "exec paused") {
		t.Errorf("paused healthz = %d %q, want 503 exec paused", code, body)
	}

	// Resumed: back to normal service.
	if err := s.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %s", err)
	}
	if s.Status() != service.StateRunning {
		t.Errorf("Status() after Resume = %s, want running", s.Status())
	}
	if code, _ := getStatus(t, base+"/healthz"); code != http.StatusOK {
		t.Errorf("resumed healthz = %d, want 200", code)
	}

	md := s.Metadata()
	if md["addr"] != s.Addr() {
		t.Errorf("Metadata addr = %q, want %q", md["addr"], s.Addr())
	}
	if _, ok := md["registry"]; ok {
		t.Error("Metadata should omit registry when unset")
	}
	if _, ok := md["policy"]; ok {
		t.Error("Metadata should omit policy when unset")
	}

	if err := s.Stop(context.Background()); err != nil {
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
	if s.Status() != service.StateStopped {
		t.Errorf("final Status() = %s, want stopped", s.Status())
	}
}

func TestServerCancelShutsDown(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", "", nil)
	if err != nil {
		t.Fatalf("NewServer: %s", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx) }()
	waitExecState(t, s, service.StateRunning)

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after cancel = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestServerStopReportsShutdownTimeout(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", "", nil)
	if err != nil {
		t.Fatalf("NewServer: %s", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()
	waitExecState(t, s, service.StateRunning)

	// A connection that never finishes its request headers counts as
	// active, so a short-deadline Shutdown cannot complete in time.
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("dial: %s", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\n")); err != nil {
		t.Fatalf("write partial request: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Stop(ctx); err == nil {
		t.Error("Stop with an expired deadline and a hanging connection should report an error")
	}

	// The listener is closed by the (failed) Shutdown, so Start still
	// unwinds; drop the hanging connection to let it finish.
	conn.Close()
	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not return after Stop and connection close")
	}
}

func TestServerListenErrorSurfaced(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	s, err := NewServer(ln.Addr().String(), "", nil)
	if err != nil {
		t.Fatalf("NewServer: %s", err)
	}
	startErr := s.Start(context.Background())
	if startErr == nil {
		t.Fatal("Start on an occupied port should fail")
	}
	if !strings.Contains(startErr.Error(), "listen") {
		t.Errorf("error = %q, want to mention listen", startErr)
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped after failed Start", s.Status())
	}
}

func TestServerDefaultsAndMetadata(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer("", "/reg/path", pol)
	if err != nil {
		t.Fatalf("NewServer: %s", err)
	}
	if got := s.Addr(); got != "127.0.0.1:8091" {
		t.Errorf("Addr() = %q, want the 127.0.0.1:8091 default", got)
	}
	if s.Registry() != "/reg/path" {
		t.Errorf("Registry() = %q, want /reg/path", s.Registry())
	}
	md := s.Metadata()
	if md["registry"] != "/reg/path" {
		t.Errorf("Metadata registry = %q, want /reg/path", md["registry"])
	}
	if md["policy"] != pol.Name() {
		t.Errorf("Metadata policy = %q, want %q", md["policy"], pol.Name())
	}
}
