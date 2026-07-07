package vault

// mcp_proxy_seam8_test.go — wave-8 coverage for two request-building arms:
// the MCP request handler appending a query to a path that already carries one
// (sep "&"), and the proxy's http.NewRequestWithContext failure arm (an invalid
// method token, reachable because a method-unrestricted capability does not
// pre-filter the method).

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestW8_MCPRequestAppendsToExistingQuery drives the callTool query-append arm
// where the path already contains "?", so the separator is "&".
func TestW8_MCPRequestAppendsToExistingQuery(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "w8-mcp", fu, "bearer")
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w8-mcp", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	w4GrantAgentCap(t, home, "k", nil) // no method restriction
	srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard}
	srv.client = fu.Client()

	resp := w4Dispatch(t, srv, "1", "tools/call",
		`{"name":"k_request","arguments":{"path":"/x?existing=1","query":{"q":"b"}}}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("request appending to an existing query = %+v", resp)
	}
}

// TestW8_ProxyBuildRequestError drives the proxy's http.NewRequestWithContext
// error arm: an invalid method token fails request building after the (method-
// unrestricted) capability check passes.
func TestW8_ProxyBuildRequestError(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "w8-proxy", fu, "bearer")
	if code, _, _ := runVault(t, "real-secret\n", "add", "--from-stdin", "--provider=w8-proxy", "k"); code != 0 {
		t.Fatal("seed add")
	}
	tok := grantOK(t, "k", nil, nil)

	p := NewProxy("127.0.0.1:0", os.Getenv(EnvHome), os.Getenv(EnvPassphrase), io.Discard, io.Discard)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/k/x", nil)
	r.Method = "GE T" // invalid HTTP method token (space) -> NewRequest fails
	r.Header.Set("Authorization", "Bearer "+tok)
	p.ServeHTTP(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("build-request failure should be 502 Bad Gateway, got %d", w.Code)
	}
}
