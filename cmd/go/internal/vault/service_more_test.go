package vault

// service_more_test.go — the ProxyService lifecycle wrapper and the
// long-running entry points' argument/refusal edges (proxy, mcp, -i).

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

func TestNewProxyServiceRefusals(t *testing.T) {
	testHome(t)
	// Not initialized.
	if _, err := NewProxyService("", "", ""); err == nil ||
		!strings.Contains(err.Error(), "not initialized") {
		t.Errorf("uninitialized: %v", err)
	}
	mustInit(t)
	// Locked.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if _, err := NewProxyService("", "", ""); err == nil ||
		!strings.Contains(err.Error(), "locked") {
		t.Errorf("locked: %v", err)
	}
}

func TestProxyServiceLifecycle(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	ps, err := NewProxyService("127.0.0.1:0", home, "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	if ps.Name() != "vault-proxy" || ps.UsesStdio() {
		t.Error("identity accessors wrong")
	}
	if ps.Status() != service.StateStopped {
		t.Errorf("initial state = %v", ps.Status())
	}
	// Before Start, Addr echoes the configured listen address.
	if ps.Addr() != "127.0.0.1:0" {
		t.Errorf("pre-start Addr = %q", ps.Addr())
	}
	md := ps.Metadata()
	if md["addr"] != "127.0.0.1:0" || md["home"] != home {
		t.Errorf("metadata = %v", md)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ps.Start(ctx) }()

	// Wait for it to come up.
	deadline := time.Now().Add(5 * time.Second)
	for ps.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("service did not reach running")
		}
		time.Sleep(5 * time.Millisecond)
	}
	addr := ps.Addr()
	if addr == "127.0.0.1:0" || addr == "" {
		t.Fatalf("resolved Addr = %q", addr)
	}
	get := func() int {
		resp, err := http.Get("http://" + addr + "/nope/x")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	// Live: the proxy answers (an unauthenticated request is denied, not 503).
	if code := get(); code == http.StatusServiceUnavailable {
		t.Errorf("live service returned 503")
	}
	// Paused: everything is 503.
	if err := ps.Pause(context.Background()); err != nil || ps.Status() != service.StatePaused {
		t.Fatalf("pause: %v state=%v", err, ps.Status())
	}
	if code := get(); code != http.StatusServiceUnavailable {
		t.Errorf("paused service returned %d, want 503", code)
	}
	if err := ps.Resume(context.Background()); err != nil || ps.Status() != service.StateRunning {
		t.Fatalf("resume: %v state=%v", err, ps.Status())
	}
	if code := get(); code == http.StatusServiceUnavailable {
		t.Errorf("resumed service still 503")
	}
	// Stop shuts it down gracefully; Start returns nil.
	if err := ps.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned %v after Stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	if ps.Status() != service.StateStopped {
		t.Errorf("final state = %v", ps.Status())
	}
}

func TestProxyServiceStartListenError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Occupy a port, then ask the service to bind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ps, err := NewProxyService(ln.Addr().String(), home, "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.Start(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "listen") {
		t.Errorf("bound-port Start = %v", err)
	}
	if ps.Status() != service.StateStopped {
		t.Errorf("state after failed start = %v", ps.Status())
	}
	// Stopping a never-started service is a no-op.
	if err := ps.Stop(context.Background()); err != nil {
		t.Errorf("stop before start: %v", err)
	}
}

func TestNopWriter(t *testing.T) {
	n, err := nopWriter{}.Write([]byte("xyz"))
	if n != 3 || err != nil {
		t.Errorf("nopWriter = %d, %v", n, err)
	}
}

func TestRunProxyRefusals(t *testing.T) {
	testHome(t)
	// Non-loopback without --allow-public.
	if code, _, errOut := runVault(t, "", "proxy", "--listen=0.0.0.0:8787"); code == 0 ||
		!strings.Contains(errOut, "loopback") {
		t.Errorf("public listen: %q", errOut)
	}
	// Not initialized.
	if code, _, errOut := runVault(t, "", "proxy"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("uninitialized proxy: %q", errOut)
	}
	mustInit(t)
	// Locked.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "proxy"); code == 0 || !strings.Contains(errOut, "locked") {
		t.Errorf("locked proxy: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// An unbindable port surfaces as a serve error (and covers the
	// --allow-public warning branch).
	code, _, errOut := runVault(t, "", "proxy", "--allow-public", "--listen=0.0.0.0:99999")
	if code == 0 {
		t.Error("unbindable listen should fail")
	}
	if !strings.Contains(errOut, "warning") {
		t.Errorf("--allow-public warning missing: %q", errOut)
	}
}

func TestRunMCPEdges(t *testing.T) {
	testHome(t)
	if code, _, errOut := runVault(t, "", "mcp"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("uninitialized mcp: %q", errOut)
	}
	mustInit(t)
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "mcp"); code == 0 || !strings.Contains(errOut, "locked") {
		t.Errorf("locked mcp: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// A garbage frame is logged and skipped; a ping round-trips; EOF ends
	// the loop with success.
	stdin := "{oops\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	code, out, errOut := runVault(t, stdin, "mcp", "--agent=tester")
	if code != 0 {
		t.Fatalf("mcp run: %d %q", code, errOut)
	}
	if !strings.Contains(errOut, "bad request") {
		t.Errorf("bad frame not logged: %q", errOut)
	}
	if !strings.Contains(out, `"id":1`) {
		t.Errorf("ping reply missing: %q", out)
	}
	// Flag errors are reported.
	if code, _, _ := runVault(t, "", "mcp", "--bogus-flag"); code == 0 {
		t.Error("bad flag should fail")
	}
}

func TestRunInteractiveRequiresTTY(t *testing.T) {
	testHome(t)
	mustInit(t)
	// runVault's stdin is a strings.Reader, never a terminal.
	code, _, errOut := runVault(t, "", "-i")
	if code == 0 || !strings.Contains(errOut, "requires a terminal") {
		t.Errorf("-i without a tty: %d %q", code, errOut)
	}
	if code, _, _ := runVault(t, "", "--interactive", "--bogus"); code == 0 {
		t.Error("bad -i flag should fail")
	}
}

func TestResolveLaunchVaultAndTTYHelpers(t *testing.T) {
	home := testHome(t)
	// An explicit folder/suffix in the env wins.
	t.Setenv(EnvFolder, home)
	t.Setenv(EnvSuffix, "work")
	folder, suffix, ok := resolveLaunchVault(home)
	if !ok || folder != home || suffix != "work" {
		t.Errorf("explicit location = %q %q %v", folder, suffix, ok)
	}
	// Without an override there is nothing to open in a fresh home.
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	if _, _, ok := resolveLaunchVault(home); ok {
		t.Error("fresh home should have no launch vault")
	}
	// Non-file readers/writers are never a TTY.
	if interactiveTTY(strings.NewReader(""), io.Discard) {
		t.Error("a strings.Reader is not a tty")
	}
}

func TestSetVersionRoundTrip(t *testing.T) {
	old := cliVersion
	defer SetVersion(old)
	SetVersion("1.2.3")
	if headerVersion() != "v1.2.3" {
		t.Errorf("headerVersion = %q", headerVersion())
	}
	SetVersion("")
	if headerVersion() != "" {
		t.Errorf("empty version should render empty, got %q", headerVersion())
	}
}
