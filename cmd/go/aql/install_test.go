package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupInstallTest(t *testing.T) (dir string, srvURL string, cleanup func()) {
	t.Helper()

	// Start a test registry server using the regsrv/registry folder.
	regDir, _ := filepath.Abs(filepath.Join("../../../lang/test/regsrv/registry"))
	srv := httptest.NewServer(registryHandler(regDir))

	// Create a temp module folder with aql.jsonic and .aql/aql.json.
	dir = t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte(`name: testmod
major: 0
minor: 1
patch: 0
files: [index.aql]
`), 0644)
	os.WriteFile(filepath.Join(dir, "index.aql"), []byte(`(import "color") "#FF0000" Color.hex2rgb .r`), 0644)
	os.MkdirAll(filepath.Join(dir, ".aql"), 0755)

	// Run prep to create .aql/aql.json.
	orig, _ := os.Getwd()
	os.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runPrep(nil, &stdout, &stderr)
	if code != 0 {
		os.Chdir(orig)
		srv.Close()
		t.Fatalf("prep failed: %s", stderr.String())
	}

	return dir, srv.URL, func() {
		os.Chdir(orig)
		srv.Close()
	}
}

func TestInstallDownloadsAndExtracts(t *testing.T) {
	dir, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()
	_ = dir // we're already cd'd into dir

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "installed color@0.1.0") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	// Verify files were extracted.
	if _, err := os.Stat(filepath.Join(".aql", "color", "color.aql")); err != nil {
		t.Errorf("expected .aql/color/color.aql: %s", err)
	}
	if _, err := os.Stat(filepath.Join(".aql", "color", "aql.jsonic")); err != nil {
		t.Errorf("expected .aql/color/aql.jsonic: %s", err)
	}
}

func TestInstallUpdatesDeps(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}

	// Read aql.jsonic and verify deps.
	data, err := os.ReadFile("aql.jsonic")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "deps:") {
		t.Fatalf("aql.jsonic missing deps: %s", content)
	}
	if !strings.Contains(content, "color: 0.1.0") {
		t.Fatalf("aql.jsonic missing color dep: %s", content)
	}
}

func TestInstallRegeneratesAqlJSON(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}

	// .aql/aql.json should now contain deps.
	data, err := os.ReadFile(filepath.Join(".aql", "aql.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	deps, ok := m["deps"].(map[string]any)
	if !ok {
		t.Fatalf("expected deps map in aql.json, got %v", m["deps"])
	}
	if deps["color"] != "0.1.0" {
		t.Errorf("deps.color = %v, want 0.1.0", deps["color"])
	}
}

func TestInstallMultipleDeps(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	// Install color first.
	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first install failed: %s", stderr.String())
	}

	// Install color-scheme second.
	stdout.Reset()
	stderr.Reset()
	code = runInstall([]string{"-r", srvURL, "color-scheme-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second install failed: %s", stderr.String())
	}

	// Verify both deps in aql.jsonic.
	data, err := os.ReadFile("aql.jsonic")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "color: 0.1.0") {
		t.Errorf("missing color dep in: %s", content)
	}
	if !strings.Contains(content, "color-scheme: 0.1.0") {
		t.Errorf("missing color-scheme dep in: %s", content)
	}

	// Verify both extracted.
	if _, err := os.Stat(filepath.Join(".aql", "color", "color.aql")); err != nil {
		t.Error("missing .aql/color/color.aql")
	}
	if _, err := os.Stat(filepath.Join(".aql", "color-scheme", "index.aql")); err != nil {
		t.Error("missing .aql/color-scheme/index.aql")
	}
}

func TestInstallNoAqlJSON(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"color-0.1.0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a valid module") {
		t.Errorf("expected module error, got %q", stderr.String())
	}
}

func TestInstallInvalidID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"badname"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid module identifier") {
		t.Errorf("expected identifier error, got %q", stderr.String())
	}
}

func TestInstallModuleNotFound(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "nonexistent-1.0.0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected not found error, got %q", stderr.String())
	}
}

func TestInstallNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runInstall(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Errorf("expected usage error, got %q", stderr.String())
	}
}

func TestInstallIdempotent(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	// First install.
	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first install failed: %s", stderr.String())
	}

	// Snapshot files after first install.
	firstAqlJsonic, _ := os.ReadFile("aql.jsonic")
	firstAqlJSON, _ := os.ReadFile(filepath.Join(".aql", "aql.json"))
	firstColorAql, _ := os.ReadFile(filepath.Join(".aql", "color", "color.aql"))
	firstColorJsonic, _ := os.ReadFile(filepath.Join(".aql", "color", "aql.jsonic"))

	// Second install of the same module.
	stdout.Reset()
	stderr.Reset()
	code = runInstall([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second install failed: %s", stderr.String())
	}

	// Verify all files are identical.
	secondAqlJsonic, _ := os.ReadFile("aql.jsonic")
	secondAqlJSON, _ := os.ReadFile(filepath.Join(".aql", "aql.json"))
	secondColorAql, _ := os.ReadFile(filepath.Join(".aql", "color", "color.aql"))
	secondColorJsonic, _ := os.ReadFile(filepath.Join(".aql", "color", "aql.jsonic"))

	if string(firstAqlJsonic) != string(secondAqlJsonic) {
		t.Errorf("aql.jsonic changed:\n  first:  %s\n  second: %s", firstAqlJsonic, secondAqlJsonic)
	}
	if string(firstAqlJSON) != string(secondAqlJSON) {
		t.Errorf(".aql/aql.json changed")
	}
	if string(firstColorAql) != string(secondColorAql) {
		t.Errorf(".aql/color/color.aql changed")
	}
	if string(firstColorJsonic) != string(secondColorJsonic) {
		t.Errorf(".aql/color/aql.jsonic changed")
	}
}

func TestInstallDeepChain(t *testing.T) {
	// Install all 10 modules of the deep dependency chain into a fresh project.
	regDir, _ := filepath.Abs(filepath.Join("../../../lang/test/regsrv/registry"))
	srv := httptest.NewServer(registryHandler(regDir))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aql.jsonic"), []byte("name: deeptest\nmajor: 0\nminor: 1\npatch: 0\nfiles: [index.aql]\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.aql"), []byte(`1`), 0644)
	os.MkdirAll(filepath.Join(dir, ".aql"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := runPrep(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prep failed: %s", stderr.String())
	}

	// Install all modules in dependency order (leaves first).
	modules := []string{
		"charops-2.3.1",
		"joiner-0.4.2",
		"wrapper-1.1.0",
		"tagger-3.0.2",
		"caser-0.2.4",
		"bracket-1.3.0",
		"formatter-2.1.1",
		"decorator-0.5.3",
		"styler-1.0.7",
		"textkit-3.2.0",
	}

	for _, mod := range modules {
		stdout.Reset()
		stderr.Reset()
		code = runInstall([]string{"-r", srv.URL, mod}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("install %s failed: %s", mod, stderr.String())
		}
		if !strings.Contains(stdout.String(), "installed") {
			t.Errorf("install %s: unexpected output %q", mod, stdout.String())
		}
	}

	// Verify all 10 modules are installed.
	for _, mod := range modules {
		name := mod[:strings.LastIndex(mod, "-")]
		modDir := filepath.Join(".aql", name)
		if _, err := os.Stat(modDir); err != nil {
			t.Errorf("expected %s directory: %s", modDir, err)
		}
	}

	// Verify deps in aql.jsonic.
	data, err := os.ReadFile("aql.jsonic")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, dep := range []string{"textkit: 3.2.0", "charops: 2.3.1", "joiner: 0.4.2"} {
		if !strings.Contains(content, dep) {
			t.Errorf("aql.jsonic missing %s", dep)
		}
	}
}
