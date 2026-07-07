package vault

// keyring_wave4_test.go — file/envelope keyring container error arms
// (unreadable paths, folder-blocked saves, malformed content), the
// macOS-keychain round-trip failure arms via PATH stubs, and the
// 1Password delete success path via a stub.

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// w4BlockedFolder returns a path that exists as a regular FILE, so any
// MkdirAll(folder) inside the keyring fails.
func w4BlockedFolder(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// w4DirAt makes the keyring path itself a directory, so ReadFile fails
// with a non-IsNotExist error.
func w4DirAt(t *testing.T) string {
	t.Helper()
	folder := t.TempDir()
	if err := os.Mkdir(filepath.Join(folder, vaultFileName("keyring")), 0o755); err != nil {
		t.Fatal(err)
	}
	return folder
}

func TestFileKeyringErrorArms(t *testing.T) {
	t.Run("load path unreadable", func(t *testing.T) {
		f := &fileKeyring{folder: w4DirAt(t), pass: "p"}
		if _, err := f.load(); err == nil {
			t.Error("load with a directory at the keyring path should error")
		}
		if err := f.Set("a", "v"); err == nil {
			t.Error("Set should surface the load failure")
		}
		if err := f.Delete("a"); err == nil {
			t.Error("Delete should surface the load failure")
		}
	})
	t.Run("save folder blocked", func(t *testing.T) {
		f := &fileKeyring{folder: filepath.Join(w4BlockedFolder(t), "sub"), pass: "p"}
		if err := f.save(map[string]string{"a": "v"}); err == nil {
			t.Error("save under a file-blocked folder should error")
		}
	})
	t.Run("lines without tabs are skipped", func(t *testing.T) {
		folder := t.TempDir()
		plain := "no-tab-line\nk\t" + base64.StdEncoding.EncodeToString([]byte("v")) + "\n"
		enc, err := encryptBlob([]byte(plain), "p")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, vaultFileName("keyring")), enc, 0o600); err != nil {
			t.Fatal(err)
		}
		f := &fileKeyring{folder: folder, pass: "p"}
		m, err := f.load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(m) != 1 || m["k"] != "v" {
			t.Errorf("load = %v, want only k=v", m)
		}
	})
}

func TestEnvelopeKeyringErrorArms(t *testing.T) {
	t.Run("load path unreadable", func(t *testing.T) {
		f := &envelopeFileKeyring{folder: w4DirAt(t)}
		if _, err := f.loadFile(); err == nil {
			t.Error("loadFile with a directory at the path should error")
		}
		if err := f.Set("a", "v"); err == nil {
			t.Error("Set should surface the load failure")
		}
		if err := f.Delete("a"); err == nil {
			t.Error("Delete should surface the load failure")
		}
		if _, err := f.list(); err == nil {
			t.Error("list should surface the load failure")
		}
		if _, _, err := f.keyringIdentity(); err == nil {
			t.Error("keyringIdentity should surface the load failure")
		}
	})
	t.Run("nil entries initialized on load", func(t *testing.T) {
		folder := t.TempDir()
		body := append([]byte(keyringMagic), byte(envelopeKeyringFormat))
		body = append(body, []byte(`{"keyring_id":"w4","generation":3}`)...)
		if err := os.WriteFile(filepath.Join(folder, vaultFileName("keyring")), body, 0o600); err != nil {
			t.Fatal(err)
		}
		f := &envelopeFileKeyring{folder: folder}
		ekf, err := f.loadFile()
		if err != nil {
			t.Fatalf("loadFile: %v", err)
		}
		if ekf.Entries == nil {
			t.Error("Entries not initialized")
		}
		if _, err := f.Get("missing"); err != ErrNotFound {
			t.Errorf("Get(missing) = %v, want ErrNotFound", err)
		}
	})
	t.Run("save folder blocked and nil entries", func(t *testing.T) {
		f := &envelopeFileKeyring{folder: filepath.Join(w4BlockedFolder(t), "sub")}
		if err := f.saveFile(&envelopeKeyringFile{}); err == nil {
			t.Error("saveFile under a file-blocked folder should error")
		}
		ok := &envelopeFileKeyring{folder: t.TempDir()}
		if err := ok.saveFile(&envelopeKeyringFile{}); err != nil {
			t.Fatalf("saveFile with nil entries: %v", err)
		}
		id, gen, err := ok.keyringIdentity()
		if err != nil || id == "" || gen != 1 {
			t.Errorf("keyringIdentity after first save = %q, %d, %v", id, gen, err)
		}
	})
}

func TestKeyringFileFormatArms(t *testing.T) {
	if _, err := keyringFileFormat(w4DirAt(t)); err == nil {
		t.Error("keyringFileFormat with a directory at the path should error")
	}
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, vaultFileName("keyring")), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := keyringFileFormat(folder); err != nil || got != 0 {
		t.Errorf("empty keyring file format = %d, %v; want 0", got, err)
	}
}

// TestMacKeychainRoundTripFailures uses a security stub whose read-back
// misbehaves, driving Set's verify-and-clean-up arms.
func TestMacKeychainRoundTripFailures(t *testing.T) {
	t.Run("read-back fails", func(t *testing.T) {
		dir := t.TempDir()
		stubBin(t, dir, "security", `case "$1" in
-i) cat > /dev/null; exit 0;;
find-generic-password) echo "boom" >&2; exit 9;;
delete-generic-password) exit 0;;
esac
exit 3
`)
		prependPath(t, dir)
		var k macKeychain
		if err := k.Set("a", "v"); err == nil ||
			!strings.Contains(err.Error(), "could not read it back") {
			t.Errorf("read-back failure = %v", err)
		}
	})
	t.Run("round-trip mismatch", func(t *testing.T) {
		dir := t.TempDir()
		stubBin(t, dir, "security", `case "$1" in
-i) cat > /dev/null; exit 0;;
find-generic-password) printf 'different-value\n'; exit 0;;
delete-generic-password) exit 0;;
esac
exit 3
`)
		prependPath(t, dir)
		var k macKeychain
		if err := k.Set("a", "v"); err == nil ||
			!strings.Contains(err.Error(), "did not round-trip") {
			t.Errorf("round-trip mismatch = %v", err)
		}
		// Delete success path (exit 0).
		if err := k.Delete("a"); err != nil {
			t.Errorf("stub delete: %v", err)
		}
	})
}

func TestOnePasswordDeleteSuccess(t *testing.T) {
	dir := t.TempDir()
	stubBin(t, dir, "op", `verb="$2"
case "$verb" in
delete) exit 0;;
esac
exit 3
`)
	prependPath(t, dir)
	k := &onePasswordKeyring{}
	if err := k.Delete("a"); err != nil {
		t.Errorf("op delete success = %v", err)
	}
}
