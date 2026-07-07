package vault

// session_seam7_test.go — authenticate/openSession/buildSession/setValue
// and ensureSessionNamespace error and skip arms, via hand-built sessions,
// crafted stores, and the s7 crypto seams (design/TEST-SEAMS.10.md).

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// s7errReader fails on the first read, driving readPassphrase's read arm.
type s7errReader struct{}

func (s7errReader) Read([]byte) (int, error) { return 0, errS7Boom }

func s7b64(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }

func TestS7_SessionCloseNil(t *testing.T) {
	var s *Session
	s.Close() // must not panic
}

func TestS7_ResolveBackendAuto(t *testing.T) {
	if got := resolveBackend(&Store{}); got == "" {
		t.Error("resolveBackend(empty) returned empty")
	}
	if got := resolveBackend(&Store{Backend: BackendAuto}); got == "" {
		t.Error("resolveBackend(auto) returned empty")
	}
	if got := resolveBackend(&Store{Backend: BackendFile}); got != BackendFile {
		t.Errorf("resolveBackend(file) = %q, want file", got)
	}
}

func TestS7_ReadPassphraseReadError(t *testing.T) {
	t.Setenv(EnvPassphrase, "")
	if _, err := readPassphrase(s7errReader{}, io.Discard, "p"); err == nil {
		t.Fatal("expected a read error from readPassphrase")
	}
}

func TestS7_AuthenticateReadPassphraseError(t *testing.T) {
	home := w4EnvelopeVault(t)
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	t.Setenv(EnvPassphrase, "") // force the prompt path
	// nil stdin + no env passphrase → readPassphrase errors → authenticate
	// propagates it.
	if _, err := authenticate(s, home, nil, io.Discard, "p"); err == nil {
		t.Fatal("expected authenticate to fail without a passphrase source")
	}
}

func TestS7_AuthenticateWithAutoNonFileBackend(t *testing.T) {
	stubDir := t.TempDir()
	stubBin(t, stubDir, "secret-tool", "exit 0\n")
	prependPath(t, stubDir)
	home := testHome(t)
	// Backend "" → BackendAuto → autoBackend() = secret-service (stub);
	// resolved != file → a keychain admin session with no passphrase.
	sess, err := authenticateWith(&Store{Backend: ""}, home, "")
	if err != nil {
		t.Fatalf("authenticateWith(auto) = %v", err)
	}
	if sess == nil || sess.Scope != ScopeAdmin {
		t.Fatalf("session = %+v, want admin", sess)
	}
	if sess.Keyring().Name() != BackendSecretService {
		t.Errorf("keyring = %q, want secret-service", sess.Keyring().Name())
	}
}

func TestS7_OpenSessionMissingVaultSalt(t *testing.T) {
	if _, err := openSession(&Store{VaultSalt: ""}, t.TempDir(), "p"); err == nil ||
		!strings.Contains(err.Error(), "vault salt") {
		t.Fatalf("openSession(no salt) err = %v", err)
	}
}

func TestS7_OpenSessionMasterKEKError(t *testing.T) {
	old := s7vaultMasterKEK
	s7vaultMasterKEK = func(string, []byte) ([]byte, error) { return nil, errS7Boom }
	t.Cleanup(func() { s7vaultMasterKEK = old })
	s := &Store{VaultSalt: s7b64(16)}
	if _, err := openSession(s, t.TempDir(), "p"); !errors.Is(err, errS7Boom) {
		t.Fatalf("openSession master-KEK arm err = %v, want boom", err)
	}
}

func TestS7_OpenSessionSlotSaltDecodeSkips(t *testing.T) {
	s := &Store{
		VaultSalt: s7b64(16),
		Passwords: []PasswordSlot{{Name: "a", Salt: "!!not-base64!!"}},
	}
	if _, err := openSession(s, t.TempDir(), "p"); !errors.Is(err, errSlotAuth) {
		t.Fatalf("openSession(bad slot salt) err = %v, want errSlotAuth", err)
	}
}

func TestS7_OpenSessionSlotKEKErrorSkips(t *testing.T) {
	old := s7slotKEK
	s7slotKEK = func([]byte, []byte) ([]byte, error) { return nil, errS7Boom }
	t.Cleanup(func() { s7slotKEK = old })
	s := &Store{
		VaultSalt: s7b64(16),
		Passwords: []PasswordSlot{{Name: "a", Salt: s7b64(16)}},
	}
	if _, err := openSession(s, t.TempDir(), "p"); !errors.Is(err, errSlotAuth) {
		t.Fatalf("openSession(slotKEK err) err = %v, want errSlotAuth", err)
	}
}

func TestS7_OpenSessionTamperedPrivKey(t *testing.T) {
	home := w4EnvelopeVault(t)
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	// Corrupt every slot's sealed private key. The slot whose verifier
	// matches "test-pass" (the admin) then fails at openPrivKey — the arm
	// after a successful verifier match.
	bad := s7b64(40)
	for i := range s.Passwords {
		s.Passwords[i].EncPrivKey = bad
	}
	if _, err := openSession(s, home, "test-pass"); err == nil {
		t.Fatal("expected openPrivKey failure after a verifier match")
	}
}

func TestS7_BuildSessionInvalidPublicKey(t *testing.T) {
	var priv [32]byte
	_, err := buildSession(&Store{Backend: BackendFile}, t.TempDir(),
		&PasswordSlot{PublicKey: "!!not-base64!!"}, &priv)
	if err == nil || !strings.Contains(err.Error(), "invalid slot public key") {
		t.Fatalf("buildSession(bad pub) err = %v", err)
	}
}

func TestS7_BuildSessionKeyringError(t *testing.T) {
	// A non-file backend with no helper on PATH makes openEnvelopeKeyring
	// fail after the public-key check passes.
	t.Setenv("PATH", t.TempDir())
	var priv [32]byte
	slot := &PasswordSlot{PublicKey: s7b64(32)}
	if _, err := buildSession(&Store{Backend: BackendSecretService}, t.TempDir(), slot, &priv); err == nil {
		t.Fatal("expected openEnvelopeKeyring error for secret-service with no helper")
	}
}

func TestS7_BuildSessionUnopenableWrappedKeySkipped(t *testing.T) {
	var priv [32]byte
	slot := &PasswordSlot{
		PublicKey:   s7b64(32),
		WrappedKeys: map[string]string{"proj#dead": "!!not-openable!!"},
	}
	sess, err := buildSession(&Store{Backend: BackendFile}, t.TempDir(), slot, &priv)
	if err != nil {
		t.Fatalf("buildSession = %v, want nil (unopenable wrapped key skipped)", err)
	}
	if len(sess.ndks) != 0 {
		t.Errorf("ndks = %v, want empty", sess.ndks)
	}
}

func TestS7_SetValueNamespaceNotWritable(t *testing.T) {
	sess := &Session{
		Slot:    &PasswordSlot{},
		Scope:   "write",
		current: map[string]string{"ns": "abcd"},
		ndks:    map[string][]byte{},
	}
	if err := sess.setValue("a", "ns", "v"); err == nil ||
		!strings.Contains(err.Error(), "cannot write namespace") {
		t.Fatalf("setValue(no ndk) err = %v", err)
	}
}

func TestS7_SetValueBadNDKID(t *testing.T) {
	sess := &Session{
		Slot:    &PasswordSlot{},
		current: map[string]string{"ns": "zz"}, // not hex
		ndks:    map[string][]byte{"ns#zz": make([]byte, 32)},
	}
	if err := sess.setValue("a", "ns", "v"); err == nil {
		t.Fatal("expected a hex-decode error for a non-hex NDK id")
	}
}

func TestS7_SetValueSealError(t *testing.T) {
	// A 3-byte NDK is not a valid AES key, so sealValue fails.
	sess := &Session{
		Slot:    &PasswordSlot{},
		current: map[string]string{"ns": "ab"},
		ndks:    map[string][]byte{"ns#ab": {1, 2, 3}},
	}
	if err := sess.setValue("a", "ns", "v"); err == nil {
		t.Fatal("expected a sealValue error for a short NDK")
	}
}

func TestS7_EnsureNamespaceLoadStoreError(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	w4CorruptStore(t, home)
	if err := ensureSessionNamespace(home, sess, "s7fresh"); err == nil {
		t.Fatal("expected LoadStore error inside the lock")
	}
}

func TestS7_EnsureNamespaceNewNDKError(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	old := s7newNDK
	s7newNDK = func() ([]byte, error) { return nil, errS7Boom }
	t.Cleanup(func() { s7newNDK = old })
	if err := ensureSessionNamespace(home, sess, "s7fresh"); !errors.Is(err, errS7Boom) {
		t.Fatalf("newNDK arm err = %v, want boom", err)
	}
}

func TestS7_EnsureNamespaceNewNDKIDError(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	old := s7newNDKID
	s7newNDKID = func() ([]byte, error) { return nil, errS7Boom }
	t.Cleanup(func() { s7newNDKID = old })
	if err := ensureSessionNamespace(home, sess, "s7fresh"); !errors.Is(err, errS7Boom) {
		t.Fatalf("newNDKID arm err = %v, want boom", err)
	}
}

func TestS7_EnsureNamespaceInitsNilCurrentNDK(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	if sess.IntegrityKey() == nil {
		t.Fatal("admin session unexpectedly has no integrity key")
	}
	// Rewrite the on-disk store with CurrentNDK removed so the store loaded
	// inside the lock has a nil map, exercising the init arm. The in-memory
	// session keeps its integrity key, so the admin re-seal still verifies.
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	s.CurrentNDK = nil
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(home), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSessionNamespace(home, sess, "s7fresh"); err != nil {
		t.Fatalf("ensureSessionNamespace(nil CurrentNDK) = %v", err)
	}
	if sess.current["s7fresh"] == "" {
		t.Error("session did not learn the new namespace id")
	}
}

func TestS7_EnsureNamespaceSealError(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)
	// Tamper the admin slot's PubMAC on disk so sealNDKToAdmins' integrity
	// check fails when re-sealing the new NDK.
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Passwords {
		if s.Passwords[i].Scope == ScopeAdmin {
			s.Passwords[i].PubMAC = "deadbeef"
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(home), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSessionNamespace(home, sess, "s7fresh"); err == nil ||
		!strings.Contains(err.Error(), "integrity check") {
		t.Fatalf("sealNDKToAdmins arm err = %v, want an integrity failure", err)
	}
}
