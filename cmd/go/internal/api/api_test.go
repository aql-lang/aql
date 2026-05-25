package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// --- test doubles ---

type fakeService struct {
	name     string
	stdio    bool
	pausable bool
	state    atomic.Int32
	meta     map[string]string

	paused atomic.Bool
}

func (f *fakeService) Name() string               { return f.name }
func (f *fakeService) Status() service.State      { return service.State(f.state.Load()) }
func (f *fakeService) Start(context.Context) error {
	f.state.Store(int32(service.StateRunning))
	return nil
}
func (f *fakeService) Stop(context.Context) error {
	f.state.Store(int32(service.StateStopped))
	return nil
}
func (f *fakeService) UsesStdio() bool             { return f.stdio }
func (f *fakeService) Metadata() map[string]string { return f.meta }

// pausableFake adds Pausable to fakeService.
type pausableFake struct{ *fakeService }

func (p *pausableFake) Pause(context.Context) error {
	if !p.pausable {
		return io.EOF // synthetic error
	}
	p.paused.Store(true)
	p.state.Store(int32(service.StatePaused))
	return nil
}
func (p *pausableFake) Resume(context.Context) error {
	if !p.pausable {
		return io.EOF
	}
	p.paused.Store(false)
	p.state.Store(int32(service.StateRunning))
	return nil
}

// fakeInspector implements service.Inspector against a static list.
type fakeInspector struct {
	svcs []service.Service
}

func (i *fakeInspector) Services() []service.Service { return i.svcs }
func (i *fakeInspector) ByName(name string) (service.Service, bool) {
	for _, s := range i.svcs {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}
func (i *fakeInspector) StopService(ctx context.Context, name string) error {
	s, ok := i.ByName(name)
	if !ok {
		return ErrUnknown
	}
	return s.Stop(ctx)
}

// ErrUnknown is a sentinel used by fakeInspector to mimic the
// supervisor's "unknown service" error.
var ErrUnknown = &unknownErr{}

type unknownErr struct{}

func (*unknownErr) Error() string { return "unknown service" }

// --- helpers ---

// newTestServer builds an api Server backed by a fake inspector and
// returns an httptest server hitting its handler.
func newTestServer(t *testing.T, token string, svcs ...service.Service) (*httptest.Server, *Server) {
	t.Helper()
	s := NewServer("127.0.0.1:0", token, io.Discard)
	s.insp = &fakeInspector{svcs: svcs}
	ts := httptest.NewServer(s.handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func get(t *testing.T, url, token string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func postJSON(t *testing.T, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

// --- tests ---

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, _ := get(t, ts.URL+"/healthz", "")
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestOpenAPISpec(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, body := get(t, ts.URL+"/openapi.yaml", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "openapi: 3.0.3") {
		t.Errorf("expected openapi 3.0.3 header, got start: %.100q", body)
	}
	if !strings.Contains(string(body), "/services/{name}/actions") {
		t.Error("openapi spec missing actions path")
	}
}

func TestListServices(t *testing.T) {
	a := &pausableFake{fakeService: &fakeService{name: "a", pausable: true, meta: map[string]string{"addr": ":1"}}}
	a.state.Store(int32(service.StateRunning))
	b := &fakeService{name: "b", stdio: true, meta: map[string]string{"mode": "stdio"}}
	b.state.Store(int32(service.StateRunning))

	ts, _ := newTestServer(t, "", a, b)
	resp, body := get(t, ts.URL+"/v1/services", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	var got []map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %s (body=%s)", err, body)
	}
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}
	if got[0]["name"] != "a" || got[1]["name"] != "b" {
		t.Errorf("order wrong: %v", got)
	}
	if got[0]["pausable"] != true {
		t.Errorf("a should be pausable")
	}
	if got[1]["usesStdio"] != true {
		t.Errorf("b should use stdio")
	}
}

func TestGetService(t *testing.T) {
	a := &fakeService{name: "a", meta: map[string]string{"k": "v"}}
	a.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", a)

	resp, body := get(t, ts.URL+"/v1/services/a", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["name"] != "a" {
		t.Errorf("got %v", got)
	}

	resp, _ = get(t, ts.URL+"/v1/services/nope", "")
	if resp.StatusCode != 404 {
		t.Errorf("nonexistent: status = %d, want 404", resp.StatusCode)
	}

	resp, _ = get(t, ts.URL+"/v1/services/bad!name", "")
	if resp.StatusCode != 400 {
		t.Errorf("bad name: status = %d, want 400", resp.StatusCode)
	}
}

func TestActionPauseAndResume(t *testing.T) {
	a := &pausableFake{fakeService: &fakeService{name: "a", pausable: true}}
	a.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", a)

	resp, body := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "pause"})
	if resp.StatusCode != 200 {
		t.Fatalf("pause: status = %d, body=%s", resp.StatusCode, body)
	}
	if !a.paused.Load() {
		t.Error("expected paused")
	}

	resp, _ = postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "resume"})
	if resp.StatusCode != 200 {
		t.Fatalf("resume: status = %d", resp.StatusCode)
	}
	if a.paused.Load() {
		t.Error("expected resumed")
	}
}

func TestActionStop(t *testing.T) {
	a := &fakeService{name: "a"}
	a.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", a)

	resp, body := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "stop"})
	if resp.StatusCode != 200 {
		t.Fatalf("stop: status = %d, body=%s", resp.StatusCode, body)
	}
	if a.Status() != service.StateStopped {
		t.Errorf("state = %s, want stopped", a.Status())
	}
}

func TestActionNotPausable(t *testing.T) {
	a := &fakeService{name: "a"} // no Pausable
	a.state.Store(int32(service.StateRunning))
	ts, _ := newTestServer(t, "", a)

	resp, body := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "pause"})
	if resp.StatusCode != 409 {
		t.Errorf("status = %d, want 409, body=%s", resp.StatusCode, body)
	}
}

func TestActionUnknown(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, _ := newTestServer(t, "", a)
	resp, _ := postJSON(t, ts.URL+"/v1/services/a/actions", "", map[string]string{"action": "explode"})
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAuthTokenEnforced(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, _ := newTestServer(t, "secret-token", a)

	// No token: 401.
	resp, _ := get(t, ts.URL+"/v1/services", "")
	if resp.StatusCode != 401 {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}

	// Wrong token: 401.
	resp, _ = get(t, ts.URL+"/v1/services", "wrong")
	if resp.StatusCode != 401 {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}

	// Correct token: 200.
	resp, _ = get(t, ts.URL+"/v1/services", "secret-token")
	if resp.StatusCode != 200 {
		t.Errorf("right token: status = %d, want 200", resp.StatusCode)
	}

	// /healthz is unauthenticated.
	resp, _ = get(t, ts.URL+"/healthz", "")
	if resp.StatusCode != 200 {
		t.Errorf("healthz with no auth: status = %d, want 200", resp.StatusCode)
	}
}

func TestServerInfo(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, srv := newTestServer(t, "", a)
	// Set startTime so uptime is meaningful.
	resp, body := get(t, ts.URL+"/v1/server", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var info map[string]any
	json.Unmarshal(body, &info)
	if info["serviceCount"] != float64(1) {
		t.Errorf("serviceCount = %v, want 1", info["serviceCount"])
	}
	if info["version"] == "" {
		t.Error("version is empty")
	}
	_ = srv // keep ref alive
}

func TestMethodNotAllowed(t *testing.T) {
	a := &fakeService{name: "a"}
	ts, _ := newTestServer(t, "", a)
	resp, _ := postJSON(t, ts.URL+"/v1/services", "", map[string]any{})
	if resp.StatusCode != 405 {
		t.Errorf("POST /v1/services: status = %d, want 405", resp.StatusCode)
	}
}

func TestDiscoveryFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := tempDirFunc
	tempDirFunc = func() string { return dir }
	defer func() { tempDirFunc = orig }()

	// With no listener bound, Addr() returns s.bind directly.
	s := NewServer("127.0.0.1:54321", "tok", io.Discard)
	if err := s.writeDiscoveryFile(); err != nil {
		t.Fatalf("write: %s", err)
	}

	url, tok, pid, err := ReadDiscoveryFile()
	if err != nil {
		t.Fatalf("read: %s", err)
	}
	if url != "http://127.0.0.1:54321" {
		t.Errorf("url = %q", url)
	}
	if tok != "tok" {
		t.Errorf("token = %q", tok)
	}
	if pid == 0 {
		t.Error("pid should be set")
	}

	s.removeDiscoveryFile()
	if _, _, _, err := ReadDiscoveryFile(); err == nil {
		t.Error("expected error reading removed discovery file")
	}
}
