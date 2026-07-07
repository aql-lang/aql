package registry

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// --- NewServer validation ---

func TestNewServerRequiresDir(t *testing.T) {
	if _, err := NewServer("", 0); err == nil {
		t.Error("expected an error for an empty dir")
	}
	if _, err := NewServer(filepath.Join(t.TempDir(), "missing"), 0); err == nil {
		t.Error("expected an error for a nonexistent dir")
	}
	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(file, 0); err == nil {
		t.Error("expected an error for a non-directory path")
	}
}

// --- Full lifecycle: Start / serveHTTP / Pause / Resume / Metadata / Stop ---

func TestServerLifecycle(t *testing.T) {
	dir := t.TempDir()
	srv, err := NewServer(dir, 0)
	if err != nil {
		t.Fatal(err)
	}

	if got := srv.Name(); got != "registry" {
		t.Errorf("Name() = %q, want registry", got)
	}
	if srv.UsesStdio() {
		t.Error("UsesStdio() = true, want false")
	}
	if got := srv.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
	if got := srv.Addr(); got != ":0" {
		t.Errorf("Addr() before start = %q, want :0", got)
	}
	if got := srv.Status(); got != service.StateStopped {
		t.Errorf("Status() before start = %v, want stopped", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(ctx) }()

	// Wait for the bound port to publish on Addr().
	deadline := time.Now().Add(5 * time.Second)
	for srv.Addr() == ":0" || srv.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach running state")
		}
		time.Sleep(5 * time.Millisecond)
	}

	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("Addr() = %q: %v", srv.Addr(), err)
	}
	base := "http://127.0.0.1:" + port

	get := func(path string) int {
		t.Helper()
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Normal serving delegates to the registry handler.
	if code := get("/module/nonexistent-1.0.0"); code != http.StatusNotFound {
		t.Errorf("running GET = %d, want 404", code)
	}

	// Pause swaps in the 503 responder but keeps the listener bound.
	if err := srv.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := srv.Status(); got != service.StatePaused {
		t.Errorf("Status() after pause = %v, want paused", got)
	}
	if code := get("/module/nonexistent-1.0.0"); code != http.StatusServiceUnavailable {
		t.Errorf("paused GET = %d, want 503", code)
	}

	// Resume restores the real handler.
	if err := srv.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := srv.Status(); got != service.StateRunning {
		t.Errorf("Status() after resume = %v, want running", got)
	}
	if code := get("/module/nonexistent-1.0.0"); code != http.StatusNotFound {
		t.Errorf("resumed GET = %d, want 404", code)
	}

	// Metadata surfaces the bound addr and dir.
	md := srv.Metadata()
	if md["addr"] != srv.Addr() || md["dir"] != dir {
		t.Errorf("Metadata() = %v, want addr=%s dir=%s", md, srv.Addr(), dir)
	}

	// Stop shuts down gracefully; Start returns nil.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := srv.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start returned %v, want nil after graceful stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	if got := srv.Status(); got != service.StateStopped {
		t.Errorf("Status() after stop = %v, want stopped", got)
	}
}

func TestServerContextCancelStopsServer(t *testing.T) {
	srv, err := NewServer(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for srv.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach running state")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start returned %v, want nil after ctx cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestServerStartPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	srv, err := NewServer(t.TempDir(), port)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected a listen error for an occupied port")
	}
	if got := srv.Status(); got != service.StateStopped {
		t.Errorf("Status() after failed start = %v, want stopped", got)
	}
}
