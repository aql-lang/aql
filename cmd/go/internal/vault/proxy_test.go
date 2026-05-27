package vault

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeUpstream records the auth header and request shape it
// receives so tests can verify the proxy injected credentials.
type fakeUpstream struct {
	*httptest.Server
	mu       sync.Mutex
	lastAuth string
	lastKey  string
	lastPath string
	lastBody string
	lastMeth string
	respond  func(w http.ResponseWriter, r *http.Request)
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	fu := &fakeUpstream{}
	fu.respond = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}
	fu.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fu.mu.Lock()
		fu.lastAuth = r.Header.Get("Authorization")
		fu.lastKey = r.Header.Get("x-api-key")
		fu.lastPath = r.URL.Path
		fu.lastBody = string(body)
		fu.lastMeth = r.Method
		fu.mu.Unlock()
		fu.respond(w, r)
	}))
	t.Cleanup(fu.Server.Close)
	return fu
}

// registerTestProvider installs a one-off provider preset that
// points at the fake upstream for the duration of the test.
func registerTestProvider(t *testing.T, name string, fu *fakeUpstream, style string) {
	t.Helper()
	prev, had := providers[name]
	providers[name] = Provider{Name: name, BaseURL: fu.URL, AuthStyle: style}
	t.Cleanup(func() {
		if had {
			providers[name] = prev
		} else {
			delete(providers, name)
		}
	})
}

// startProxy launches a Proxy on a random local port and returns
// its base URL. The proxy is stopped on test cleanup.
func startProxy(t *testing.T) string {
	t.Helper()
	homeDir := os.Getenv(EnvHome)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	p := NewProxy(addr, homeDir, os.Getenv(EnvPassphrase), io.Discard, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	// Wait briefly for the listener to come up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return "http://" + addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("proxy did not start in time")
	return ""
}

// grantOK creates a capability for alias and returns its id.
func grantOK(t *testing.T, alias string, hosts, methods []string) string {
	t.Helper()
	home := os.Getenv(EnvHome)
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.NewCapability(alias, "test-agent", hosts, methods, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	return tok.ID
}

func TestProxyForwardsAndInjectsBearer(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake-bearer", fu, "bearer")

	// Add an alias tagged with the fake provider so the proxy
	// knows where to forward.
	if code, _, errOut := runVault(t, "real-secret-XYZ\n", "add",
		"--from-stdin", "--provider=fake-bearer", "openai"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	tok := grantOK(t, "openai", []string{mustHost(fu.URL)}, []string{"POST"})

	base := startProxy(t)
	req, _ := http.NewRequest("POST", base+"/openai/v1/chat/completions",
		strings.NewReader(`{"hello":1}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}

	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastAuth != "Bearer real-secret-XYZ" {
		t.Errorf("upstream Authorization = %q, want Bearer real-secret-XYZ", fu.lastAuth)
	}
	if fu.lastPath != "/v1/chat/completions" {
		t.Errorf("upstream path = %q, want /v1/chat/completions", fu.lastPath)
	}
	if fu.lastBody != `{"hello":1}` {
		t.Errorf("upstream body = %q", fu.lastBody)
	}
	if fu.lastMeth != "POST" {
		t.Errorf("upstream method = %q, want POST", fu.lastMeth)
	}
}

func TestProxyInjectsXApiKeyForAnthropicStyle(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake-anthropic", fu, "x-api-key")

	if code, _, errOut := runVault(t, "ant-key\n", "add",
		"--from-stdin", "--provider=fake-anthropic", "anthropic"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	tok := grantOK(t, "anthropic", nil, nil)

	base := startProxy(t)
	req, _ := http.NewRequest("POST", base+"/anthropic/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastKey != "ant-key" {
		t.Errorf("upstream x-api-key = %q, want ant-key", fu.lastKey)
	}
	if fu.lastAuth != "" {
		t.Errorf("upstream Authorization leaked: %q", fu.lastAuth)
	}
}

func TestProxyRejectsMissingToken(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")

	if code, _, errOut := runVault(t, "v\n", "add",
		"--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	_ = grantOK(t, "k", nil, nil)

	base := startProxy(t)
	resp, err := http.Get(base + "/k/foo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "denied") {
		t.Errorf("body=%q", body)
	}
}

func TestProxyRejectsUnknownToken(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", resp.StatusCode)
	}
}

func TestProxyRejectsRevokedToken(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, nil)
	if code, _, _ := runVault(t, "", "revoke", tok); code != 0 {
		t.Fatal("revoke")
	}

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}

func TestProxyEnforcesMethodAllowlist(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, []string{"GET"})

	base := startProxy(t)
	req, _ := http.NewRequest("POST", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}

func TestProxyEnforcesHostAllowlist(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	// Restrict to a host that does NOT match the fake upstream.
	tok := grantOK(t, "k", []string{"example.com"}, nil)

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}

func TestProxyRejectsAliasMismatch(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "a"); code != 0 {
		t.Fatal("add a")
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "b"); code != 0 {
		t.Fatal("add b")
	}
	tokForA := grantOK(t, "a", nil, nil)

	base := startProxy(t)
	// Try to use A's capability against alias B.
	req, _ := http.NewRequest("GET", base+"/b/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tokForA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}

func TestProxyRejectsLockedVault(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, nil)

	// Start proxy first so it serves while the vault is locked.
	base := startProxy(t)
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}

	req, _ := http.NewRequest("GET", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status=%d, want 503", resp.StatusCode)
	}
}

func TestProxyLogRedactsToken(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "real-secret\n", "add",
		"--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, nil)

	var logs bytes.Buffer
	p := NewProxy("127.0.0.1:0", os.Getenv(EnvHome), os.Getenv(EnvPassphrase), &logs, io.Discard)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/k/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	p.ServeHTTP(w, r)

	out := logs.String()
	if strings.Contains(out, tok) {
		t.Errorf("log leaked capability token: %q", out)
	}
	if strings.Contains(out, "real-secret") {
		t.Errorf("log leaked real secret: %q", out)
	}
	if !strings.Contains(out, "alias=k") {
		t.Errorf("log missing alias tag: %q", out)
	}
}

func TestSplitAliasPath(t *testing.T) {
	tests := []struct {
		in, alias, rest string
		ok              bool
	}{
		{"/openai/v1/chat", "openai", "/v1/chat", true},
		{"/openai/", "openai", "/", true},
		{"/openai", "openai", "/", true},
		{"/", "", "", false},
		{"", "", "", false},
		{"no-slash", "", "", false},
	}
	for _, tc := range tests {
		a, r, ok := splitAliasPath(tc.in)
		if ok != tc.ok || a != tc.alias || r != tc.rest {
			t.Errorf("splitAliasPath(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, a, r, ok, tc.alias, tc.rest, tc.ok)
		}
	}
}

func TestExtractToken(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"Bearer abc123", "abc123", true},
		{"bearer abc123", "abc123", true},
		{"Basic xyz", "", false},
		{"", "", false},
		{"Bearer ", "", false},
	}
	for _, tc := range tests {
		got, ok := extractToken(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("extractToken(%q) = (%q, %v), want (%q, %v)",
				tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestProviderInjectAuth(t *testing.T) {
	tests := []struct {
		style, header, queryKey string
	}{
		{"bearer", "Authorization", ""},
		{"x-api-key", "x-api-key", ""},
		{"header:X-Custom", "X-Custom", ""},
		{"query:api_key", "", "api_key"},
	}
	for _, tc := range tests {
		p := Provider{Name: "t", AuthStyle: tc.style}
		req, _ := http.NewRequest("GET", "http://example.com/foo", nil)
		if err := p.InjectAuth(req, "secret-val"); err != nil {
			t.Errorf("InjectAuth(%q): %s", tc.style, err)
			continue
		}
		switch {
		case tc.header == "Authorization":
			if req.Header.Get("Authorization") != "Bearer secret-val" {
				t.Errorf("style=%q got Authorization=%q", tc.style, req.Header.Get("Authorization"))
			}
		case tc.header != "":
			if req.Header.Get(tc.header) != "secret-val" {
				t.Errorf("style=%q got %s=%q", tc.style, tc.header, req.Header.Get(tc.header))
			}
		case tc.queryKey != "":
			if req.URL.Query().Get(tc.queryKey) != "secret-val" {
				t.Errorf("style=%q got query[%s]=%q", tc.style, tc.queryKey, req.URL.Query().Get(tc.queryKey))
			}
		}
	}
}

func TestProvidersModeLists(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, out, _ := runVault(t, "", "providers")
	if code != 0 {
		t.Fatal("providers")
	}
	for _, want := range []string{"openai", "anthropic", "github", "generic"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in providers output: %q", want, out)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"127.0.0.1:8787", true},
		{"localhost:8787", true},
		{"[::1]:8787", true},
		{"0.0.0.0:8787", false},
		{"192.168.1.1:8787", false},
		{"badport", false},
	}
	for _, tc := range tests {
		if got := isLoopback(tc.in); got != tc.ok {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
}
