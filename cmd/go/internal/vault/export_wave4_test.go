package vault

// export_wave4_test.go — error-arm coverage for export.go: parse and
// store failures, seal-passphrase prompt edges, unwritable outputs,
// failing stdout, and the bundle-import negatives (interactive
// passphrase, malformed payload, locked vault, scope denials,
// blocked writes, and external-change detection).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// w4WriteBundle seals payload under pass and writes it to a temp file.
func w4WriteBundle(t *testing.T, payload []byte, pass string) string {
	t.Helper()
	blob, err := sealExport(payload, pass)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundle.aqlx")
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// w4BundleFor marshals an exportBundle holding the given aliases.
func w4BundleFor(t *testing.T, aliases ...exportAlias) []byte {
	t.Helper()
	b, err := json.Marshal(exportBundle{Version: exportVersion, ExportedAt: "2020-01-01T00:00:00Z", Aliases: aliases})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExportErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "export", "-nope"); code == 0 {
			t.Error("export with a bad flag should fail")
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "export", "--out=x.aqlx"); code == 0 {
			t.Error("export on a corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		out := filepath.Join(t.TempDir(), "b.aqlx")
		if code, _, e := runVault(t, "", "export", "--out="+out); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("export wrong pass = %d, %q", code, e)
		}
	})
	t.Run("unreadable value", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "ghost", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "b.aqlx")
		if code, _, e := runVault(t, "", "export", "--out="+out, "ghost"); code == 0 ||
			!strings.Contains(e, "reading ghost") {
			t.Errorf("export of a store-only alias = %d, %q", code, e)
		}
	})
	t.Run("seal passphrase prompt EOF", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		t.Setenv(EnvExportPassphrase, "")
		out := filepath.Join(t.TempDir(), "b.aqlx")
		if code, _, e := runVault(t, "", "export", "--out="+out); code == 0 || e == "" {
			t.Errorf("EOF at the first seal prompt = %d, %q", code, e)
		}
		if code, _, e := runVault(t, "bundlepw\n", "export", "--out="+out); code == 0 || e == "" {
			t.Errorf("EOF at the confirm seal prompt = %d, %q", code, e)
		}
	})
	t.Run("unwritable out path", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		t.Setenv(EnvExportPassphrase, "bp")
		if code, _, e := runVault(t, "", "export", "--out=/w4-no-such-dir/b.aqlx"); code == 0 || e == "" {
			t.Errorf("export into a missing directory = %d, %q", code, e)
		}
	})
	t.Run("failing stdout", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		t.Setenv(EnvExportPassphrase, "bp")
		var errb bytes.Buffer
		if code := Run([]string{"export"}, strings.NewReader(""), w4FailWriter{}, &errb); code == 0 ||
			!strings.Contains(errb.String(), "synthetic write failure") {
			t.Errorf("export to a failing writer = %d, %q", code, errb.String())
		}
	})
}

func TestImportBundleErrorArms(t *testing.T) {
	t.Run("interactive passphrase from file source", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "bk", Value: "bv"}), "bundle-pw")
		t.Setenv(EnvExportPassphrase, "")
		// The prompt reads the bundle passphrase from stdin.
		code, out, e := runVault(t, "bundle-pw\n", "import", path)
		if code != 0 {
			t.Fatalf("import with prompted passphrase: %s", e)
		}
		if !strings.Contains(out, "imported bk") {
			t.Errorf("import output = %q", out)
		}
		// EOF at that prompt is an error.
		if code, _, e := runVault(t, "", "import", path); code == 0 || e == "" {
			t.Errorf("EOF at the bundle passphrase prompt = %d, %q", code, e)
		}
	})
	t.Run("non-json payload", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		path := w4WriteBundle(t, []byte("this is not json"), "bp")
		t.Setenv(EnvExportPassphrase, "bp")
		if code, _, e := runVault(t, "", "import", path); code == 0 ||
			!strings.Contains(e, "parsing bundle") {
			t.Errorf("import of a non-JSON payload = %d, %q", code, e)
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "bk", Value: "bv"}), "bp")
		w4CorruptStore(t, home)
		t.Setenv(EnvExportPassphrase, "bp")
		if code, _, _ := runVault(t, "", "import", path); code == 0 {
			t.Error("bundle import on a corrupt store should fail")
		}
	})
	t.Run("locked vault", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "bk", Value: "bv"}), "bp")
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		t.Setenv(EnvExportPassphrase, "bp")
		if code, _, e := runVault(t, "", "import", path); code == 0 || !strings.Contains(e, "locked") {
			t.Errorf("bundle import into a locked vault = %d, %q", code, e)
		}
	})
	t.Run("wrong vault passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "bk", Value: "bv"}), "bp")
		t.Setenv(EnvExportPassphrase, "bp")
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "import", path); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("bundle import with wrong vault pass = %d, %q", code, e)
		}
	})
	t.Run("read scope cannot import", func(t *testing.T) {
		w4EnvelopeVault(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "bk", Value: "bv"}), "bp")
		t.Setenv(EnvExportPassphrase, "bp")
		setPass(t, "reader-pass")
		if code, _, e := runVault(t, "", "import", path); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("bundle import by a read password = %d, %q", code, e)
		}
	})
	t.Run("write scope blocked outside its namespace", func(t *testing.T) {
		w4EnvelopeVault(t)
		mustAddPassword(t, "test-pass", "writer-pass", "writer", "--scope=write", "--namespaces=proj")
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "other:x", Value: "v"}), "bp")
		t.Setenv(EnvExportPassphrase, "bp")
		setPass(t, "writer-pass")
		if code, _, e := runVault(t, "", "import", path); code == 0 ||
			!strings.Contains(e, "storing other:x") {
			t.Errorf("bundle import outside the writer's namespace = %d, %q", code, e)
		}
	})
	t.Run("external change blocks the metadata commit", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		path := w4WriteBundle(t, w4BundleFor(t, exportAlias{Name: "proj:newk", Value: "nv"}), "bp")
		t.Setenv(EnvExportPassphrase, "bp")
		// Tamper: still parses, but no longer matches the sidecar.
		data, err := os.ReadFile(StorePath(home))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(StorePath(home), append(data, ' '), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "import", path); code == 0 ||
			!strings.Contains(e, "changed outside aql") {
			t.Errorf("bundle import over an external change = %d, %q", code, e)
		}
	})
}
