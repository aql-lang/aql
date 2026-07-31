package vault

// export_seam7_test.go — seam-7 coverage for export.go: the terminal-refuse
// guard, the read-scope gate (c7requireScope), runExport's marshal/seal
// failure arms, importBundle's from-stdin branch, and the sealExport /
// openExport envelope-crypto error ladder (randRead / scryptKey / aes /
// gcmFromBlock seams).

import (
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func c7seedExportVault(t *testing.T) string {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sekret-value\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	return home
}

// c7randReadFailAt swaps randRead to fail on the failCall-th invocation and
// pass earlier ones through to the real implementation.
func c7randReadFailAt(t *testing.T, failCall int) {
	t.Helper()
	old := randRead
	n := 0
	randRead = func(b []byte) (int, error) {
		n++
		if n == failCall {
			return 0, errors.New("entropy boom")
		}
		return old(b)
	}
	t.Cleanup(func() { randRead = old })
}

func c7setScryptKey(t *testing.T, fn func(string, []byte) ([]byte, error)) {
	t.Helper()
	old := c7scryptKey
	c7scryptKey = fn
	t.Cleanup(func() { c7scryptKey = old })
}

func c7failGCM(t *testing.T) {
	t.Helper()
	old := gcmFromBlock
	gcmFromBlock = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm boom") }
	t.Cleanup(func() { gcmFromBlock = old })
}

func TestC7ExportRefusesTerminal(t *testing.T) {
	home := c7seedExportVault(t)
	old := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = old })

	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var errb strings.Builder
	if code := runExport(nil, home, strings.NewReader(""), f, &errb); code != 1 {
		t.Fatalf("terminal refuse code=%d", code)
	}
	if !strings.Contains(errb.String(), "refusing to write a binary bundle") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7ExportReadScopeDenied(t *testing.T) {
	home := c7seedExportVault(t)
	old := c7requireScope
	t.Cleanup(func() { c7requireScope = old })
	c7requireScope = func(*Session, Op) error { return errors.New("scope denied") }

	t.Setenv(EnvExportPassphrase, "bundlepass")
	out := filepath.Join(t.TempDir(), "b.borux")
	var errb strings.Builder
	if code := runExport([]string{"--out=" + out}, home, strings.NewReader(""), &errb, &errb); code != 1 {
		t.Fatalf("read-scope denial code=%d", code)
	}
	if !strings.Contains(errb.String(), "scope denied") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7ExportMarshalError(t *testing.T) {
	home := c7seedExportVault(t)
	t.Setenv(EnvExportPassphrase, "bundlepass")
	old := jsonMarshal
	t.Cleanup(func() { jsonMarshal = old })
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }

	out := filepath.Join(t.TempDir(), "b.borux")
	var errb strings.Builder
	if code := runExport([]string{"--out=" + out}, home, strings.NewReader(""), &errb, &errb); code != 1 {
		t.Fatalf("marshal error code=%d", code)
	}
	if !strings.Contains(errb.String(), "marshal boom") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7ExportSealError(t *testing.T) {
	home := c7seedExportVault(t)
	t.Setenv(EnvExportPassphrase, "bundlepass")
	c7failGCM(t) // gcmFromBlock is used only by the export envelope crypto

	out := filepath.Join(t.TempDir(), "b.borux")
	var errb strings.Builder
	if code := runExport([]string{"--out=" + out}, home, strings.NewReader(""), &errb, &errb); code != 1 {
		t.Fatalf("seal error code=%d", code)
	}
	if !strings.Contains(errb.String(), "gcm boom") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7ImportBundleFromStdin(t *testing.T) {
	home := c7seedExportVault(t)
	bundle := exportBundle{Version: exportVersion, Aliases: []exportAlias{{Name: "k2", Value: "imported-secret"}}}
	plain, err := jsonMarshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := sealExport(plain, "bundlepass")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvExportPassphrase, "bundlepass") // required when reading from stdin
	var out, errb strings.Builder
	code := importBundle(blob, true, home, strings.NewReader(""), &out, &errb, "", "", false)
	if code != 0 {
		t.Fatalf("import from stdin code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "imported k2") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestC7SealExportCryptoArms(t *testing.T) {
	t.Run("salt-entropy", func(t *testing.T) {
		c7randReadFailAt(t, 1)
		if _, err := sealExport([]byte("data"), "p"); err == nil {
			t.Error("salt randRead failure should propagate")
		}
	})
	t.Run("nonce-entropy", func(t *testing.T) {
		c7randReadFailAt(t, 2)
		if _, err := sealExport([]byte("data"), "p"); err == nil {
			t.Error("nonce randRead failure should propagate")
		}
	})
	t.Run("scrypt", func(t *testing.T) {
		c7setScryptKey(t, func(string, []byte) ([]byte, error) { return nil, errors.New("kdf boom") })
		if _, err := sealExport([]byte("data"), "p"); err == nil {
			t.Error("scryptKey failure should propagate")
		}
	})
	t.Run("bad-key-len", func(t *testing.T) {
		c7setScryptKey(t, func(string, []byte) ([]byte, error) { return []byte("short"), nil })
		if _, err := sealExport([]byte("data"), "p"); err == nil {
			t.Error("bad key length should fail aes.NewCipher")
		}
	})
	t.Run("gcm", func(t *testing.T) {
		c7failGCM(t)
		if _, err := sealExport([]byte("data"), "p"); err == nil {
			t.Error("gcmFromBlock failure should propagate")
		}
	})
}

func TestC7OpenExportCryptoArms(t *testing.T) {
	blob, err := sealExport([]byte("data"), "p")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("scrypt", func(t *testing.T) {
		c7setScryptKey(t, func(string, []byte) ([]byte, error) { return nil, errors.New("kdf boom") })
		if _, err := openExport(blob, "p"); err == nil {
			t.Error("scryptKey failure should propagate in openExport")
		}
	})
	t.Run("bad-key-len", func(t *testing.T) {
		c7setScryptKey(t, func(string, []byte) ([]byte, error) { return []byte("short"), nil })
		if _, err := openExport(blob, "p"); err == nil {
			t.Error("bad key length should fail aes.NewCipher in openExport")
		}
	})
	t.Run("gcm", func(t *testing.T) {
		c7failGCM(t)
		if _, err := openExport(blob, "p"); err == nil {
			t.Error("gcmFromBlock failure should propagate in openExport")
		}
	})
}
