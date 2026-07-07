package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAPI serves the three endpoints the tui client uses and records
// the last request for assertions.
type fakeAPI struct {
	lastPath     string
	lastAuth     string
	lastBody     map[string]string
	actionStatus int
	actionErr    string
}

func (f *fakeAPI) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/services", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "registry", "state": "running", "pausable": true,
				"metadata": map[string]string{"addr": ":8080"}},
		})
	})
	mux.HandleFunc("GET /v1/server", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pid": 99, "version": "1.0.0", "uptimeSeconds": 5.0, "serviceCount": 1,
		})
	})
	mux.HandleFunc("POST /v1/services/{name}/actions", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.lastBody = body
		status := f.actionStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if status != http.StatusOK && f.actionErr != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": f.actionErr})
		}
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func (f *fakeAPI) record(r *http.Request) {
	f.lastPath = r.URL.Path
	f.lastAuth = r.Header.Get("Authorization")
}

func TestClientListServices(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	c := newAPIClient(ts.URL, "tok")
	svcs, err := c.listServices()
	if err != nil {
		t.Fatalf("listServices: %v", err)
	}
	if len(svcs) != 1 || svcs[0].Name != "registry" || !svcs[0].Pausable {
		t.Errorf("services = %+v", svcs)
	}
	if f.lastAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", f.lastAuth)
	}
}

func TestClientGetServer(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	c := newAPIClient(ts.URL, "")
	info, err := c.getServer()
	if err != nil {
		t.Fatalf("getServer: %v", err)
	}
	if info.PID != 99 || info.Version != "1.0.0" || info.ServiceCount != 1 {
		t.Errorf("info = %+v", info)
	}
	if f.lastAuth != "" {
		t.Errorf("empty token must not send auth; got %q", f.lastAuth)
	}
}

func TestClientApplyActionOK(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	c := newAPIClient(ts.URL, "")
	if err := c.applyAction("registry", "pause"); err != nil {
		t.Fatalf("applyAction: %v", err)
	}
	if f.lastPath != "/v1/services/registry/actions" {
		t.Errorf("path = %q", f.lastPath)
	}
	if f.lastBody["action"] != "pause" {
		t.Errorf("body = %+v", f.lastBody)
	}
}

func TestClientApplyActionErrorBody(t *testing.T) {
	f := &fakeAPI{actionStatus: http.StatusConflict, actionErr: "not pausable"}
	ts := f.start(t)
	c := newAPIClient(ts.URL, "")
	err := c.applyAction("registry", "pause")
	if err == nil || !strings.Contains(err.Error(), "not pausable") {
		t.Errorf("err = %v, want the server's error string", err)
	}
}

func TestClientApplyActionErrorWithoutBody(t *testing.T) {
	f := &fakeAPI{actionStatus: http.StatusInternalServerError}
	ts := f.start(t)
	c := newAPIClient(ts.URL, "")
	err := c.applyAction("registry", "stop")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want the raw status", err)
	}
}

func TestClientGetJSONNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer ts.Close()
	c := newAPIClient(ts.URL, "")
	if _, err := c.listServices(); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want a 401 error", err)
	}
	if _, err := c.getServer(); err == nil {
		t.Error("getServer should propagate the non-200")
	}
}

func TestClientUnreachable(t *testing.T) {
	c := newAPIClient("http://127.0.0.1:1", "")
	if _, err := c.listServices(); err == nil {
		t.Error("expected a transport error")
	}
	if err := c.applyAction("x", "pause"); err == nil {
		t.Error("expected a transport error")
	}
}
