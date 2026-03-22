package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeModuleZip creates an in-memory zip with the given files.
func makeModuleZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func setupPublishServer(t *testing.T) (srvURL string, regDir string) {
	t.Helper()
	regDir = t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
	t.Cleanup(srv.Close)
	return srv.URL, regDir
}

// --- Happy path ---

func TestPublishValid(t *testing.T) {
	srvURL, regDir := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: hello\nmain: hello.aql\nmajor: 1\nminor: 0\npatch: 0\nfiles: [hello.aql]\n",
		"hello.aql":  `export Hello {greet: "hi"}`,
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201; body: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["module"] != "hello" {
		t.Errorf("module = %q, want hello", result["module"])
	}
	if result["version"] != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", result["version"])
	}

	// Verify zip was written to registry.
	if _, err := os.Stat(filepath.Join(regDir, "hello-1.0.0.zip")); err != nil {
		t.Errorf("expected hello-1.0.0.zip in registry: %s", err)
	}
}

// --- Immutability ---

func TestPublishRejectsOverwrite(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: mymod\nmajor: 0\nminor: 1\npatch: 0\nfiles: [mymod.aql]\n",
		"mymod.aql":  `export Mymod {val: 1}`,
	})

	// First publish succeeds.
	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first publish: status = %d, want 201", resp.StatusCode)
	}

	// Second publish of same version fails with 409 Conflict.
	resp, err = http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second publish: status = %d, want 409; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "already exists") {
		t.Errorf("expected 'already exists' error, got %q", body)
	}
}

// --- Version increments ---

func TestPublishMultipleVersions(t *testing.T) {
	srvURL, regDir := setupPublishServer(t)

	versions := []struct {
		major, minor, patch int
		version             string
	}{
		{1, 0, 0, "1.0.0"},
		{1, 0, 1, "1.0.1"},
		{1, 1, 0, "1.1.0"},
		{2, 0, 0, "2.0.0"},
	}

	for _, v := range versions {
		jContent := fmt.Sprintf("name: vmod\nmajor: %d\nminor: %d\npatch: %d\nfiles: [vmod.aql]\n",
			v.major, v.minor, v.patch)

		zd := makeModuleZip(t, map[string]string{
			"aql.jsonic": jContent,
			"vmod.aql":   `export Vmod {v: 1}`,
		})

		resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zd))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("publish %s: status = %d, want 201", v.version, resp.StatusCode)
		}

		if _, err := os.Stat(filepath.Join(regDir, "vmod-"+v.version+".zip")); err != nil {
			t.Errorf("expected vmod-%s.zip: %s", v.version, err)
		}
	}
}

// --- Validation errors ---

func TestPublishRejectsEmptyBody(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPublishRejectsInvalidZip(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader([]byte("not a zip")))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestPublishRejectsMissingAqlJsonic(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"hello.aql": `export Hello {greet: "hi"}`,
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "aql.jsonic") {
		t.Errorf("expected aql.jsonic error, got %q", body)
	}
}

func TestPublishRejectsMissingName(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "major: 1\nminor: 0\npatch: 0\nfiles: [x.aql]\n",
		"x.aql":      "1",
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "name") {
		t.Errorf("expected name error, got %q", body)
	}
}

func TestPublishRejectsMissingVersion(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: noversion\nfiles: [x.aql]\n",
		"x.aql":      "1",
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "version") {
		t.Errorf("expected version error, got %q", body)
	}
}

func TestPublishRejectsMissingFiles(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: nofiles\nmajor: 1\nminor: 0\npatch: 0\n",
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "files") {
		t.Errorf("expected files error, got %q", body)
	}
}

func TestPublishRejectsMissingDeclaredFile(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	// files declares "missing.aql" but zip doesn't contain it.
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: broken\nmajor: 1\nminor: 0\npatch: 0\nfiles: [missing.aql]\n",
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "missing.aql") {
		t.Errorf("expected missing file error, got %q", body)
	}
}

func TestPublishRejectsGetMethod(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	resp, err := http.Get(srvURL + "/api/publish")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

// --- Publish then install roundtrip ---

func TestPublishThenInstall(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	// Publish a module.
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: greeter\nmain: greeter.aql\nmajor: 2\nminor: 3\npatch: 4\nfiles: [greeter.aql]\n",
		"greeter.aql": `
def greet fn [[name:String] [String] [("Hello, " name " !" +3)]]
export Greeter {greet: greet}
`,
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish: status = %d, want 201", resp.StatusCode)
	}

	// Set up a project and install the published module.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte("name: myapp\nmajor: 0\nminor: 1\npatch: 0\nfiles: [index.aql]\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.aql"), []byte("1"), 0644)
	os.MkdirAll(filepath.Join(dir, ".aql"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := runPrep(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prep failed: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runInstall([]string{"-r", srvURL, "greeter-2.3.4"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "installed greeter@2.3.4") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	// Verify extracted files.
	if _, err := os.Stat(filepath.Join(".aql", "greeter", "greeter.aql")); err != nil {
		t.Error("missing .aql/greeter/greeter.aql")
	}
	if _, err := os.Stat(filepath.Join(".aql", "greeter", "aql.jsonic")); err != nil {
		t.Error("missing .aql/greeter/aql.jsonic")
	}

	// Verify deps updated.
	data, _ := os.ReadFile("aql.jsonic")
	if !strings.Contains(string(data), "greeter: 2.3.4") {
		t.Errorf("aql.jsonic missing greeter dep: %s", data)
	}
}

// --- Publish new version after existing ---

func TestPublishVersionIncrement(t *testing.T) {
	srvURL, _ := setupPublishServer(t)

	// Publish v1.0.0.
	zip1 := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: incr\nmajor: 1\nminor: 0\npatch: 0\nfiles: [incr.aql]\n",
		"incr.aql":   `export Incr {v: 1}`,
	})
	resp, _ := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zip1))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("v1.0.0: status = %d", resp.StatusCode)
	}

	// Publish v1.0.1 (patch bump).
	zip2 := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: incr\nmajor: 1\nminor: 0\npatch: 1\nfiles: [incr.aql]\n",
		"incr.aql":   `export Incr {v: 2}`,
	})
	resp, _ = http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zip2))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("v1.0.1: status = %d", resp.StatusCode)
	}

	// v1.0.0 still exists and is unchanged.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte("name: app\nmajor: 0\nminor: 1\npatch: 0\nfiles: [i.aql]\n"), 0644)
	os.WriteFile(filepath.Join(dir, "i.aql"), []byte("1"), 0644)
	os.MkdirAll(filepath.Join(dir, ".aql"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	runPrep(nil, &stdout, &stderr)

	// Install v1.0.0.
	stdout.Reset()
	stderr.Reset()
	code := runInstall([]string{"-r", srvURL, "incr-1.0.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install v1.0.0 failed: %s", stderr.String())
	}
	aql1, _ := os.ReadFile(filepath.Join(".aql", "incr", "incr.aql"))
	if !strings.Contains(string(aql1), "v: 1") {
		t.Errorf("v1.0.0 content wrong: %s", aql1)
	}

	// Install v1.0.1 (overwrites local install dir, not registry).
	stdout.Reset()
	stderr.Reset()
	code = runInstall([]string{"-r", srvURL, "incr-1.0.1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install v1.0.1 failed: %s", stderr.String())
	}
	aql2, _ := os.ReadFile(filepath.Join(".aql", "incr", "incr.aql"))
	if !strings.Contains(string(aql2), "v: 2") {
		t.Errorf("v1.0.1 content wrong: %s", aql2)
	}

	// Re-attempting to publish v1.0.0 still fails.
	resp, _ = http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zip1))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("re-publish v1.0.0: status = %d, body: %s", resp.StatusCode, body)
	}
}
