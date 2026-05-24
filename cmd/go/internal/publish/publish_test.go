package publish

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/install"
	"github.com/aql-lang/aql/cmd/go/internal/pack"
	"github.com/aql-lang/aql/cmd/go/internal/prep"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
)

// --- "not logged in" guard ---

func TestRunPublishNotLoggedIn(t *testing.T) {
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	var stdout, stderr bytes.Buffer
	code := Run(nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "not logged in") {
		t.Errorf("unexpected error: %q", stderr.String())
	}
}

// --- full pack → publish round-trip ---

func TestRunPublishCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	regBody := `{"email":"pub@example.com","username":"pubuser","password":"pubpass"}`
	resp, _ := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	loginBody := `{"username":"pubuser","password":"pubpass"}`
	resp, _ = http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(loginBody))
	var loginResult map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	auth.SaveClientUser(homeDir, &auth.ClientUser{
		Username: "pubuser",
		Email:    "pub@example.com",
		Token:    loginResult["token"],
		Registry: srv.URL,
	})

	moduleDir := t.TempDir()
	os.WriteFile(filepath.Join(moduleDir, "aql.jsonic"),
		[]byte("name: clipub\nmajor: 1\nminor: 0\npatch: 0\nfiles: [clipub.aql]\n"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "clipub.aql"), []byte("1"), 0644)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srv.URL, moduleDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "published clipub@1.0.0") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}

// --- register → login → publish → install integration ---

func TestRegisterLoginPublishFlow(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()
	srvURL := srv.URL

	regBody := `{"email":"dev@example.com","username":"dev","password":"devpass"}`
	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status = %d", resp.StatusCode)
	}

	loginBody := `{"username":"dev","password":"devpass"}`
	resp, err = http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	var loginResult map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d", resp.StatusCode)
	}
	token := loginResult["token"]
	if token == "" {
		t.Fatal("login returned empty token")
	}

	moduleDir := t.TempDir()
	os.WriteFile(filepath.Join(moduleDir, "aql.jsonic"),
		[]byte("name: flowmod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [flowmod.aql]\n"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "flowmod.aql"),
		[]byte(`export Flowmod {val: 42}`), 0644)

	var packOut, packErr bytes.Buffer
	code := pack.Run([]string{moduleDir}, &packOut, &packErr)
	if code != 0 {
		t.Fatalf("pack failed: %s", packErr.String())
	}
	zipPath := strings.TrimSpace(packOut.String())
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("reading zip: %s", err)
	}

	// POST /api/publish with the token.
	req, _ := http.NewRequest(http.MethodPost, srvURL+"/api/publish", bytes.NewReader(zipData))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pubBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish: status = %d; body: %s", resp.StatusCode, pubBody)
	}

	var pubResult map[string]string
	json.Unmarshal(pubBody, &pubResult)
	if pubResult["module"] != "flowmod" {
		t.Errorf("published module = %q, want flowmod", pubResult["module"])
	}
	if pubResult["version"] != "1.0.0" {
		t.Errorf("published version = %q, want 1.0.0", pubResult["version"])
	}

	installDir := t.TempDir()
	os.WriteFile(filepath.Join(installDir, "aql.jsonic"),
		[]byte("name: myapp\nmajor: 0\nminor: 1\npatch: 0\nfiles: [app.aql]\n"), 0644)
	os.WriteFile(filepath.Join(installDir, "app.aql"), []byte("1"), 0644)
	os.MkdirAll(filepath.Join(installDir, ".aql"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(installDir)
	defer os.Chdir(orig)

	var prepOut, prepErr bytes.Buffer
	prep.Run(nil, &prepOut, &prepErr)

	var instOut, instErr bytes.Buffer
	code = install.Run([]string{"-r", srvURL, "flowmod-1.0.0"}, &instOut, &instErr)
	if code != 0 {
		t.Fatalf("install failed: %s", instErr.String())
	}
	if !strings.Contains(instOut.String(), "installed flowmod@1.0.0") {
		t.Errorf("unexpected install output: %q", instOut.String())
	}

	modAql, _ := os.ReadFile(filepath.Join(".aql", "flowmod", "flowmod.aql"))
	if !strings.Contains(string(modAql), "val: 42") {
		t.Errorf("installed module content wrong: %s", modAql)
	}
}
