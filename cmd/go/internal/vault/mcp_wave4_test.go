package vault

// mcp_wave4_test.go — mcp.go arms: the serve loop's skip/encode-failure
// paths, dispatch notifications and unknown methods, tools/list error
// and collision handling, and callTool's denial / lookup / transport
// failures.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMCPServeLoopArms(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Blank lines are skipped; a notification produces no reply; a bad
	// JSON line only warns.
	in := "\n" +
		"not-json\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	var out, errb strings.Builder
	code := Run([]string{"mcp"}, strings.NewReader(in), &out, &errb)
	if code != 0 {
		t.Fatalf("mcp run: %d, %q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "bad request") {
		t.Errorf("bad JSON line not warned: %q", errb.String())
	}
	if got := strings.Count(out.String(), "\n"); got != 1 {
		t.Errorf("expected exactly one response line, got %d:\n%s", got, out.String())
	}
	_ = home

	// A failing stdout aborts the serve loop after the first response.
	errb.Reset()
	code = Run([]string{"mcp"}, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"+
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n"), w4FailWriter{}, &errb)
	if code != 0 {
		t.Fatalf("mcp with failing stdout: %d", code)
	}
	if !strings.Contains(errb.String(), "encoding response") {
		t.Errorf("encode failure not reported: %q", errb.String())
	}
}

// w4Dispatch runs one request through dispatch on a fresh server.
func w4Dispatch(t *testing.T, srv *mcpServer, id, method, params string) *mcpResponse {
	t.Helper()
	req := &mcpRequest{JSONRPC: "2.0", Method: method}
	if id != "" {
		req.ID = json.RawMessage(id)
	}
	if params != "" {
		req.Params = json.RawMessage(params)
	}
	return srv.dispatch(req)
}

func TestMCPDispatchArms(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}

	if resp := w4Dispatch(t, srv, "", "totally/unknown", ""); resp != nil {
		t.Errorf("unknown notification should be swallowed, got %+v", resp)
	}
	resp := w4Dispatch(t, srv, "1", "totally/unknown", "")
	if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, "method not found") {
		t.Errorf("unknown method = %+v", resp)
	}

	// tools/list fails on a corrupt store, and reports an uninitialized one.
	w4CorruptStore(t, home)
	if resp := w4Dispatch(t, srv, "2", "tools/list", ""); resp == nil || resp.Error == nil {
		t.Errorf("tools/list on a corrupt store = %+v", resp)
	}
	empty := t.TempDir()
	srv2 := &mcpServer{homeDir: empty, agent: "mcp", stderr: io.Discard}
	if resp := w4Dispatch(t, srv2, "3", "tools/list", ""); resp == nil || resp.Error == nil ||
		!strings.Contains(resp.Error.Message, "not initialized") {
		t.Errorf("tools/list without a vault = %+v", resp)
	}
}

// w4GrantAgentCap grants a capability for alias to the "mcp" agent with
// the given method allowlist.
func w4GrantAgentCap(t *testing.T, home, alias string, methods []string) {
	t.Helper()
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.NewCapability(alias, "mcp", nil, methods, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
}

func TestMCPToolCollisionsAndListWarnings(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "w4-mcp", fu, "bearer")
	for _, a := range []string{"proj:key", "proj_key"} {
		if code, _, e := runVault(t, "s\n", "add", "--from-stdin", "--provider=w4-mcp", a); code != 0 {
			t.Fatalf("seed %s: %s", a, e)
		}
		w4GrantAgentCap(t, home, a, nil)
	}
	var errb strings.Builder
	srv := &mcpServer{homeDir: home, agent: "mcp", stderr: &errb}
	tools, err := srv.listTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Errorf("collision should leave one tool, got %d", len(tools))
	}
	if !strings.Contains(errb.String(), "collides") {
		t.Errorf("collision not warned: %q", errb.String())
	}
}

func TestMCPCallToolArms(t *testing.T) {
	call := func(t *testing.T, srv *mcpServer, params string) *mcpResponse {
		t.Helper()
		return w4Dispatch(t, srv, "9", "tools/call", params)
	}
	expectErr := func(t *testing.T, resp *mcpResponse, want string) {
		t.Helper()
		if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, want) {
			t.Errorf("response = %+v, want error containing %q", resp, want)
		}
	}

	t.Run("bad params and names", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		expectErr(t, call(t, srv, `"not-an-object"`), "invalid params")
		expectErr(t, call(t, srv, `{"name":"nosuffix"}`), "must end with _request")
		expectErr(t, call(t, srv, `{"name":"missing_request"}`), "unknown alias")
	})
	t.Run("corrupt and locked stores", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		expectErr(t, call(t, srv, `{"name":"k_request"}`), "locked")
		w4CorruptStore(t, home)
		expectErr(t, call(t, srv, `{"name":"k_request"}`), "parsing")
	})
	t.Run("no provider", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		// generic (providerless) alias resolves via the literal fallback.
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "plain"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		expectErr(t, call(t, srv, `{"name":"plain_request"}`), "no provider preset")
	})
	t.Run("no capability", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-mcp2", fu, "bearer")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w4-mcp2", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		expectErr(t, call(t, srv, `{"name":"k_request"}`), "no capability")
	})
	t.Run("denied, path fixup, bad method, injection, upstream", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-mcp3", fu, "bearer")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w4-mcp3", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4GrantAgentCap(t, home, "k", []string{"GET"})
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		srv.client = fu.Client()

		// Method allowlist denial through the shared capabilityDenial.
		expectErr(t, call(t, srv, `{"name":"k_request","arguments":{"method":"POST","path":"/x"}}`), "capability denied")
		// A second alias without a method allowlist: an invalid HTTP
		// method reaches (and fails) request building; it also exercises
		// the tool-walk's continue arm.
		if code, _, e := runVault(t, "v2\n", "add", "--from-stdin", "--provider=w4-mcp3", "aaa"); code != 0 {
			t.Fatalf("seed aaa: %s", e)
		}
		w4GrantAgentCap(t, home, "aaa", nil)
		expectErr(t, call(t, srv, `{"name":"aaa_request","arguments":{"method":"GE T","path":"/x"}}`), "building request")
		// A leading-slash-free path is fixed up; query args are encoded.
		resp := call(t, srv, `{"name":"k_request","arguments":{"path":"things","query":{"q":"a b"}}}`)
		if resp == nil || resp.Error != nil {
			t.Fatalf("call with query = %+v", resp)
		}
		if fu.lastPath != "/things" {
			t.Errorf("path fixup: upstream saw %q", fu.lastPath)
		}
		// Injection failure via a bogus auth style.
		registerTestProvider(t, "w4-mcp3", fu, "w4-bogus-style")
		expectErr(t, call(t, srv, `{"name":"k_request","arguments":{"path":"/x"}}`), "credential injection failed")
		// Upstream unreachable.
		prev := providers["w4-mcp3"]
		providers["w4-mcp3"] = Provider{Name: "w4-mcp3", BaseURL: "http://127.0.0.1:1", AuthStyle: "bearer"}
		t.Cleanup(func() { providers["w4-mcp3"] = prev })
		expectErr(t, call(t, srv, `{"name":"k_request","arguments":{"path":"/x"}}`), "upstream")
	})
	t.Run("keyring unavailable and secret lookup failure", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-mcp4", fu, "bearer")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w4-mcp4", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4GrantAgentCap(t, home, "k", nil)
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
		srv.client = fu.Client()
		// No passphrase: per-call authenticate fails.
		t.Setenv(EnvPassphrase, "")
		expectErr(t, call(t, srv, `{"name":"k_request","arguments":{"path":"/x"}}`), "keyring unavailable")
		// Value missing from the keyring.
		t.Setenv(EnvPassphrase, "test-pass")
		kr := &fileKeyring{folder: vaultFolder(home), pass: "test-pass"}
		if err := kr.Delete("k"); err != nil {
			t.Fatal(err)
		}
		expectErr(t, call(t, srv, `{"name":"k_request","arguments":{"path":"/x"}}`), "secret lookup failed")
	})
	t.Run("counter persistence failure is a warning", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-mcp5", fu, "bearer")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w4-mcp5", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4GrantAgentCap(t, home, "k", nil)
		w4BlockLock(t, home)
		var errb strings.Builder
		srv := &mcpServer{homeDir: home, agent: "mcp", stderr: &errb}
		srv.client = fu.Client()
		resp := call(t, srv, `{"name":"k_request","arguments":{"path":"/x"}}`)
		if resp == nil || resp.Error != nil {
			t.Fatalf("call should succeed despite counter failure: %+v", resp)
		}
		if !strings.Contains(errb.String(), "persisting capability counters") {
			t.Errorf("counter failure not warned: %q", errb.String())
		}
	})
}

func TestMCPStringArgNil(t *testing.T) {
	if got := stringArg(nil, "method", "GET"); got != "GET" {
		t.Errorf("stringArg(nil) = %q", got)
	}
	if got := fmt.Sprint(stringArg(map[string]any{"n": 3}, "n", "d")); got != "d" {
		t.Errorf("stringArg non-string = %q", got)
	}
}
