package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryHandlerServesZip(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/color-0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("invalid zip: %s", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["aql.jsonic"] {
		t.Error("zip missing aql.jsonic")
	}
	if !names["color.aql"] {
		t.Error("zip missing color.aql")
	}
}

func TestRegistryHandlerServesColorScheme(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/color-scheme-0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("invalid zip: %s", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["aql.jsonic"] {
		t.Error("zip missing aql.jsonic")
	}
	if !names["index.aql"] {
		t.Error("zip missing index.aql")
	}
}

func TestRegistryHandlerNotFound(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/nonexistent-1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRegistryHandlerEmptyPath(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRegistryHandlerRejectsPost(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/module/color-0.1.0", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 405 {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestRegistryHandlerRejectsTraversal(t *testing.T) {
	dir := filepath.Join("../../../lang/test/regsrv/registry")
	srv := httptest.NewServer(registryHandler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/../../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRunRegistryMissingFolder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runRegistry([]string{"-r", "/nonexistent/dir", "-p", "0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("not found")) {
		t.Errorf("expected 'not found' error, got %q", stderr.String())
	}
}

func TestRunRegistryMissingFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runRegistry([]string{"-p", "0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("-r")) {
		t.Errorf("expected '-r' error, got %q", stderr.String())
	}
}

func TestExecuteRegistrySubcommand(t *testing.T) {
	// Use a temp dir with a zip to test the full execute path.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test-1.0.0.zip"), []byte("PK\x03\x04"), 0644)

	var stdout, stderr bytes.Buffer
	// Use -p 0 which will get a random port - but we can't easily test
	// the full server lifecycle via execute() since it blocks.
	// Instead just test that missing -r is caught.
	code := execute([]string{"registry", "-p", "0"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}
