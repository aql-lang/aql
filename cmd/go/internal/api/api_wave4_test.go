// Wave-4 coverage for the api service: the Server lifecycle (Start,
// Stop, Bind, Addr, Metadata, accessor methods), the discovery-file
// error arms, and the handler branches the earlier suite left
// untouched (method rejections, action error arms, the async
// self-stop path).
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// failStopInspector wraps fakeInspector but fails StopService.
type failStopInspector struct{ fakeInspector }

func (i *failStopInspector) StopService(context.Context, string) error {
	return errors.New("stop exploded")
}

func TestW4AccessorMethods(t *testing.T) {
	s := NewServer("127.0.0.1:0", "tok", io.Discard)
	if s.Name() != "api" {
		t.Errorf("Name() = %q, want api", s.Name())
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped", s.Status())
	}
	if s.UsesStdio() {
		t.Error("UsesStdio() = true, want false")
	}
	if got := s.Addr(); got != "127.0.0.1:0" {
		t.Errorf("Addr() before Start = %q, want the bind string", got)
	}

	m := s.Metadata()
	if m["auth"] != "bearer" {
		t.Errorf("Metadata auth = %q, want bearer", m["auth"])
	}
	if m["addr"] == "" {
		t.Error("Metadata addr is empty")
	}

	noTok := NewServer("127.0.0.1:0", "", io.Discard)
	if noTok.Metadata()["auth"] != "none" {
		t.Errorf("Metadata auth = %q, want none", noTok.Metadata()["auth"])
	}
}

func TestW4SetSupervisorVersion(t *testing.T) {
	orig := supervisorVersion
	defer SetSupervisorVersion(orig)
	SetSupervisorVersion("9.9.9-w4")
	if supervisorVersion != "9.9.9-w4" {
		t.Errorf("supervisorVersion = %q, want 9.9.9-w4", supervisorVersion)
	}
}

func TestW4StartWithoutBind(t *testing.T) {
	s := NewServer("127.0.0.1:0", "", io.Discard)
	err := s.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing Bind call") {
		t.Fatalf("Start without Bind = %v, want missing-Bind error", err)
	}
}

func TestW4StartListenError(t *testing.T) {
	s := NewServer("256.256.256.256:0", "", io.Discard)
	s.Bind(&fakeInspector{})
	err := s.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("Start with bad bind = %v, want listen error", err)
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status after failed Start = %s, want stopped", s.Status())
	}
}

// startServer runs Start on a goroutine and waits for StateRunning.
func startServer(t *testing.T, s *Server, ctx context.Context) chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for s.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach running state")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return done
}

func TestW4StartServeAndStop(t *testing.T) {
	tmp := t.TempDir()
	origTmp := tempDirFunc
	tempDirFunc = func() string { return tmp }
	defer func() { tempDirFunc = origTmp }()

	a := &fakeService{name: "a"}
	a.state.Store(int32(service.StateRunning))
	s := NewServer("127.0.0.1:0", "", io.Discard)
	s.Bind(&fakeInspector{svcs: []service.Service{a, s}})

	done := startServer(t, s, context.Background())

	// The bound Addr now reflects the real listener.
	addr := s.Addr()
	if strings.HasSuffix(addr, ":0") {
		t.Errorf("Addr() after Start = %q, want a resolved port", addr)
	}

	// Discovery file exists while running.
	if _, err := os.Stat(filepath.Join(tmp, "boru-api.json")); err != nil {
		t.Errorf("discovery file missing while running: %s", err)
	}
	url, _, pid, err := ReadDiscoveryFile()
	if err != nil {
		t.Fatalf("ReadDiscoveryFile: %s", err)
	}
	if !strings.Contains(url, addr) || pid != os.Getpid() {
		t.Errorf("discovery = (%q, %d), want url with %q and pid %d", url, pid, addr, os.Getpid())
	}

	// The live server answers real HTTP.
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %s", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("healthz = %d, want 200", resp.StatusCode)
	}

	// Stop shuts down gracefully; Start returns nil.
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %s", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status after Stop = %s, want stopped", s.Status())
	}
	// Discovery file removed on shutdown.
	if _, err := os.Stat(filepath.Join(tmp, "boru-api.json")); !os.IsNotExist(err) {
		t.Error("discovery file survived shutdown")
	}
}

func TestW4StartContextCancel(t *testing.T) {
	tmp := t.TempDir()
	origTmp := tempDirFunc
	tempDirFunc = func() string { return tmp }
	defer func() { tempDirFunc = origTmp }()

	s := NewServer("127.0.0.1:0", "", io.Discard)
	s.Bind(&fakeInspector{})
	ctx, cancel := context.WithCancel(context.Background())
	done := startServer(t, s, ctx)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start after ctx cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestW4StartDiscoveryWriteFailureIsNonFatal(t *testing.T) {
	// tempDirFunc pointing at a nonexistent dir makes writeDiscoveryFile
	// fail; Start logs and keeps serving.
	origTmp := tempDirFunc
	tempDirFunc = func() string { return filepath.Join(os.TempDir(), "zz-w4-does-not-exist") }
	defer func() { tempDirFunc = origTmp }()

	var errBuf strings.Builder
	s := NewServer("127.0.0.1:0", "", &errBuf)
	s.Bind(&fakeInspector{})
	done := startServer(t, s, context.Background())

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Stop(stopCtx)
	<-done

	if !strings.Contains(errBuf.String(), "discovery file") {
		t.Errorf("stderr = %q, want a discovery-file warning", errBuf.String())
	}
}

func TestW4StopBeforeStart(t *testing.T) {
	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start = %v, want nil", err)
	}
}

func TestW4WriteDiscoveryFileCreateTempError(t *testing.T) {
	origTmp := tempDirFunc
	tempDirFunc = func() string { return filepath.Join(os.TempDir(), "zz-w4-missing-dir") }
	defer func() { tempDirFunc = origTmp }()

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); err == nil {
		t.Error("writeDiscoveryFile into a missing dir succeeded, want error")
	}
}

func TestW4WriteDiscoveryFileRenameError(t *testing.T) {
	// The target path exists as a non-empty DIRECTORY, so the final
	// rename fails after the temp file was written.
	tmp := t.TempDir()
	origTmp := tempDirFunc
	tempDirFunc = func() string { return tmp }
	defer func() { tempDirFunc = origTmp }()

	blocked := filepath.Join(tmp, "boru-api.json")
	if err := os.MkdirAll(filepath.Join(blocked, "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); err == nil {
		t.Error("writeDiscoveryFile over a directory succeeded, want rename error")
	}
}

func TestW4ReadDiscoveryFileMalformed(t *testing.T) {
	tmp := t.TempDir()
	origTmp := tempDirFunc
	tempDirFunc = func() string { return tmp }
	defer func() { tempDirFunc = origTmp }()

	if err := os.WriteFile(filepath.Join(tmp, "boru-api.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ReadDiscoveryFile(); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("ReadDiscoveryFile = %v, want malformed error", err)
	}
}

func TestW4RemoveDiscoveryFileNoop(t *testing.T) {
	// With no discoverPath recorded, removal is a no-op.
	s := NewServer("127.0.0.1:0", "", io.Discard)
	s.removeDiscoveryFile()
}

// --- handler branches ---

func TestW4PublicRoutesMethodNotAllowed(t *testing.T) {
	ts, _ := newTestServer(t, "")
	if status, _ := postJSON(t, ts.URL+"/healthz", "", map[string]any{}); status != 405 {
		t.Errorf("POST /healthz = %d, want 405", status)
	}
	if status, _ := postJSON(t, ts.URL+"/openapi.yaml", "", map[string]any{}); status != 405 {
		t.Errorf("POST /openapi.yaml = %d, want 405", status)
	}
	if status, _ := postJSON(t, ts.URL+"/v1/server", "", map[string]any{}); status != 405 {
		t.Errorf("POST /v1/server = %d, want 405", status)
	}
}

func TestW4ServiceByNameEdgeCases(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, _ := newTestServer(t, "", a)

	// Empty name segment.
	if status, _ := get(t, ts.URL+"/v1/services/", ""); status != 404 {
		t.Errorf("GET /v1/services/ = %d, want 404", status)
	}
	// Unknown subresource.
	if status, _ := get(t, ts.URL+"/v1/services/a/zz", ""); status != 404 {
		t.Errorf("GET /v1/services/a/zz = %d, want 404", status)
	}
	// POST on the entity itself.
	if status, _ := postJSON(t, ts.URL+"/v1/services/a", "", map[string]any{}); status != 405 {
		t.Errorf("POST /v1/services/a = %d, want 405", status)
	}
	// GET on the actions subresource.
	if status, _ := get(t, ts.URL+"/v1/services/a/actions", ""); status != 405 {
		t.Errorf("GET /v1/services/a/actions = %d, want 405", status)
	}
}

func TestW4ActionInvalidBody(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, _ := newTestServer(t, "", a)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/services/a/actions", strings.NewReader("{oops"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("invalid body = %d, want 400", resp.StatusCode)
	}
}

func TestW4ActionPauseResumeErrors(t *testing.T) {
	// pausable=false makes the fake's Pause/Resume return an error.
	a := &pausableFake{fakeService: &fakeService{name: "a"}}
	a.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", a)

	if status, _ := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "pause"}); status != 500 {
		t.Errorf("pause error = %d, want 500", status)
	}
	if status, _ := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "resume"}); status != 500 {
		t.Errorf("resume error = %d, want 500", status)
	}
}

func TestW4ActionResumeNotPausable(t *testing.T) {
	a := &fakeService{name: "a"} // no Pausable at all
	ts, _ := newTestServer(t, "", a)
	if status, _ := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "resume"}); status != 409 {
		t.Errorf("resume on non-pausable = %d, want 409", status)
	}
}

func TestW4ActionStopError(t *testing.T) {
	a := &fakeService{name: "a"}
	a.state.Store(int32(service.StateRunning))
	s := NewServer("127.0.0.1:0", "", io.Discard)
	insp := &failStopInspector{}
	insp.svcs = []service.Service{a}
	s.insp = insp
	ts := httptest.NewServer(s.handler())
	t.Cleanup(ts.Close)

	status, body := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "stop"})
	if status != 500 {
		t.Errorf("stop error = %d, want 500; body=%s", status, body)
	}
}

func TestW4ActionStopAPIItselfIsAsync(t *testing.T) {
	// Stopping the service named "api" answers 200 immediately and
	// dispatches the stop asynchronously.
	apiFake := &fakeService{name: "api"}
	apiFake.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", apiFake)

	code, body := postJSON(t, ts.URL+"/v1/services/api/actions", "", map[string]string{"action": "stop"})
	if code != 200 {
		t.Fatalf("stop api = %d, want 200; body=%s", code, body)
	}
	// The async goroutine stops the fake shortly after.
	deadline := time.Now().Add(5 * time.Second)
	for apiFake.Status() != service.StateStopped {
		if time.Now().After(deadline) {
			t.Fatal("async stop never reached the api service")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
