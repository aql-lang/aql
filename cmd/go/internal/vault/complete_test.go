package vault

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- capability constraints + rotation -------------------------------------

func TestGrantWithConstraintsPersists(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatal("add")
	}
	code, out, errOut := runVault(t, "", "grant",
		"--agent=claude", "--max-calls=3", "--max-cost-cents=50", "--require-approval", "k")
	if code != 0 {
		t.Fatalf("grant: %s", errOut)
	}
	for _, want := range []string{"max-calls:", "max-cost:", "approval:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in grant output: %q", want, out)
		}
	}
	s, _ := LoadStore(home)
	if len(s.Capabilities) != 1 {
		t.Fatalf("expected one capability")
	}
	c := s.Capabilities[0]
	if c.MaxCalls != 3 || c.MaxCostCents != 50 || !c.RequireApproval {
		t.Errorf("constraints not persisted: %+v", c)
	}
}

func TestProxyExhaustsCallQuotaAfterMaxCalls(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	// Grant with max-calls=2 directly via the store so we don't
	// have to round-trip through the CLI.
	s, _ := LoadStore(home)
	tok, _ := s.NewCapability("k", "test", nil, nil, hour())
	idx := len(s.Capabilities) - 1
	s.Capabilities[idx].MaxCalls = 2
	_ = SaveStore(home, s)

	base := startProxy(t)
	for i := 1; i <= 3; i++ {
		req, _ := http.NewRequest("GET", base+"/k/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok.ID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		switch {
		case i <= 2 && resp.StatusCode != 200:
			t.Errorf("call %d: expected 200, got %d", i, resp.StatusCode)
		case i == 3 && resp.StatusCode != http.StatusTooManyRequests:
			t.Errorf("call 3: expected 429, got %d", resp.StatusCode)
		}
	}

	s, _ = LoadStore(home)
	if s.Capabilities[0].UsedCalls != 2 {
		t.Errorf("UsedCalls = %d, want 2", s.Capabilities[0].UsedCalls)
	}
}

func TestProxyExhaustsCostBudget(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	// Make the upstream report a cost on each call so the proxy
	// debits the capability budget.
	fu.respond = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-AQL-Vault-Cost-Cents", "30")
		w.WriteHeader(200)
	}
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	s, _ := LoadStore(home)
	tok, _ := s.NewCapability("k", "test", nil, nil, hour())
	idx := len(s.Capabilities) - 1
	s.Capabilities[idx].MaxCostCents = 50
	_ = SaveStore(home, s)

	base := startProxy(t)
	for i := 1; i <= 3; i++ {
		req, _ := http.NewRequest("GET", base+"/k/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok.ID)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		switch {
		case i == 1 && resp.StatusCode != 200:
			t.Errorf("call 1: expected 200, got %d", resp.StatusCode)
		case i == 2 && resp.StatusCode != 200:
			// 2nd call should still pass (30 used, budget 50,
			// counter increments after call).
			t.Errorf("call 2: expected 200, got %d", resp.StatusCode)
		case i == 3 && resp.StatusCode != http.StatusPaymentRequired:
			t.Errorf("call 3: expected 402, got %d", resp.StatusCode)
		}
	}
}

func TestProxyDeniesRequireApproval(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	s, _ := LoadStore(home)
	tok, _ := s.NewCapability("k", "test", nil, nil, hour())
	s.Capabilities[len(s.Capabilities)-1].RequireApproval = true
	_ = SaveStore(home, s)

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/k/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok.ID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "approval") {
		t.Errorf("missing approval message: %q", body)
	}
}

func TestRotateReplacesValueAndKeepsMetadata(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "old-val\n", "add", "--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, errOut := runVault(t, "new-val\n", "rotate", "--from-stdin", "k"); code != 0 {
		t.Fatalf("rotate: %s", errOut)
	}
	// Value updated.
	code, out, _ := runVault(t, "", "get", "--reveal", "k")
	if code != 0 || !strings.Contains(out, "new-val") || strings.Contains(out, "old-val") {
		t.Errorf("get after rotate: %q (code=%d)", out, code)
	}
	// Metadata preserved (provider stays).
	s, _ := LoadStore(home)
	a, _ := s.FindAlias("k")
	if a.Provider != "openai" {
		t.Errorf("provider lost after rotate: %+v", a)
	}
	if a.UpdatedAt == "" {
		t.Errorf("UpdatedAt not set after rotate: %+v", a)
	}
}

func TestRotateWithRevokeCaps(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "x\n", "add", "--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatal("add")
	}
	_ = grantOK(t, "k", nil, nil)
	_ = grantOK(t, "k", nil, nil)

	if code, _, errOut := runVault(t, "y\n", "rotate", "--from-stdin", "--revoke-caps", "k"); code != 0 {
		t.Fatalf("rotate: %s", errOut)
	}
	s, _ := LoadStore(home)
	for _, c := range s.Capabilities {
		if !c.Revoked {
			t.Errorf("capability not revoked after rotate --revoke-caps: %+v", c)
		}
	}
}

// --- policy-as-code --------------------------------------------------------

func TestPolicyApplyIsIdempotent(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	// Pre-seed the keyring so the policy doesn't need FromEnv.
	if code, _, _ := runVault(t, "val\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("seed add")
	}

	pol := Policy{Version: 1,
		Aliases: []PolicyAlias{
			{Name: "k", Provider: "openai", Namespace: "team"},
		},
		Capabilities: []PolicyCapability{
			{Alias: "k", Agent: "ci", Hosts: []string{"api.openai.com"},
				Methods: []string{"POST"}, TTL: "1h", MaxCalls: 10},
		},
	}
	path := filepath.Join(home, "pol.json")
	b, _ := json.Marshal(pol)
	_ = os.WriteFile(path, b, 0600)

	code, out, errOut := runVault(t, "", "policy", "apply", path)
	if code != 0 {
		t.Fatalf("apply: %s", errOut)
	}
	if !strings.Contains(out, "+capability") {
		t.Errorf("missing capability change: %q", out)
	}

	// Reapply — should refresh the (alias, agent) capability,
	// not pile up duplicates.
	code, out, _ = runVault(t, "", "policy", "apply", path)
	if code != 0 {
		t.Fatal("reapply")
	}
	if !strings.Contains(out, "~capability") {
		t.Errorf("reapply should refresh existing cap: %q", out)
	}
	s, _ := LoadStore(home)
	// Expect 2 capabilities total (one revoked from first apply,
	// one active from second). ActiveCapabilities returns just
	// one.
	if len(s.ActiveCapabilities(nowUTC())) != 1 {
		t.Errorf("expected 1 active capability after reapply, got %d", len(s.ActiveCapabilities(nowUTC())))
	}
	a, _ := s.FindAlias("k")
	if a.Provider != "openai" || a.Namespace != "team" {
		t.Errorf("alias metadata not updated by apply: %+v", a)
	}
}

func TestPolicyApplyFromEnv(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	t.Setenv("MY_SECRET", "from-env-val")

	pol := Policy{Version: 1, Aliases: []PolicyAlias{
		{Name: "k2", Provider: "openai", FromEnv: "MY_SECRET"},
	}}
	path := filepath.Join(home, "pol.json")
	b, _ := json.Marshal(pol)
	_ = os.WriteFile(path, b, 0600)

	code, out, errOut := runVault(t, "", "policy", "apply", path)
	if code != 0 {
		t.Fatalf("apply: %s", errOut)
	}
	if !strings.Contains(out, "+secret k2") {
		t.Errorf("missing +secret line: %q", out)
	}
	code, out, _ = runVault(t, "", "get", "--reveal", "k2")
	if code != 0 || !strings.Contains(out, "from-env-val") {
		t.Errorf("FromEnv secret not stored: %q", out)
	}
}

func TestPolicyApplyDryRun(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	pol := Policy{Version: 1, Aliases: []PolicyAlias{{Name: "x", Provider: "openai"}}}
	path := filepath.Join(home, "pol.json")
	b, _ := json.Marshal(pol)
	_ = os.WriteFile(path, b, 0600)
	code, out, _ := runVault(t, "", "policy", "apply", "--dry-run", path)
	if code != 0 {
		t.Fatal("dry-run")
	}
	if !strings.Contains(out, "(dry-run") {
		t.Errorf("missing dry-run marker: %q", out)
	}
	s, _ := LoadStore(home)
	if a, _ := s.FindAlias("x"); a != nil {
		t.Error("dry-run should not have written the alias")
	}
}

func TestPolicyShowEmitsValidJSON(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatal("add")
	}
	_ = grantOK(t, "k", []string{"api.openai.com"}, []string{"POST"})

	code, out, _ := runVault(t, "", "policy", "show")
	if code != 0 {
		t.Fatal("show")
	}
	var got Policy
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("policy show output is not valid JSON: %s\n%s", err, out)
	}
	if len(got.Aliases) != 1 || got.Aliases[0].Name != "k" {
		t.Errorf("show: missing alias: %+v", got)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Agent != "test-agent" {
		t.Errorf("show: missing capability: %+v", got)
	}
}

// --- MCP server ------------------------------------------------------------

func mcpRPC(t *testing.T, srv *mcpServer, method string, params any) *mcpResponse {
	t.Helper()
	var p json.RawMessage
	if params != nil {
		p, _ = json.Marshal(params)
	}
	req := &mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: method, Params: p}
	return srv.dispatch(req)
}

func TestMCPInitializeAndPing(t *testing.T) {
	testHome(t)
	mustInit(t)
	srv := &mcpServer{homeDir: os.Getenv(EnvHome), stderr: io.Discard, client: http.DefaultClient}

	resp := mcpRPC(t, srv, "initialize", nil)
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize: %+v", resp)
	}
	resp = mcpRPC(t, srv, "ping", nil)
	if resp == nil || resp.Error != nil {
		t.Fatalf("ping: %+v", resp)
	}
}

func TestMCPListToolsExposesAliases(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "alpha"); code != 0 {
		t.Fatal("add alpha")
	}
	// Alias with no provider should be omitted.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "beta"); code != 0 {
		t.Fatal("add beta")
	}

	srv := &mcpServer{homeDir: os.Getenv(EnvHome), stderr: io.Discard, client: http.DefaultClient}
	resp := mcpRPC(t, srv, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	body := string(b)
	if !strings.Contains(body, "alpha_request") {
		t.Errorf("missing alpha tool: %s", body)
	}
	if strings.Contains(body, "beta_request") {
		t.Errorf("beta (no provider) should be omitted: %s", body)
	}
}

func TestMCPCallToolForwardsAndAuthInjects(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "real-mcp-secret\n", "add",
		"--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}

	srv := &mcpServer{homeDir: os.Getenv(EnvHome), agent: "claude", stderr: io.Discard, client: http.DefaultClient}
	resp := mcpRPC(t, srv, "tools/call", map[string]any{
		"name":      "k_request",
		"arguments": map[string]any{"method": "POST", "path": "/v1/x", "body": `{"a":1}`},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call error: %+v", resp.Error)
	}
	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastAuth != "Bearer real-mcp-secret" {
		t.Errorf("auth not injected: %q", fu.lastAuth)
	}
	if fu.lastMeth != "POST" || fu.lastPath != "/v1/x" || fu.lastBody != `{"a":1}` {
		t.Errorf("upstream did not see expected request: meth=%q path=%q body=%q",
			fu.lastMeth, fu.lastPath, fu.lastBody)
	}
}

func TestMCPCallToolUnknownAlias(t *testing.T) {
	testHome(t)
	mustInit(t)
	srv := &mcpServer{homeDir: os.Getenv(EnvHome), stderr: io.Discard, client: http.DefaultClient}
	resp := mcpRPC(t, srv, "tools/call", map[string]any{"name": "nope_request"})
	if resp.Error == nil {
		t.Fatal("expected error for unknown alias")
	}
}

func TestMCPNotificationProducesNoReply(t *testing.T) {
	testHome(t)
	mustInit(t)
	srv := &mcpServer{homeDir: os.Getenv(EnvHome), stderr: io.Discard, client: http.DefaultClient}
	req := &mcpRequest{JSONRPC: "2.0", Method: "notifications/initialized"} // no ID
	if resp := srv.dispatch(req); resp != nil {
		t.Errorf("notification should not produce a reply, got %+v", resp)
	}
}

func TestMCPServeLoopHandlesMultipleRequests(t *testing.T) {
	testHome(t)
	mustInit(t)
	srv := &mcpServer{homeDir: os.Getenv(EnvHome), stderr: io.Discard, client: http.DefaultClient}
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n",
	)
	var out bytes.Buffer
	srv.serve(in, &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 replies, got %d (%q)", len(lines), out.String())
	}
	for _, line := range lines {
		var resp mcpResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Errorf("non-JSON reply: %q", line)
		}
		if resp.Error != nil {
			t.Errorf("reply error: %+v", resp.Error)
		}
	}
}

// --- shared helpers --------------------------------------------------------

// hour and nowUTC are tiny aliases used by capability tests so
// they don't have to import time in this file's main path.
func hour() time.Duration { return time.Hour }
func nowUTC() time.Time   { return time.Now().UTC() }
