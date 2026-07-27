package vault

// provider_cli_test.go — the `vault provider` mode family and the
// store-defined provider preset machinery it fronts: CLI add/rm/list
// with every refusal arm, the LookupProviderIn/ListProvidersIn
// resolution rules, mint-time validation, the v7 store migration, and
// the proxy/MCP brokering end-to-end through a custom preset. Each
// positive case is paired with the negative that must be rejected.

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestProviderAddBrokersThroughProxy is the end-to-end proof: a preset
// minted with `provider add` routes and injects exactly like a built-in
// — the proxy resolves the alias through the store, forwards to the
// custom base URL, and attaches the secret in the declared style.
func TestProviderAddBrokersThroughProxy(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)

	// Mint with a trailing slash (must be trimmed) and an http URL
	// (must warn — the secret would travel unencrypted).
	code, out, errOut := runVault(t, "", "provider", "add",
		"--url="+fu.URL+"/", "--auth-style=x-api-key", "corp")
	if code != 0 {
		t.Fatalf("provider add: %s", errOut)
	}
	if !strings.Contains(out, `added provider "corp"`) || strings.Contains(out, fu.URL+"/") {
		t.Errorf("add output should confirm with the trimmed URL: %q", out)
	}
	if !strings.Contains(errOut, "plain http") {
		t.Errorf("http base URL should warn on stderr: %q", errOut)
	}

	// The listing shows it, marked custom, next to the built-ins.
	code, out, _ = runVault(t, "", "providers")
	if code != 0 {
		t.Fatal("providers list")
	}
	if !strings.Contains(out, "corp") || !strings.Contains(out, "custom") ||
		!strings.Contains(out, "openai") || !strings.Contains(out, "builtin") {
		t.Errorf("list should show the custom preset beside the built-ins: %q", out)
	}

	// An alias tagged with the custom preset brokers through the proxy.
	if code, _, errOut := runVault(t, "real-secret-77\n", "add",
		"--from-stdin", "--provider=corp", "k"); code != 0 {
		t.Fatalf("alias add: %s", errOut)
	}
	tok := grantOK(t, "k", nil, nil)
	base := startProxy(t)

	req, _ := http.NewRequest("GET", base+"/k/v1/things?q=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy status = %d, body %q", resp.StatusCode, body)
	}
	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastPath != "/v1/things" {
		t.Errorf("upstream path = %q, want /v1/things", fu.lastPath)
	}
	if fu.lastKey != "real-secret-77" {
		t.Errorf("upstream x-api-key = %q, want the real secret", fu.lastKey)
	}
	if fu.lastAuth != "" {
		t.Errorf("capability token leaked upstream as Authorization: %q", fu.lastAuth)
	}
}

// TestProviderAddHTTPSNoWarning pairs the http-warning case: an https
// preset mints silently.
func TestProviderAddHTTPSNoWarning(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, out, errOut := runVault(t, "", "provider", "add",
		"--url=https://api.corp.example/v2", "corp")
	if code != 0 {
		t.Fatalf("provider add: %s", errOut)
	}
	if !strings.Contains(out, "https://api.corp.example/v2") {
		t.Errorf("add should echo the URL: %q", out)
	}
	if strings.Contains(errOut, "plain http") {
		t.Errorf("https must not warn: %q", errOut)
	}
}

// TestProviderAddUpdatesInPlace re-adding a taken name replaces the
// preset (reports "updated") instead of duplicating it.
func TestProviderAddUpdatesInPlace(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "", "provider", "add", "--url=https://one.example", "corp"); code != 0 {
		t.Fatal("first add")
	}
	code, out, _ := runVault(t, "", "provider", "add", "--url=https://two.example", "corp")
	if code != 0 || !strings.Contains(out, `updated provider "corp"`) {
		t.Fatalf("re-add should update: code=%d out=%q", code, out)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.CustomProviders) != 1 {
		t.Fatalf("expected 1 custom preset after re-add, got %d", len(s.CustomProviders))
	}
	if s.CustomProviders[0].BaseURL != "https://two.example" {
		t.Errorf("re-add did not replace the URL: %q", s.CustomProviders[0].BaseURL)
	}
}

// TestProviderAddRejections walks every mint-time refusal arm. Each
// entry must fail with the fragment named — nothing may reach the store.
func TestProviderAddRejections(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no url", []string{"provider", "add", "corp"}, "usage"},
		{"no name", []string{"provider", "add", "--url=https://x.example"}, "usage"},
		{"two names", []string{"provider", "add", "--url=https://x.example", "a", "b"}, "usage"},
		{"bad name", []string{"provider", "add", "--url=https://x.example", "bad name!"}, "invalid provider name"},
		{"colon name", []string{"provider", "add", "--url=https://x.example", "a:b"}, "invalid provider name"},
		{"builtin name", []string{"provider", "add", "--url=https://x.example", "openai"}, "built-in preset"},
		{"generic name", []string{"provider", "add", "--url=https://x.example", "generic"}, "built-in preset"},
		{"no scheme", []string{"provider", "add", "--url=api.example.com", "corp"}, "http:// or https://"},
		{"ftp scheme", []string{"provider", "add", "--url=ftp://x.example", "corp"}, "http:// or https://"},
		{"no host", []string{"provider", "add", "--url=https://", "corp"}, "no host"},
		{"unparseable url", []string{"provider", "add", "--url=https://bad host/", "corp"}, "invalid base URL"},
		{"userinfo in url", []string{"provider", "add", "--url=https://u:p@x.example", "corp"}, "must not embed credentials"},
		{"query in url", []string{"provider", "add", "--url=https://x.example?a=b", "corp"}, "query or fragment"},
		{"forcequery in url", []string{"provider", "add", "--url=https://x.example?", "corp"}, "query or fragment"},
		{"fragment in url", []string{"provider", "add", "--url=https://x.example#frag", "corp"}, "query or fragment"},
		{"bad auth style", []string{"provider", "add", "--url=https://x.example", "--auth-style=bogus", "corp"}, "unknown auth style"},
		{"empty header style", []string{"provider", "add", "--url=https://x.example", "--auth-style=header:", "corp"}, "needs a header name"},
		{"bad header name", []string{"provider", "add", "--url=https://x.example", "--auth-style=header:X Key", "corp"}, "not a valid HTTP header name"},
		{"empty query style", []string{"provider", "add", "--url=https://x.example", "--auth-style=query:", "corp"}, "needs a parameter name"},
	}
	for _, tc := range cases {
		code, _, errOut := runVault(t, "", tc.args...)
		if code == 0 {
			t.Errorf("%s: expected failure", tc.name)
			continue
		}
		if !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: stderr %q should contain %q", tc.name, errOut, tc.want)
		}
	}
	// Nothing above may have reached the store.
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.CustomProviders) != 0 {
		t.Errorf("rejected adds leaked into the store: %+v", s.CustomProviders)
	}
}

// TestProviderFlagParseErrors: an unknown flag fails each sub-mode's
// flag parse cleanly (exit 1, flag error on stderr) instead of being
// swallowed as a positional.
func TestProviderFlagParseErrors(t *testing.T) {
	testHome(t)
	for _, args := range [][]string{
		{"provider", "add", "--bogus-flag", "corp"},
		{"provider", "rm", "--bogus-flag", "corp"},
		{"provider", "list", "--bogus-flag"},
	} {
		code, _, errOut := runVault(t, "", args...)
		if code == 0 || !strings.Contains(errOut, "bogus-flag") {
			t.Errorf("%v: code=%d err=%q, want a flag-parse failure", args, code, errOut)
		}
	}
}

// TestTUIControllerCustomProvidersLenient: the TUI picker helper
// degrades to nil (built-ins only) on a missing store and surfaces the
// vault's presets once one exists.
func TestTUIControllerCustomProvidersLenient(t *testing.T) {
	// Missing store: nil, no error.
	if got := newTUIController(t.TempDir()).customProviders(); got != nil {
		t.Errorf("customProviders without a vault = %+v, want nil", got)
	}
	// With a vault holding a preset: surfaced.
	home := testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "", "provider", "add", "--url=https://x.example", "corp"); code != 0 {
		t.Fatal("provider add")
	}
	got := newTUIController(home).customProviders()
	if len(got) != 1 || got[0].Name != "corp" {
		t.Errorf("customProviders = %+v, want the corp preset", got)
	}
}

// TestProviderAddRmRequireVaultState pairs the vault-state guards: both
// mutations refuse an uninitialized vault and a locked one.
func TestProviderAddRmRequireVaultState(t *testing.T) {
	testHome(t)
	// Uninitialized.
	if code, _, errOut := runVault(t, "", "provider", "add", "--url=https://x.example", "corp"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("add on uninitialized vault: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runVault(t, "", "provider", "rm", "corp"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("rm on uninitialized vault: code=%d err=%q", code, errOut)
	}
	// Locked.
	mustInit(t)
	if code, _, _ := runVault(t, "", "provider", "add", "--url=https://x.example", "corp"); code != 0 {
		t.Fatal("add before lock")
	}
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "provider", "add", "--url=https://y.example", "corp2"); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("add on locked vault: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runVault(t, "", "provider", "rm", "corp"); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("rm on locked vault: code=%d err=%q", code, errOut)
	}
}

// TestProviderRemove covers the removal arms: refusing built-ins,
// refusing a preset still referenced by aliases (naming the blockers),
// missing preset, bare usage — and the clean removal once unreferenced.
func TestProviderRemove(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "", "provider", "add", "--url=https://x.example", "corp"); code != 0 {
		t.Fatal("add provider")
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=corp", "k1"); code != 0 {
		t.Fatal("add alias")
	}

	if code, _, errOut := runVault(t, "", "provider", "rm"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("bare rm: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runVault(t, "", "provider", "rm", "openai"); code == 0 ||
		!strings.Contains(errOut, "cannot be removed") {
		t.Errorf("rm builtin: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runVault(t, "", "provider", "rm", "nope"); code == 0 ||
		!strings.Contains(errOut, `no custom provider named "nope"`) {
		t.Errorf("rm missing: code=%d err=%q", code, errOut)
	}
	// Still referenced: refused, and the blocker is named.
	if code, _, errOut := runVault(t, "", "provider", "rm", "corp"); code == 0 ||
		!strings.Contains(errOut, "still used") || !strings.Contains(errOut, "k1") {
		t.Errorf("rm while referenced: code=%d err=%q", code, errOut)
	}
	// Retag the alias out of the way; removal then succeeds and the
	// listing forgets the preset.
	if code, _, _ := runVault(t, "", "rm", "--yes", "k1"); code != 0 {
		t.Fatal("rm alias")
	}
	code, out, errOut := runVault(t, "", "provider", "rm", "corp")
	if code != 0 || !strings.Contains(out, `removed provider "corp"`) {
		t.Fatalf("rm: code=%d out=%q err=%q", code, out, errOut)
	}
	if _, out, _ := runVault(t, "", "provider", "list"); strings.Contains(out, "corp") {
		t.Errorf("removed preset still listed: %q", out)
	}
}

// TestProviderListEdges: bare and `list` both list; an unknown
// subcommand fails loud; an uninitialized home lists just the built-ins
// (the listing must work before `vault init`); a corrupt store is an
// error, not an empty listing.
func TestProviderListEdges(t *testing.T) {
	home := testHome(t)
	// Uninitialized: built-ins only, exit 0.
	code, out, _ := runVault(t, "", "provider", "list")
	if code != 0 || !strings.Contains(out, "openai") {
		t.Fatalf("uninitialized list: code=%d out=%q", code, out)
	}
	if strings.Contains(out, "custom") {
		t.Errorf("uninitialized list conjured a custom row: %q", out)
	}
	// Unknown subcommand.
	if code, _, errOut := runVault(t, "", "provider", "bogus"); code == 0 ||
		!strings.Contains(errOut, "unknown provider subcommand") {
		t.Errorf("unknown subcommand: code=%d err=%q", code, errOut)
	}
	// Corrupt store: fail loud.
	mustInit(t)
	if err := os.WriteFile(StorePath(home), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runVault(t, "", "providers"); code == 0 || errOut == "" {
		t.Errorf("corrupt store should fail the listing: code=%d err=%q", code, errOut)
	}
}

// TestLookupProviderIn locks the resolution rules: built-ins first (a
// store entry can never shadow one), then customs, then generic; a nil
// store degrades to the built-in lookup; and — the security-critical
// case — a smuggled entry with an un-mintable name (empty, built-in
// collision, colon) is inert, so the empty tag every untagged alias
// carries can never be captured to redirect a secret.
func TestLookupProviderIn(t *testing.T) {
	s := &Store{CustomProviders: []Provider{
		{Name: "corp", BaseURL: "https://api.corp.example", AuthStyle: "bearer"},
		{Name: "openai", BaseURL: "https://evil.example", AuthStyle: "bearer"},  // smuggled shadow of a built-in
		{Name: "", BaseURL: "https://attacker.example", AuthStyle: "bearer"},    // smuggled empty-name capture
		{Name: "a:b", BaseURL: "https://attacker.example", AuthStyle: "bearer"}, // smuggled colon name
	}}
	if got := LookupProviderIn(s, "corp"); got.BaseURL != "https://api.corp.example" {
		t.Errorf("custom lookup = %+v", got)
	}
	if got := LookupProviderIn(s, "openai"); got.BaseURL != "https://api.openai.com" {
		t.Errorf("a smuggled store entry shadowed a built-in: %+v", got)
	}
	// The empty name is the tag every provider-untagged alias carries: it
	// must resolve to the URL-less generic preset, never the smuggled URL.
	if got := LookupProviderIn(s, ""); got.BaseURL != "" {
		t.Errorf("empty-name smuggled preset captured untagged aliases: %+v", got)
	}
	if got := LookupProviderIn(s, "a:b"); got.BaseURL != "" {
		t.Errorf("smuggled colon-name preset resolved: %+v", got)
	}
	if got := LookupProviderIn(s, "nope"); got.Name != "generic" || got.BaseURL != "" {
		t.Errorf("unknown name should fall back to generic: %+v", got)
	}
	if got := LookupProviderIn(nil, "anthropic"); got.BaseURL != "https://api.anthropic.com" {
		t.Errorf("nil store should resolve built-ins: %+v", got)
	}
	if got := LookupProviderIn(nil, "corp"); got.Name != "generic" {
		t.Errorf("nil store custom lookup should fall back to generic: %+v", got)
	}
}

// TestListProvidersIn: merged and sorted, the smuggled built-in
// collision dropped, nil store = built-ins only.
func TestListProvidersIn(t *testing.T) {
	s := &Store{CustomProviders: []Provider{
		{Name: "corp", BaseURL: "https://api.corp.example"},
		{Name: "github", BaseURL: "https://evil.example"}, // smuggled built-in shadow
		{Name: "", BaseURL: "https://evil.example"},       // smuggled empty name
	}}
	got := ListProvidersIn(s)
	names := make([]string, 0, len(got))
	sawEvil := false
	for _, p := range got {
		names = append(names, p.Name)
		if p.BaseURL == "https://evil.example" {
			sawEvil = true
		}
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "corp") {
		t.Errorf("custom preset missing from merged list: %v", names)
	}
	if sawEvil {
		t.Errorf("smuggled github shadow survived into the listing: %v", got)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("listing not sorted: %v", names)
		}
	}
	if n := len(ListProvidersIn(nil)); n != len(ListProviders()) {
		t.Errorf("nil store listing = %d entries, want the %d built-ins", n, len(ListProviders()))
	}
}

// TestValidateProviderBaseURLCanonicalizes pairs the CLI rejection
// table with the accepted shapes: scheme+host survive, a path is kept,
// trailing slashes are trimmed.
func TestValidateProviderBaseURLCanonicalizes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.com", "https://api.example.com"},
		{"https://api.example.com/", "https://api.example.com"},
		{"https://api.example.com/v2/", "https://api.example.com/v2"},
		{"http://127.0.0.1:8080/base", "http://127.0.0.1:8080/base"},
		// A percent-encoded slash in the path must survive the
		// trailing-slash trim rather than being silently decoded to a
		// path separator (which would address a different resource).
		{"https://api.example.com/a%2Fb/", "https://api.example.com/a%2Fb"},
	}
	for _, tc := range cases {
		got, err := validateProviderBaseURL(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("validateProviderBaseURL(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

// TestValidHeaderFieldName pairs accepted RFC-7230 header tokens with
// the shapes net/http would reject at write time.
func TestValidHeaderFieldName(t *testing.T) {
	for _, ok := range []string{"X-Api-Key", "Authorization", "a", "X_Corp.Key-1"} {
		if !validHeaderFieldName(ok) {
			t.Errorf("validHeaderFieldName(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "X Key", "X:Key", "X\tKey", "X\nKey", "naïve"} {
		if validHeaderFieldName(bad) {
			t.Errorf("validHeaderFieldName(%q) = true, want false", bad)
		}
	}
}

// TestValidCustomProviderName: a legit segment passes; empty, colon, and
// built-in names are rejected (the guard that neutralizes a smuggled
// empty-name capture).
func TestValidCustomProviderName(t *testing.T) {
	for _, ok := range []string{"corp", "corp-2", "a.b_c"} {
		if !validCustomProviderName(ok) {
			t.Errorf("validCustomProviderName(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "a:b", "openai", "generic", "bad name"} {
		if validCustomProviderName(bad) {
			t.Errorf("validCustomProviderName(%q) = true, want false", bad)
		}
	}
}

// TestValidateAuthStyleAccepts pairs the CLI rejection table with the
// accepted styles, including the parameterized forms.
func TestValidateAuthStyleAccepts(t *testing.T) {
	for _, ok := range []string{"", "bearer", "x-api-key", "none", "header:X-Api-Key", "query:api_key"} {
		if err := validateAuthStyle(ok); err != nil {
			t.Errorf("validateAuthStyle(%q) = %v, want nil", ok, err)
		}
	}
}

// TestLoadMigratesV6Store: the v6->v7 bump is a clean no-op — version
// advances, aliases survive, and no custom presets are conjured.
func TestLoadMigratesV6Store(t *testing.T) {
	data, err := json.Marshal(Store{Version: 6, Backend: "file",
		Aliases: []Alias{{Name: "k", Provider: "corp", CreatedAt: "2026-01-01T00:00:00Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := writeStoreFile(t, data)
	s, err := LoadStore(dir)
	if err != nil {
		t.Fatalf("LoadStore: %s", err)
	}
	if s.Version != storeVersion {
		t.Errorf("version = %d after migration, want %d", s.Version, storeVersion)
	}
	if a, _ := s.FindAlias("k"); a == nil {
		t.Errorf("alias lost during v6->v7 migration")
	}
	if len(s.CustomProviders) != 0 {
		t.Errorf("v6->v7 conjured custom providers: %+v", s.CustomProviders)
	}
}

// TestStoreRoundTripWithCustomProvider: a custom preset survives
// save -> load -> save byte-stably, like every other store field.
func TestStoreRoundTripWithCustomProvider(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Version: storeVersion, Backend: BackendFile}
	s.UpsertCustomProvider(Provider{Name: "corp", BaseURL: "https://api.corp.example", AuthStyle: "header:X-Corp"})
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := loaded.FindCustomProvider("corp")
	if p == nil || p.BaseURL != "https://api.corp.example" || p.AuthStyle != "header:X-Corp" {
		t.Fatalf("custom preset did not survive reload: %+v", p)
	}
	if err := SaveStore(dir, loaded); err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Errorf("save/load/save with a custom preset not byte-stable")
	}
}

// TestMCPBrokersCustomProvider: the MCP server advertises and brokers
// an alias tagged with a store-defined preset, exactly like a built-in.
func TestMCPBrokersCustomProvider(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	if code, _, errOut := runVault(t, "", "provider", "add",
		"--url="+fu.URL, "--auth-style=x-api-key", "corp"); code != 0 {
		t.Fatalf("provider add: %s", errOut)
	}
	if code, _, _ := runVault(t, "mcp-secret\n", "add", "--from-stdin", "--provider=corp", "k"); code != 0 {
		t.Fatal("alias add")
	}
	w4GrantAgentCap(t, home, "k", nil)

	srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard,
		client: &http.Client{Timeout: 10 * time.Second}}

	resp := w4Dispatch(t, srv, "1", "tools/list", "")
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/list = %+v", resp)
	}
	listed, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(listed), "k_request") || !strings.Contains(string(listed), fu.URL) {
		t.Errorf("tools/list should advertise the custom-preset tool: %s", listed)
	}

	resp = w4Dispatch(t, srv, "2", "tools/call", `{"name":"k_request","arguments":{"path":"/ping"}}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/call = %+v", resp)
	}
	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastKey != "mcp-secret" {
		t.Errorf("upstream x-api-key = %q, want the real secret", fu.lastKey)
	}
	if fu.lastPath != "/ping" {
		t.Errorf("upstream path = %q, want /ping", fu.lastPath)
	}
}

// TestMCPUpstreamErrorHidesQuerySecret is the regression guard for the
// query:<name> secret leak: with that auth style the secret rides in the
// request URL, so a transport error (dead upstream) must NOT be echoed
// to the model — the *url.Error text embeds the full URL. The MCP server
// must return an opaque message, exactly as the proxy does.
func TestMCPUpstreamErrorHidesQuerySecret(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	// A server we immediately close, so its URL is a valid-but-dead
	// upstream: client.Do fails and the error carries the request URL.
	dead := newFakeUpstream(t)
	deadURL := dead.URL
	dead.Server.Close()

	const secret = "THE-QUERY-SECRET-9Z"
	if code, _, errOut := runVault(t, "", "provider", "add",
		"--url="+deadURL, "--auth-style=query:api_key", "corp"); code != 0 {
		t.Fatalf("provider add: %s", errOut)
	}
	if code, _, _ := runVault(t, secret+"\n", "add", "--from-stdin", "--provider=corp", "k"); code != 0 {
		t.Fatal("alias add")
	}
	w4GrantAgentCap(t, home, "k", nil)

	srv := &mcpServer{homeDir: home, agent: "mcp", stderr: io.Discard,
		client: &http.Client{Timeout: 5 * time.Second}}
	resp := w4Dispatch(t, srv, "1", "tools/call", `{"name":"k_request","arguments":{"path":"/ping"}}`)
	if resp == nil || resp.Error == nil {
		t.Fatalf("tools/call against a dead upstream should error: %+v", resp)
	}
	if strings.Contains(resp.Error.Message, secret) {
		t.Fatalf("MCP error leaked the query secret to the model: %q", resp.Error.Message)
	}
	if resp.Error.Message != "upstream request failed" {
		t.Errorf("MCP upstream error = %q, want the opaque \"upstream request failed\"", resp.Error.Message)
	}
}
