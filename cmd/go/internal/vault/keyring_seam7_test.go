package vault

// keyring_seam7_test.go — seam7 coverage for keyring.go: the darwin/windows
// backend-dispatch arms (driven by the p7goosName seam plus PATH stubs) and
// the encrypt/decrypt/marshal/entropy error arms of the file and 1Password
// backends (driven by the randRead / p7scryptKey / gcmFromBlock / jsonMarshal
// / p7randomID seams). Seams are saved and restored with t.Cleanup; no
// t.Parallel.

import (
	"crypto/cipher"
	"testing"
)

// --- seam-swap helpers -----------------------------------------------------

func p7swapGoos(t *testing.T, goos string) {
	t.Helper()
	old := p7goosName
	p7goosName = goos
	t.Cleanup(func() { p7goosName = old })
}

func p7swapGCM(t *testing.T, fn func(cipher.Block) (cipher.AEAD, error)) {
	t.Helper()
	old := gcmFromBlock
	gcmFromBlock = fn
	t.Cleanup(func() { gcmFromBlock = old })
}

func p7swapJSONMarshal(t *testing.T, fn func(any) ([]byte, error)) {
	t.Helper()
	old := jsonMarshal
	jsonMarshal = fn
	t.Cleanup(func() { jsonMarshal = old })
}

func p7swapRandomID(t *testing.T, fn func() (string, error)) {
	t.Helper()
	old := p7randomID
	p7randomID = fn
	t.Cleanup(func() { p7randomID = old })
}

// p7cleanPath points PATH at a fresh empty dir so exec.LookPath fails for
// every helper binary, and returns the dir for later stubbing.
func p7cleanPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	return dir
}

func p7failGCM(cipher.Block) (cipher.AEAD, error) { return nil, p7boom() }
func p7failMarshal(any) ([]byte, error)           { return nil, p7boom() }
func p7failScrypt(string, []byte) ([]byte, error) { return nil, p7boom() }
func p7shortScrypt(string, []byte) ([]byte, error) {
	return []byte{1, 2, 3}, nil // not a valid AES key length
}

// --- backend dispatch (goos + LookPath) ------------------------------------

func TestP7_SelectKeyringKeychainDispatch(t *testing.T) {
	dir := p7cleanPath(t)
	p7swapGoos(t, "darwin")
	// security absent -> not-found arm.
	if _, err := selectKeyring(BackendKeychain, "", ""); err == nil {
		t.Fatal("keychain without /usr/bin/security should fail")
	}
	// security present -> macKeychain constructed.
	stubBin(t, dir, "security", "exit 0\n")
	kr, err := selectKeyring(BackendKeychain, "", "")
	if err != nil {
		t.Fatalf("keychain with security stubbed = %v", err)
	}
	if _, ok := kr.(*macKeychain); !ok {
		t.Fatalf("keychain backend = %T, want *macKeychain", kr)
	}
}

func TestP7_SelectKeyringWinCredDispatch(t *testing.T) {
	dir := p7cleanPath(t)
	p7swapGoos(t, "windows")
	if _, err := selectKeyring(BackendWinCred, "", ""); err == nil {
		t.Fatal("wincred without powershell should fail")
	}
	stubBin(t, dir, "powershell", "exit 0\n")
	kr, err := selectKeyring(BackendWinCred, "", "")
	if err != nil {
		t.Fatalf("wincred with powershell stubbed = %v", err)
	}
	if _, ok := kr.(*winCred); !ok {
		t.Fatalf("wincred backend = %T, want *winCred", kr)
	}
}

func TestP7_AutoBackendDarwin(t *testing.T) {
	dir := p7cleanPath(t)
	p7swapGoos(t, "darwin")
	stubBin(t, dir, "security", "exit 0\n")
	if got := autoBackend(); got != BackendKeychain {
		t.Fatalf("autoBackend darwin = %q, want %q", got, BackendKeychain)
	}
}

func TestP7_AutoBackendWindows(t *testing.T) {
	dir := p7cleanPath(t)
	p7swapGoos(t, "windows")
	stubBin(t, dir, "powershell", "exit 0\n")
	if got := autoBackend(); got != BackendWinCred {
		t.Fatalf("autoBackend windows = %q, want %q", got, BackendWinCred)
	}
}

// --- file backend encrypt/decrypt error arms -------------------------------

func TestP7_FileKeyringSaveEntropyFailure(t *testing.T) {
	f := &fileKeyring{folder: t.TempDir(), pass: "pw"}
	p7swapRandRead(t, p7failRead)
	if err := f.save(map[string]string{"a": "b"}); err == nil {
		t.Fatal("save should surface an encryptBlob entropy failure")
	}
}

func TestP7_EncryptBlobArms(t *testing.T) {
	t.Run("scrypt error", func(t *testing.T) {
		p7swapScryptKey(t, p7failScrypt)
		if _, err := encryptBlob([]byte("x"), "pw"); err == nil {
			t.Fatal("encryptBlob should surface a scrypt failure")
		}
	})
	t.Run("bad key length", func(t *testing.T) {
		p7swapScryptKey(t, p7shortScrypt)
		if _, err := encryptBlob([]byte("x"), "pw"); err == nil {
			t.Fatal("encryptBlob should fail aes.NewCipher for a short key")
		}
	})
	t.Run("gcm error", func(t *testing.T) {
		p7swapGCM(t, p7failGCM)
		if _, err := encryptBlob([]byte("x"), "pw"); err == nil {
			t.Fatal("encryptBlob should surface a cipher.NewGCM failure")
		}
	})
	t.Run("nonce entropy", func(t *testing.T) {
		p7swapRandRead(t, p7countRead(1)) // salt ok, nonce fails
		if _, err := encryptBlob([]byte("x"), "pw"); err == nil {
			t.Fatal("encryptBlob should surface a nonce entropy failure")
		}
	})
}

func TestP7_AESGCMOpenArms(t *testing.T) {
	salt := make([]byte, keyringSaltLen)
	nonce := make([]byte, keyringNonceLen)
	ct := make([]byte, 16)
	t.Run("scrypt error", func(t *testing.T) {
		p7swapScryptKey(t, p7failScrypt)
		if _, err := aesGCMOpen("pw", salt, nonce, ct, salt); err == nil {
			t.Fatal("aesGCMOpen should surface a scrypt failure")
		}
	})
	t.Run("bad key length", func(t *testing.T) {
		p7swapScryptKey(t, p7shortScrypt)
		if _, err := aesGCMOpen("pw", salt, nonce, ct, salt); err == nil {
			t.Fatal("aesGCMOpen should fail aes.NewCipher for a short key")
		}
	})
	t.Run("gcm error", func(t *testing.T) {
		p7swapGCM(t, p7failGCM)
		if _, err := aesGCMOpen("pw", salt, nonce, ct, salt); err == nil {
			t.Fatal("aesGCMOpen should surface a cipher.NewGCM failure")
		}
	})
}

// --- envelope keyring saveFile arms ----------------------------------------

func TestP7_EnvelopeSaveFileRandomIDFailure(t *testing.T) {
	f := &envelopeFileKeyring{folder: t.TempDir()}
	p7swapRandomID(t, func() (string, error) { return "", p7boom() })
	if err := f.saveFile(&envelopeKeyringFile{Entries: map[string]string{}}); err == nil {
		t.Fatal("saveFile should surface a KeyringID mint failure")
	}
}

func TestP7_EnvelopeSaveFileMarshalFailure(t *testing.T) {
	f := &envelopeFileKeyring{folder: t.TempDir()}
	p7swapJSONMarshal(t, p7failMarshal)
	ekf := &envelopeKeyringFile{KeyringID: "fixed-id", Entries: map[string]string{}}
	if err := f.saveFile(ekf); err == nil {
		t.Fatal("saveFile should surface a json.Marshal failure")
	}
}

// --- 1Password backend marshal arm -----------------------------------------

func TestP7_OnePasswordSetMarshalFailure(t *testing.T) {
	p7cleanPath(t) // no `op` on PATH: the best-effort pre-delete fails fast and is ignored
	p7swapJSONMarshal(t, p7failMarshal)
	k := &onePasswordKeyring{vault: "Private"}
	if err := k.Set("alias", "val"); err == nil {
		t.Fatal("onePassword Set should surface a json.Marshal failure")
	}
}
