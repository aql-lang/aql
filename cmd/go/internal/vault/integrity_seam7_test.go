package vault

// integrity_seam7_test.go — sidecar/anchor/quarantine/sealIntegrity and
// checkStoreIntegrity error and journal-head arms, driven with on-disk
// fixtures and the jsonMarshal / createTemp seams.

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestS7_StoreContentHashMarshalError(t *testing.T) {
	s := &Store{Config: map[string]any{"c": make(chan int)}}
	if got := storeContentHash(s); got != "" {
		t.Errorf("storeContentHash(unmarshalable) = %q, want empty", got)
	}
}

func TestS7_ReadSidecarReadError(t *testing.T) {
	dir, _ := integHome(t, 1)
	// A directory at the sidecar path makes ReadFile fail with a non-
	// IsNotExist error.
	if err := os.Mkdir(sidecarPath(dir), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := readSidecar(dir); err == nil {
		t.Fatal("expected a read error for a directory sidecar path")
	}
}

func TestS7_ReadSidecarParseError(t *testing.T) {
	dir, _ := integHome(t, 1)
	if err := os.WriteFile(sidecarPath(dir), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSidecar(dir); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("readSidecar parse err = %v, want a parse error", err)
	}
}

func TestS7_WriteSidecarMarshalError(t *testing.T) {
	old := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errS7Boom }
	t.Cleanup(func() { jsonMarshal = old })
	dir := t.TempDir()
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	if err := writeSidecar(dir, 1, []byte("{}"), nil); !errors.Is(err, errS7Boom) {
		t.Fatalf("writeSidecar marshal arm err = %v, want boom", err)
	}
}

func TestS7_ReadAnchorNonNumeric(t *testing.T) {
	kr := &envelopeFileKeyring{folder: t.TempDir()}
	if err := kr.Set(anchorKeyName, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if got, _ := readAnchor(kr); got != 0 {
		t.Errorf("readAnchor(non-numeric) = %d, want 0", got)
	}
}

func TestS7_QuarantineStoreReadError(t *testing.T) {
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	dir := t.TempDir() // no store file
	if _, err := quarantineStore(dir); err == nil {
		t.Fatal("expected read error quarantining a missing store")
	}
}

func TestS7_QuarantineStoreWriteError(t *testing.T) {
	dir, _ := integHome(t, 3)
	old := createTemp
	createTemp = func(string, string) (*os.File, error) { return nil, errS7Boom }
	t.Cleanup(func() { createTemp = old })
	if _, err := quarantineStore(dir); !errors.Is(err, errS7Boom) {
		t.Fatalf("quarantine write arm err = %v, want boom", err)
	}
}

func TestS7_SealIntegrityNilSession(t *testing.T) {
	dir, _ := integHome(t, 1)
	sealIntegrity(dir, nil) // no-op, must not panic
	if _, err := os.Stat(sidecarPath(dir)); !os.IsNotExist(err) {
		t.Errorf("nil-session sealIntegrity wrote a sidecar: %v", err)
	}
}

func TestS7_SealIntegrityStoreReadError(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	if sess.IntegrityKey() == nil {
		t.Fatal("admin session unexpectedly has no integrity key")
	}
	other := t.TempDir() // no store file
	sealIntegrity(other, sess)
	if _, err := os.Stat(sidecarPath(other)); !os.IsNotExist(err) {
		t.Errorf("sealIntegrity wrote a sidecar despite a missing store: %v", err)
	}
}

func TestS7_SealIntegrityUnparsableStore(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	other := t.TempDir()
	if err := os.MkdirAll(vaultFolder(other), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(other), []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	sealIntegrity(other, sess)
	if _, err := os.Stat(sidecarPath(other)); !os.IsNotExist(err) {
		t.Errorf("sealIntegrity wrote a sidecar for an unparsable store: %v", err)
	}
}

func TestS7_CheckStoreIntegrityReadError(t *testing.T) {
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	dir := t.TempDir() // no store
	if got, err := checkStoreIntegrity(dir, nil, 0); got != IntegrityCorrupt || err == nil {
		t.Fatalf("checkStoreIntegrity(missing) = %v, %v; want corrupt + err", got, err)
	}
}

func TestS7_CheckStoreIntegritySidecarError(t *testing.T) {
	dir, _ := integHome(t, 2)
	if err := os.WriteFile(sidecarPath(dir), []byte("{bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := checkStoreIntegrity(dir, nil, 0); got != IntegrityCorrupt || err == nil {
		t.Fatalf("checkStoreIntegrity(bad sidecar) = %v, %v; want corrupt + err", got, err)
	}
}

func TestS7_CheckStoreIntegrityJournalHeadUnverifiedNoKey(t *testing.T) {
	ik := mustRandom(t, 32)
	dir, data := integHome(t, 5)
	sha := storeSHA256(data)
	if err := appendJournal(dir, ik, 5, "u", "vault.add", sha, &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	// No sidecar; store matches the journal head; no key to verify with.
	if got, _ := checkStoreIntegrity(dir, nil, 0); got != IntegrityUnverified {
		t.Errorf("got %v, want unverified (no key)", got)
	}
}

func TestS7_CheckStoreIntegrityJournalHeadEmptyHMAC(t *testing.T) {
	dir, data := integHome(t, 5)
	sha := storeSHA256(data)
	// A journal head written without an integrity key has an empty HMAC.
	if err := appendJournal(dir, nil, 5, "u", "vault.add", sha, &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	if got, _ := checkStoreIntegrity(dir, mustRandom(t, 32), 0); got != IntegrityUnverified {
		t.Errorf("got %v, want unverified (empty head HMAC)", got)
	}
}

func TestS7_CheckStoreIntegrityJournalHeadHMACInvalid(t *testing.T) {
	dir, data := integHome(t, 5)
	sha := storeSHA256(data)
	if err := appendJournal(dir, mustRandom(t, 32), 5, "u", "vault.add", sha, &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	// Verify with a DIFFERENT key → the chained head HMAC mismatches.
	if got, _ := checkStoreIntegrity(dir, mustRandom(t, 32), 0); got != IntegrityHMACInvalid {
		t.Errorf("got %v, want hmac-invalid", got)
	}
}

func TestS7_CheckStoreIntegrityKeylessSidecarWithKey(t *testing.T) {
	dir, data := integHome(t, 5)
	_ = writeSidecar(dir, 5, data, nil) // keyless sidecar (no HMAC)
	if got, _ := checkStoreIntegrity(dir, mustRandom(t, 32), 0); got != IntegrityUnverified {
		t.Errorf("got %v, want unverified (keyless sidecar, keyed session)", got)
	}
}
