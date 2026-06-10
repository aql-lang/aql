package vault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initVaultAt initializes a fresh file-backed vault under dir with the
// given vault passphrase, and points AQL_HOME at it.
func initVaultAt(t *testing.T, dir, pass string) {
	t.Helper()
	t.Setenv(EnvHome, dir)
	t.Setenv(EnvPassphrase, pass)
	if code, _, errOut := runVault(t, "", "init", "--backend=file"); code != 0 {
		t.Fatalf("init at %s: %s", dir, errOut)
	}
}

// TestExportImportRoundTripAcrossVaults proves the portability story:
// secrets exported from one vault import cleanly into a *separate* vault
// (a stand-in for another machine), with metadata preserved.
func TestExportImportRoundTripAcrossVaults(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	bundle := filepath.Join(t.TempDir(), "vault.aqlx")

	// Source vault.
	src := t.TempDir()
	initVaultAt(t, src, "src-pass")
	if code, _, e := runVault(t, "value-one\n", "add", "--from-stdin", "--provider=openai", "--namespace=proj", "k1"); code != 0 {
		t.Fatalf("add k1: %s", e)
	}
	if code, _, e := runVault(t, "value-two\n", "add", "--from-stdin", "k2"); code != 0 {
		t.Fatalf("add k2: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}
	info, err := os.Stat(bundle)
	if err != nil {
		t.Fatalf("bundle not written: %s", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("bundle mode = %o, want 600", info.Mode().Perm())
	}

	// Destination vault: a different home and a different vault
	// passphrase, like a move to another machine.
	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	if code, out, e := runVault(t, "", "import", bundle); code != 0 {
		t.Fatalf("import: %s", e)
	} else if !strings.Contains(out, "imported k1") || !strings.Contains(out, "imported k2") {
		t.Errorf("import output missing aliases: %q", out)
	}

	// Values survived the move.
	for alias, want := range map[string]string{"k1": "value-one", "k2": "value-two"} {
		code, out, _ := runVault(t, "", "get", "--reveal", alias)
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("get %s = %q (code %d), want %q", alias, out, code, want)
		}
	}
	// Metadata survived too.
	s, _ := LoadStore(dst)
	if a, _ := s.FindAlias("k1"); a == nil || a.Provider != "openai" || a.Namespace != "proj" {
		t.Errorf("k1 metadata not preserved: %+v", a)
	}
}

func TestExportSubsetByName(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	for _, k := range []string{"a", "b", "c"} {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", k); code != 0 {
			t.Fatalf("add %s: %s", k, e)
		}
	}
	bundle := filepath.Join(t.TempDir(), "b.aqlx")
	if code, _, e := runVault(t, "", "export", "--out="+bundle, "a", "c"); code != 0 {
		t.Fatalf("export: %s", e)
	}
	data, _ := os.ReadFile(bundle)
	plain, err := openExport(data, "bp")
	if err != nil {
		t.Fatal(err)
	}
	var b exportBundle
	_ = json.Unmarshal(plain, &b)
	got := map[string]bool{}
	for _, a := range b.Aliases {
		got[a.Name] = true
	}
	if !got["a"] || !got["c"] || got["b"] {
		t.Errorf("subset export wrong: %v", got)
	}
}

func TestImportBundleSkipsExistingUnlessOverwrite(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	bundle := filepath.Join(t.TempDir(), "b.aqlx")

	src := t.TempDir()
	initVaultAt(t, src, "p")
	if code, _, _ := runVault(t, "fresh\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}

	// Destination already has k with a different value.
	dst := t.TempDir()
	initVaultAt(t, dst, "p")
	if code, _, _ := runVault(t, "original\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add dst")
	}

	// Default: skip, value unchanged.
	if code, out, _ := runVault(t, "", "import", bundle); code != 0 || !strings.Contains(out, "skipped 1") {
		t.Errorf("expected skip, code=%d out=%q", code, out)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); !strings.Contains(out, "original") {
		t.Errorf("value should be unchanged, got %q", out)
	}

	// --overwrite replaces it.
	if code, _, e := runVault(t, "", "import", "--overwrite", bundle); code != 0 {
		t.Fatalf("import --overwrite: %s", e)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); !strings.Contains(out, "fresh") {
		t.Errorf("value should be overwritten, got %q", out)
	}
}

func TestImportBundleWrongPassphrase(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "b.aqlx")
	src := t.TempDir()
	t.Setenv(EnvExportPassphrase, "right-pass")
	initVaultAt(t, src, "p")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "p")
	t.Setenv(EnvExportPassphrase, "wrong-pass")
	code, _, errOut := runVault(t, "", "import", bundle)
	if code == 0 {
		t.Fatal("expected failure with wrong export passphrase")
	}
	if !strings.Contains(errOut, "wrong export passphrase") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestExportRefusesEmptyPassphrase(t *testing.T) {
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	// No env passphrase; two empty prompt lines -> empty passphrase.
	t.Setenv(EnvExportPassphrase, "")
	bundle := filepath.Join(t.TempDir(), "b.aqlx")
	code, _, errOut := runVault(t, "\n\n", "export", "--out="+bundle)
	if code == 0 {
		t.Fatal("expected refusal of empty export passphrase")
	}
	if !strings.Contains(errOut, "must not be empty") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestImportRejectsFutureBundleVersion(t *testing.T) {
	// Craft a bundle whose inner schema version is from the future.
	future := exportBundle{Version: exportVersion + 1, Aliases: []exportAlias{{Name: "k", Value: "v"}}}
	plain, _ := json.Marshal(future)
	blob, err := sealExport(plain, "bp")
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "b.aqlx")
	if err := os.WriteFile(bundle, blob, 0600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	t.Setenv(EnvExportPassphrase, "bp")
	initVaultAt(t, dir, "p")
	code, _, errOut := runVault(t, "", "import", bundle)
	if code == 0 {
		t.Fatal("expected refusal of future bundle version")
	}
	if !strings.Contains(errOut, "upgrade aql") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestExportBundleEnvelopeShape(t *testing.T) {
	blob, err := sealExport([]byte(`{"version":1}`), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(blob, []byte(exportMagic)) {
		t.Errorf("bundle missing magic header")
	}
	if int(blob[len(exportMagic)]) != exportEnvelopeFormat {
		t.Errorf("envelope format byte = %d, want %d", blob[len(exportMagic)], exportEnvelopeFormat)
	}
	// A bundle is NOT mistaken for a keyring file, and vice versa.
	if isExportBundle([]byte("AQLK\x01rest")) {
		t.Errorf("keyring magic misclassified as export bundle")
	}
	got, err := openExport(blob, "pw")
	if err != nil || string(got) != `{"version":1}` {
		t.Errorf("round-trip failed: %q, %v", got, err)
	}
}

func TestImportBundleFromStdinRequiresEnvPassphrase(t *testing.T) {
	// Build a valid bundle.
	plain, _ := json.Marshal(exportBundle{Version: exportVersion, Aliases: []exportAlias{{Name: "k", Value: "v"}}})
	blob, _ := sealExport(plain, "bp")

	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	t.Setenv(EnvExportPassphrase, "") // force prompt path, which stdin can't serve

	// Feed the bundle on stdin with no --in path and no env passphrase.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"import"}, bytes.NewReader(blob), &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected failure: cannot prompt for passphrase while stdin carries the bundle")
	}
	if !strings.Contains(stderr.String(), EnvExportPassphrase) {
		t.Errorf("error should name the env var, got %q", stderr.String())
	}
}
