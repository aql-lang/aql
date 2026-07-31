package install

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/prep"
	"github.com/boru-lang/boru/cmd/go/internal/registry"
)

func setupInstallTest(t *testing.T) (dir string, srvURL string, cleanup func()) {
	t.Helper()

	regDir, _ := filepath.Abs(filepath.Join("../../../../lang/go/test/regsrv/registry"))
	srv := httptest.NewServer(registry.Handler(regDir))

	dir = t.TempDir()
	os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte(`name: testmod
major: 0
minor: 1
patch: 0
files: [index.boru]
`), 0644)
	os.WriteFile(filepath.Join(dir, "index.boru"), []byte(`(import "color") "#FF0000" Color.hex2rgb .r`), 0644)
	os.MkdirAll(filepath.Join(dir, ".boru"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := prep.Run(nil, &stdout, &stderr)
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
	_ = dir

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "installed color@0.1.0") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(".boru", "color", "color.boru")); err != nil {
		t.Errorf("expected .boru/color/color.boru: %s", err)
	}
	if _, err := os.Stat(filepath.Join(".boru", "color", "boru.jsonic")); err != nil {
		t.Errorf("expected .boru/color/boru.jsonic: %s", err)
	}
}

func TestInstallUpdatesDeps(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}

	data, err := os.ReadFile("boru.jsonic")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "deps:") {
		t.Fatalf("boru.jsonic missing deps: %s", content)
	}
	if !strings.Contains(content, "color: 0.1.0") {
		t.Fatalf("boru.jsonic missing color dep: %s", content)
	}
}

func TestInstallRegeneratesBoruJSON(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(".boru", "boru.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	deps, ok := m["deps"].(map[string]any)
	if !ok {
		t.Fatalf("expected deps map in boru.json, got %v", m["deps"])
	}
	if deps["color"] != "0.1.0" {
		t.Errorf("deps.color = %v, want 0.1.0", deps["color"])
	}
}

func TestInstallMultipleDeps(t *testing.T) {
	_, srvURL, cleanup := setupInstallTest(t)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first install failed: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"-r", srvURL, "color-scheme-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second install failed: %s", stderr.String())
	}

	data, err := os.ReadFile("boru.jsonic")
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

	if _, err := os.Stat(filepath.Join(".boru", "color", "color.boru")); err != nil {
		t.Error("missing .boru/color/color.boru")
	}
	if _, err := os.Stat(filepath.Join(".boru", "color-scheme", "index.boru")); err != nil {
		t.Error("missing .boru/color-scheme/index.boru")
	}
}

func TestInstallNoBoruJSON(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"color-0.1.0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a valid module") {
		t.Errorf("expected module error, got %q", stderr.String())
	}
}

func TestInstallInvalidID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"badname"}, &stdout, &stderr)
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
	code := Run([]string{"-r", srvURL, "nonexistent-1.0.0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected not found error, got %q", stderr.String())
	}
}

func TestInstallNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
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

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first install failed: %s", stderr.String())
	}

	firstBoruJsonic, _ := os.ReadFile("boru.jsonic")
	firstBoruJSON, _ := os.ReadFile(filepath.Join(".boru", "boru.json"))
	firstColorBoru, _ := os.ReadFile(filepath.Join(".boru", "color", "color.boru"))
	firstColorJsonic, _ := os.ReadFile(filepath.Join(".boru", "color", "boru.jsonic"))

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"-r", srvURL, "color-0.1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second install failed: %s", stderr.String())
	}

	secondBoruJsonic, _ := os.ReadFile("boru.jsonic")
	secondBoruJSON, _ := os.ReadFile(filepath.Join(".boru", "boru.json"))
	secondColorBoru, _ := os.ReadFile(filepath.Join(".boru", "color", "color.boru"))
	secondColorJsonic, _ := os.ReadFile(filepath.Join(".boru", "color", "boru.jsonic"))

	if string(firstBoruJsonic) != string(secondBoruJsonic) {
		t.Errorf("boru.jsonic changed:\n  first:  %s\n  second: %s", firstBoruJsonic, secondBoruJsonic)
	}
	if string(firstBoruJSON) != string(secondBoruJSON) {
		t.Errorf(".boru/boru.json changed")
	}
	if string(firstColorBoru) != string(secondColorBoru) {
		t.Errorf(".boru/color/color.boru changed")
	}
	if string(firstColorJsonic) != string(secondColorJsonic) {
		t.Errorf(".boru/color/boru.jsonic changed")
	}
}

func TestInstallDeepChain(t *testing.T) {
	regDir, _ := filepath.Abs(filepath.Join("../../../../lang/go/test/regsrv/registry"))
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte("name: deeptest\nmajor: 0\nminor: 1\npatch: 0\nfiles: [index.boru]\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.boru"), []byte(`1`), 0644)
	os.MkdirAll(filepath.Join(dir, ".boru"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	var stdout, stderr bytes.Buffer
	code := prep.Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prep failed: %s", stderr.String())
	}

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
		code = Run([]string{"-r", srv.URL, mod}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("install %s failed: %s", mod, stderr.String())
		}
		if !strings.Contains(stdout.String(), "installed") {
			t.Errorf("install %s: unexpected output %q", mod, stdout.String())
		}
	}

	for _, mod := range modules {
		name := mod[:strings.LastIndex(mod, "-")]
		modDir := filepath.Join(".boru", name)
		if _, err := os.Stat(modDir); err != nil {
			t.Errorf("expected %s directory: %s", modDir, err)
		}
	}

	data, err := os.ReadFile("boru.jsonic")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, dep := range []string{"textkit: 3.2.0", "charops: 2.3.1", "joiner: 0.4.2"} {
		if !strings.Contains(content, dep) {
			t.Errorf("boru.jsonic missing %s", dep)
		}
	}
}
