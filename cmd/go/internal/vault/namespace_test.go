package vault

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestValidAliasQualified(t *testing.T) {
	valid := []string{"key", "proj:key", "a.b:c-d_e", "p:k"}
	invalid := []string{":key", "proj:", ":", "a:b:c", "proj:b:c", "", "a b", "p!:k", "p:k!"}
	for _, s := range valid {
		if !validAlias(s) {
			t.Errorf("validAlias(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if validAlias(s) {
			t.Errorf("validAlias(%q) = true, want false", s)
		}
	}
}

func TestResolveAliasRef(t *testing.T) {
	root := &Store{} // no default namespace configured
	withDefault := &Store{Config: map[string]any{configNamespaceKey: "proj"}}

	tests := []struct {
		name string
		s    *Store
		env  string // AQL_VAULT_NAMESPACE; "" = unset
		ref  string
		want string
		err  bool
	}{
		{"bare no default -> root", root, "", "key", "key", false},
		{"bare with config default", withDefault, "", "key", "proj:key", false},
		{"bare with env default", root, "envns", "key", "envns:key", false},
		{"env overrides config", withDefault, "envns", "key", "envns:key", false},
		{"env ':' forces root over config", withDefault, ":", "key", "key", false},
		{"explicit root strips", withDefault, "", ":key", "key", false},
		{"qualified passes through", withDefault, "", "other:key", "other:key", false},
		{"invalid env default errors", root, "bad ns", "key", "", true},
		{"leading-colon qualified rejected", root, "", ":a:b", "", true},
		{"bare invalid rejected", root, "", "a b", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvNamespace, tc.env)
			got, err := resolveAliasRef(tc.s, tc.ref)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != tc.want {
				t.Errorf("resolveAliasRef(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestAliasNamespaceDerivedAndLegacy(t *testing.T) {
	if ns := aliasNamespace(Alias{Name: "proj:key"}); ns != "proj" {
		t.Errorf("derived ns = %q, want proj", ns)
	}
	// Legacy: unqualified name with a metadata tag.
	if ns := aliasNamespace(Alias{Name: "key", Namespace: "legacy"}); ns != "legacy" {
		t.Errorf("legacy ns = %q, want legacy", ns)
	}
	if ns := aliasNamespace(Alias{Name: "key"}); ns != "" {
		t.Errorf("root ns = %q, want empty", ns)
	}
	// A qualified name wins over a (stale) metadata tag.
	if ns := aliasNamespace(Alias{Name: "proj:key", Namespace: "stale"}); ns != "proj" {
		t.Errorf("qualified-over-tag ns = %q, want proj", ns)
	}
}

// TestNamespaceDefaultSugarRoundTrip is the headline flow: set a
// default namespace, add and read back with bare names, escape to
// root with :name.
func TestNamespaceDefaultSugarRoundTrip(t *testing.T) {
	testHome(t)
	mustInit(t)

	// A root-level key created before any default is configured.
	if code, _, e := runVault(t, "root-val\n", "add", "--from-stdin", "shared"); code != 0 {
		t.Fatalf("add shared: %s", e)
	}

	if code, _, e := runVault(t, "", "config", "--set", "namespace.default=proj"); code != 0 {
		t.Fatalf("config: %s", e)
	}

	// Bare add lands in the default namespace.
	if code, out, e := runVault(t, "proj-val\n", "add", "--from-stdin", "key"); code != 0 {
		t.Fatalf("add key: %s", e)
	} else if !strings.Contains(out, "stored proj:key") {
		t.Errorf("add output = %q, want stored proj:key", out)
	}

	// Bare get reads it back.
	if code, out, _ := runVault(t, "", "get", "--reveal", "key"); code != 0 || !strings.Contains(out, "proj-val") {
		t.Errorf("get key = %q (code %d), want proj-val", out, code)
	}
	// :name escapes to root.
	if code, out, _ := runVault(t, "", "get", "--reveal", ":shared"); code != 0 || !strings.Contains(out, "root-val") {
		t.Errorf("get :shared = %q (code %d), want root-val", out, code)
	}
	// AQL_VAULT_NAMESPACE=: forces root for a bare name.
	t.Setenv(EnvNamespace, ":")
	if code, out, _ := runVault(t, "", "get", "--reveal", "shared"); code != 0 || !strings.Contains(out, "root-val") {
		t.Errorf("get shared with env ':' = %q (code %d), want root-val", out, code)
	}
	t.Setenv(EnvNamespace, "")

	// A bare miss whose root twin exists points at the :name escape.
	if code, _, errOut := runVault(t, "", "get", "shared"); code == 0 {
		t.Errorf("get shared should miss (resolves to proj:shared)")
	} else if !strings.Contains(errOut, ":shared") {
		t.Errorf("miss error should hint at :shared, got %q", errOut)
	}
}

func TestAddNamespaceFlagSugarAndConflict(t *testing.T) {
	testHome(t)
	mustInit(t)

	if code, out, e := runVault(t, "v\n", "add", "--from-stdin", "--namespace=proj", "key"); code != 0 {
		t.Fatalf("add: %s", e)
	} else if !strings.Contains(out, "stored proj:key") {
		t.Errorf("flag did not qualify: %q", out)
	}
	// ':' = explicit root.
	if code, out, e := runVault(t, "v\n", "add", "--from-stdin", "--namespace=:", "rootkey"); code != 0 {
		t.Fatalf("add root: %s", e)
	} else if !strings.Contains(out, "stored rootkey") {
		t.Errorf("':' flag should store at root: %q", out)
	}
	// Flag + prefix is a conflict, not a precedence rule.
	if code, _, errOut := runVault(t, "v\n", "add", "--from-stdin", "--namespace=a", "b:key"); code == 0 {
		t.Error("expected conflict error for --namespace with qualified alias")
	} else if !strings.Contains(errOut, "already carries a namespace") {
		t.Errorf("unexpected conflict error: %q", errOut)
	}
}

func TestListAndStatusNamespaceReporting(t *testing.T) {
	testHome(t)
	mustInit(t)
	for ref, val := range map[string]string{"proj:a": "1", "other:b": "2", "c": "3"} {
		if code, _, e := runVault(t, val+"\n", "add", "--from-stdin", ref); code != 0 {
			t.Fatalf("add %s: %s", ref, e)
		}
	}

	code, out, _ := runVault(t, "", "list", "--namespace=proj")
	if code != 0 {
		t.Fatal("list proj")
	}
	if !strings.Contains(out, "proj:a") || strings.Contains(out, "other:b") || strings.Contains(out, "\nc ") {
		t.Errorf("list --namespace=proj wrong rows: %q", out)
	}
	// ':' filters to root only.
	code, out, _ = runVault(t, "", "list", "--namespace=:")
	if code != 0 {
		t.Fatal("list root")
	}
	if !strings.Contains(out, "c") || strings.Contains(out, "proj:a") || strings.Contains(out, "other:b") {
		t.Errorf("list --namespace=: wrong rows: %q", out)
	}
	// Invalid filter value is rejected.
	if code, _, _ := runVault(t, "", "list", "--namespace=a:b"); code == 0 {
		t.Error("expected error for invalid namespace filter")
	}

	// Status shows the per-namespace breakdown.
	code, out, _ = runVault(t, "", "status")
	if code != 0 {
		t.Fatal("status")
	}
	if !strings.Contains(out, "namespaces:") || !strings.Contains(out, "(root)=1") ||
		!strings.Contains(out, "proj=1") || !strings.Contains(out, "other=1") {
		t.Errorf("status missing namespace breakdown: %q", out)
	}
}

func TestStatusOmitsNamespacesWhenAllRoot(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	_, out, _ := runVault(t, "", "status")
	if strings.Contains(out, "namespaces:") {
		t.Errorf("all-root vault should not print a namespaces line: %q", out)
	}
}

func TestAuditNamespaceFilter(t *testing.T) {
	testHome(t)
	mustInit(t)
	for _, ref := range []string{"proj:a", "b"} {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", ref); code != 0 {
			t.Fatalf("add %s: %s", ref, e)
		}
	}
	code, out, _ := runVault(t, "", "audit", "--namespace=proj", "--action=vault.add")
	if code != 0 {
		t.Fatal("audit proj")
	}
	if !strings.Contains(out, "alias=proj:a") || strings.Contains(out, "alias=b") {
		t.Errorf("audit --namespace=proj wrong events: %q", out)
	}
	code, out, _ = runVault(t, "", "audit", "--namespace=:", "--action=vault.add")
	if code != 0 {
		t.Fatal("audit root")
	}
	if !strings.Contains(out, "alias=b") || strings.Contains(out, "alias=proj:a") {
		t.Errorf("audit --namespace=: wrong events: %q", out)
	}
}

func TestExecNamespacedEnvNameDerivation(t *testing.T) {
	mappings, err := parseExecAliases("proj:key,:rootkey,plain", "", false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"proj:key": "key", ":rootkey": "rootkey", "plain": "plain"}
	for _, m := range mappings {
		if want[m.alias] != m.envName {
			t.Errorf("alias %q env = %q, want %q", m.alias, m.envName, want[m.alias])
		}
	}
	// Explicit env name still wins.
	mappings, err = parseExecAliases("proj:key=TOKEN", "", false)
	if err != nil || mappings[0].envName != "TOKEN" {
		t.Errorf("explicit env name: %+v, %v", mappings, err)
	}
}

func TestExecResolvesDefaultNamespace(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "ns-val\n", "add", "--from-stdin", "proj:key"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "config", "--set", "namespace.default=proj"); code != 0 {
		t.Fatalf("config: %s", e)
	}
	// `exec key -- sh -c 'printf %s "$key"'` resolves proj:key, env $key.
	code, out, errOut := runVault(t, "", "exec", "key", "--", "sh", "-c", `printf %s "$key"`)
	if code != 0 {
		t.Fatalf("exec: %s", errOut)
	}
	if out != "ns-val" {
		t.Errorf("exec output = %q, want ns-val", out)
	}
}

func TestExportNamespaceFilterAndImportRemap(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	bundle := t.TempDir() + "/b.aqlx"

	src := t.TempDir()
	initVaultAt(t, src, "p")
	for ref, val := range map[string]string{"proj:a": "1", "other:b": "2"} {
		if code, _, e := runVault(t, val+"\n", "add", "--from-stdin", ref); code != 0 {
			t.Fatalf("add %s: %s", ref, e)
		}
	}
	// Export only proj.
	if code, _, e := runVault(t, "", "export", "--out="+bundle, "--namespace=proj"); code != 0 {
		t.Fatalf("export: %s", e)
	}

	// Import remapped into a different namespace.
	dst := t.TempDir()
	initVaultAt(t, dst, "p")
	if code, out, e := runVault(t, "", "import", "--namespace=mirror", bundle); code != 0 {
		t.Fatalf("import: %s", e)
	} else if !strings.Contains(out, "imported mirror:a") {
		t.Errorf("remap output = %q", out)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "mirror:a"); code != 0 || !strings.Contains(out, "1") {
		t.Errorf("get mirror:a = %q (code %d)", out, code)
	}
	// other:b was filtered out of the bundle.
	if code, _, _ := runVault(t, "", "get", "other:b"); code == 0 {
		t.Error("other:b should not have been exported")
	}

	// Import remapped to root via ':'.
	dst2 := t.TempDir()
	initVaultAt(t, dst2, "p")
	if code, out, e := runVault(t, "", "import", "--namespace=:", bundle); code != 0 {
		t.Fatalf("import root: %s", e)
	} else if !strings.Contains(out, "imported a\n") {
		t.Errorf("root remap output = %q", out)
	}
}

func TestProxyServesQualifiedAlias(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, e := runVault(t, "qv\n", "add", "--from-stdin", "--provider=fake", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	tok := grantOK(t, "proj:k", nil, nil)

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/proj:k/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (%s)", resp.StatusCode, body)
	}
	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastAuth != "Bearer qv" {
		t.Errorf("upstream auth = %q", fu.lastAuth)
	}
	if fu.lastPath != "/v1/x" {
		t.Errorf("upstream path = %q", fu.lastPath)
	}
}

func TestMCPToolNameManglingAndCall(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, e := runVault(t, "mcp-ns-secret\n", "add", "--from-stdin", "--provider=fake", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "grant", "--agent=claude", "proj:k"); code != 0 {
		t.Fatalf("grant: %s", e)
	}

	srv := &mcpServer{homeDir: os.Getenv(EnvHome), agent: "claude", stderr: io.Discard, client: http.DefaultClient}
	resp := mcpRPC(t, srv, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list: %+v", resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	body := string(b)
	if !strings.Contains(body, "proj_k_request") {
		t.Errorf("mangled tool name missing: %s", body)
	}
	if strings.Contains(body, "proj:k_request") {
		t.Errorf("unmangled ':' leaked into tool name: %s", body)
	}

	resp = mcpRPC(t, srv, "tools/call", map[string]any{
		"name":      "proj_k_request",
		"arguments": map[string]any{"path": "/v1/y"},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call: %+v", resp.Error)
	}
	fu.mu.Lock()
	defer fu.mu.Unlock()
	if fu.lastAuth != "Bearer mcp-ns-secret" {
		t.Errorf("upstream auth = %q", fu.lastAuth)
	}
}
