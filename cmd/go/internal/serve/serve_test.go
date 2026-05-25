package serve

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// fakeService is a controllable Service used to exercise the
// supervisor without spinning up real network listeners.
type fakeService struct {
	name     string
	stdio    bool
	pausable bool
	startErr error

	state     atomic.Int32
	paused    atomic.Bool
	started   atomic.Bool
	stopped   atomic.Bool
	startCh   chan struct{} // closed on first Start
	releaseCh chan struct{} // tests close to let Start return
	mu        sync.Mutex
}

func newFakeService(name string, stdio, pausable bool) *fakeService {
	return &fakeService{
		name:      name,
		stdio:     stdio,
		pausable:  pausable,
		startCh:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (f *fakeService) Name() string { return f.name }

func (f *fakeService) Status() service.State {
	return service.State(f.state.Load())
}

func (f *fakeService) UsesStdio() bool { return f.stdio }

func (f *fakeService) Start(ctx context.Context) error {
	if f.startErr != nil {
		return f.startErr
	}
	if f.started.CompareAndSwap(false, true) {
		close(f.startCh)
	}
	f.state.Store(int32(service.StateRunning))
	select {
	case <-ctx.Done():
	case <-f.releaseCh:
	}
	f.state.Store(int32(service.StateStopped))
	f.stopped.Store(true)
	return nil
}

func (f *fakeService) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.releaseCh:
	default:
		close(f.releaseCh)
	}
	return nil
}

func (f *fakeService) Pause(context.Context) error {
	if !f.pausable {
		return fmt.Errorf("not pausable")
	}
	f.paused.Store(true)
	f.state.Store(int32(service.StatePaused))
	return nil
}

func (f *fakeService) Resume(context.Context) error {
	if !f.pausable {
		return fmt.Errorf("not pausable")
	}
	f.paused.Store(false)
	f.state.Store(int32(service.StateRunning))
	return nil
}

// --- stdio conflict ---

func TestCheckStdioConflictNone(t *testing.T) {
	svcs := []service.Service{
		newFakeService("a", false, false),
		newFakeService("b", false, false),
	}
	if err := checkStdioConflict(svcs); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestCheckStdioConflictOne(t *testing.T) {
	svcs := []service.Service{
		newFakeService("a", true, false),
		newFakeService("b", false, false),
	}
	if err := checkStdioConflict(svcs); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestCheckStdioConflictTwo(t *testing.T) {
	svcs := []service.Service{
		newFakeService("repl", true, false),
		newFakeService("lsp", true, false),
	}
	err := checkStdioConflict(svcs)
	if err == nil {
		t.Fatal("expected stdio conflict error")
	}
	if !strings.Contains(err.Error(), "stdio conflict") {
		t.Errorf("error = %q, want it to mention stdio conflict", err)
	}
	if !strings.Contains(err.Error(), "lsp") || !strings.Contains(err.Error(), "repl") {
		t.Errorf("error %q should name the conflicting services", err)
	}
}

// --- duplicate names ---

func TestCheckDuplicateNames(t *testing.T) {
	svcs := []service.Service{
		newFakeService("lsp", false, false),
		newFakeService("lsp", false, false),
	}
	if err := checkDuplicateNames(svcs); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestCheckDuplicateNamesOK(t *testing.T) {
	svcs := []service.Service{
		newFakeService("repl", false, false),
		newFakeService("lsp", false, false),
	}
	if err := checkDuplicateNames(svcs); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

// --- supervisor run + signal-driven shutdown ---

func TestSupervisorRunsAndStopsOnCancel(t *testing.T) {
	a := newFakeService("a", false, false)
	b := newFakeService("b", false, false)

	var stdout, stderr bytes.Buffer
	sup := newSupervisor([]service.Service{a, b}, &stdout, &stderr)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait until both services have started, then cancel.
		<-a.startCh
		<-b.startCh
		cancel()
	}()

	code := sup.run(ctx)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !a.stopped.Load() || !b.stopped.Load() {
		t.Errorf("both services should be stopped: a=%v b=%v", a.stopped.Load(), b.stopped.Load())
	}
}

// --- Inspector surface (used by api/tui) ---

func TestSupervisorInspector(t *testing.T) {
	a := newFakeService("a", false, true)
	b := newFakeService("b", false, false)
	sup := newSupervisor([]service.Service{a, b}, &bytes.Buffer{}, &bytes.Buffer{})

	// Inspector view: Services, ByName.
	all := sup.Services()
	if len(all) != 2 || all[0].Name() != "a" || all[1].Name() != "b" {
		t.Errorf("Services() = %v, want [a b]", all)
	}
	if got, ok := sup.ByName("a"); !ok || got.Name() != "a" {
		t.Errorf("ByName(a) = (%v, %v)", got, ok)
	}
	if _, ok := sup.ByName("nope"); ok {
		t.Error("ByName(nope) should be !ok")
	}

	// StopService: cancel context + Stop the service.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	supDone := make(chan int, 1)
	go func() { supDone <- sup.run(ctx) }()

	<-a.startCh
	<-b.startCh

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := sup.StopService(stopCtx, "a"); err != nil {
		t.Fatalf("StopService(a): %s", err)
	}
	if err := sup.StopService(stopCtx, "b"); err != nil {
		t.Fatalf("StopService(b): %s", err)
	}
	select {
	case <-supDone:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not exit after stopping all services")
	}
	if err := sup.StopService(stopCtx, "nope"); err == nil {
		t.Error("StopService(nope) should fail")
	}
}

// --- factory routing ---

func TestBuildServicesUnknown(t *testing.T) {
	_, err := buildServices([][]string{{"nope"}}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("error = %q, want 'unknown service'", err)
	}
}

func TestBuildServicesRegistryRequiresDir(t *testing.T) {
	_, err := buildServices([][]string{{"registry", "-p", "0"}}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing -r")
	}
	if !strings.Contains(err.Error(), "-r") {
		t.Errorf("error = %q, want it to mention -r", err)
	}
}

// --- config file ---

func TestLoadConfigBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.jsonic")
	body := `[
  { name: registry, flags: [-r, ./mods, -p, "8080"] }
  { name: lsp,      flags: [-p, "9000"] }
]`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	segs, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %s", err)
	}
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	if segs[0][0] != "registry" || segs[1][0] != "lsp" {
		t.Errorf("names = %s,%s, want registry,lsp", segs[0][0], segs[1][0])
	}
	if got := strings.Join(segs[0], " "); got != "registry -r ./mods -p 8080" {
		t.Errorf("segment 0 = %q", got)
	}
}

func TestLoadConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.jsonic")
	os.WriteFile(path, []byte("[]"), 0644)
	if _, err := loadConfig(path); err == nil {
		t.Error("expected error for empty list")
	}
}

func TestLoadConfigMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.jsonic")
	os.WriteFile(path, []byte(`[{ flags: [-p, "9000"] }]`), 0644)
	if _, err := loadConfig(path); err == nil {
		t.Error("expected error for missing name")
	}
}

// --- registry Service integration (smoke test) ---

func TestRegistryServiceStartStop(t *testing.T) {
	// Use the real registry Service to verify the lifecycle works
	// end-to-end through the supervisor.
	dir := t.TempDir()

	svcs, err := buildServices([][]string{{"registry", "-r", dir, "-p", "0"}}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildServices: %s", err)
	}

	var stdout, stderr bytes.Buffer
	sup := newSupervisor(svcs, &stdout, &stderr)

	ctx, cancel := context.WithCancel(context.Background())
	supDone := make(chan int, 1)
	go func() {
		supDone <- sup.run(ctx)
	}()

	// Wait for the service to be Running, then verify we can hit
	// it over HTTP.
	waitForState(t, svcs[0], service.StateRunning)

	// Reach into the registry server to get the bound port.
	type addrer interface{ Addr() string }
	addr := svcs[0].(addrer).Addr()
	// addr looks like "[::]:40773" or "0.0.0.0:40773"; pull the port.
	_, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		t.Fatalf("split addr %q: %s", addr, splitErr)
	}
	resp, err := http.Get("http://127.0.0.1:" + port + "/module/does-not-exist")
	if err != nil {
		t.Fatalf("http get: %s", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}

	cancel()
	select {
	case <-supDone:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not exit within 3s")
	}
}

// --- helpers ---

// ctlReply is the local shape we decode replies into.
// waitForState polls until svc reaches state or 2s elapses.
func waitForState(t *testing.T, svc service.Service, state service.State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.Status() == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("service %s never reached state %s (current: %s)", svc.Name(), state, svc.Status())
}
