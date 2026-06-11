package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// legacyEncrypt reproduces the original headerless keyring layout
// (salt|nonce|ciphertext with AAD = salt) so tests can prove the
// current binary still reads vaults written before the format header.
func legacyEncrypt(t *testing.T, plain []byte, pass string) []byte {
	t.Helper()
	salt := make([]byte, keyringSaltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	key, err := scryptKey(pass, salt)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, nonce, plain, salt)
	out := append([]byte{}, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out
}

// keyringEntry renders one stored line in the on-disk keyring format,
// "key\tbase64(value)\n".
func keyringEntry(key, value string) string {
	return key + "\t" + base64.StdEncoding.EncodeToString([]byte(value)) + "\n"
}

func TestKeyringHeaderRoundTrip(t *testing.T) {
	plain := []byte(keyringEntry("k", "secret"))
	blob, err := encryptBlob(plain, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(blob, []byte(keyringMagic)) {
		t.Errorf("encrypted blob missing magic header")
	}
	if int(blob[len(keyringMagic)]) != keyringFormat {
		t.Errorf("format byte = %d, want %d", blob[len(keyringMagic)], keyringFormat)
	}
	got, err := decryptBlob(blob, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch: %q != %q", got, plain)
	}
	if _, err := decryptBlob(blob, "wrong"); err == nil {
		t.Errorf("wrong passphrase should fail")
	}
}

func TestKeyringReadsLegacyBlob(t *testing.T) {
	plain := []byte(keyringEntry("legacy", "value"))
	legacy := legacyEncrypt(t, plain, "pw")
	if bytes.HasPrefix(legacy, []byte(keyringMagic)) {
		t.Fatal("legacy fixture unexpectedly carries the magic header")
	}
	got, err := decryptBlob(legacy, "pw")
	if err != nil {
		t.Fatalf("decrypting legacy blob: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("legacy round-trip mismatch: %q != %q", got, plain)
	}
}

func TestKeyringRejectsNewerFormat(t *testing.T) {
	blob, err := encryptBlob([]byte("x"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	// Bump the format byte beyond what this binary understands. The
	// version check precedes decryption, so this is reported as an
	// upgrade hint rather than a generic corruption error.
	blob[len(keyringMagic)] = byte(keyringFormat + 1)
	_, err = decryptBlob(blob, "pw")
	if err == nil {
		t.Fatal("expected error for newer keyring format")
	}
	if !strings.Contains(err.Error(), "upgrade aql") {
		t.Errorf("error should advise upgrading aql, got %q", err.Error())
	}
}

// TestFileKeyringUpgradesLegacyOnSave proves a vault written in the
// legacy layout is read transparently and rewritten with the format
// header on the next mutation.
func TestFileKeyringUpgradesLegacyOnSave(t *testing.T) {
	dir := t.TempDir()
	kr := &fileKeyring{folder: dir, pass: "pw"}

	// Seed the on-disk file with a legacy-format single entry.
	legacy := legacyEncrypt(t, []byte(keyringEntry("a", "first")), "pw")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kr.path(), legacy, 0600); err != nil {
		t.Fatal(err)
	}

	// Reading the legacy file works.
	got, err := kr.Get("a")
	if err != nil {
		t.Fatalf("Get on legacy file: %s", err)
	}
	if got != "first" {
		t.Errorf("Get = %q, want first", got)
	}

	// A mutation rewrites the file; it is now headered.
	if err := kr.Set("b", "second"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(kr.path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte(keyringMagic)) {
		t.Errorf("file not upgraded to headered format after save")
	}
	// Both entries survive the upgrade.
	for k, want := range map[string]string{"a": "first", "b": "second"} {
		v, err := kr.Get(k)
		if err != nil || v != want {
			t.Errorf("Get(%q) = %q, %v; want %q", k, v, err, want)
		}
	}
}
