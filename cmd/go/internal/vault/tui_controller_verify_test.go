package vault

import "testing"

// TestVerifyPassphraseFileBackend is the regression for the interactive-TUI
// corruption: a mistyped file-backend passphrase used to be silently
// accepted (authenticateWith builds the keyring without verifying, deferring
// the failure to the first value read). The TUI then cached the wrong
// passphrase, never re-prompted, and could run a mutation (e.g. rename)
// under it. verifyPassphrase now decrypts the AES-GCM keyring so a wrong
// passphrase is rejected up front.
func TestVerifyPassphraseFileBackend(t *testing.T) {
	ctl, home := newTestController(t)

	// Store a secret so the keyring holds ciphertext under "test-pass"
	// (init already sealed integrity material, so the keyring exists even
	// before this — a wrong passphrase is catchable either way).
	if err := ctl.addSecret("tok", "s3cr3t", "", "", ""); err != nil {
		t.Fatalf("addSecret: %v", err)
	}
	s, err := requireStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPassphrase(s, home, "test-pass"); err != nil {
		t.Errorf("correct passphrase rejected: %v", err)
	}
	if err := verifyPassphrase(s, home, "wrong-pass"); err == nil {
		t.Error("wrong file-backend passphrase accepted (the bug)")
	}
	if err := verifyPassphrase(s, home, ""); err == nil {
		t.Error("empty passphrase accepted for a file backend")
	}

	// A file-backend vault with NO keyring file yet (nothing sealed) cannot
	// be verified — any passphrase is provisionally accepted, since it is the
	// key that will encrypt the first write (first-use semantics). Point the
	// folder override at an empty dir so vaultFolder resolves there.
	fresh := t.TempDir()
	t.Setenv(EnvFolder, fresh)
	if err := verifyPassphrase(&Store{Backend: BackendFile}, fresh, "anything"); err != nil {
		t.Errorf("file vault with no keyring should accept any passphrase (first-use): %v", err)
	}
}

// TestAuthenticateRejectsWrongPassphrase confirms the controller does not
// cache a wrong passphrase or flip authed, so the TUI re-prompts (huh shows
// the validation error) instead of wedging every later operation.
func TestAuthenticateRejectsWrongPassphrase(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("tok", "s3cr3t", "", "", ""); err != nil {
		t.Fatalf("addSecret: %v", err)
	}
	ctl.authed, ctl.pass = false, ""
	if err := ctl.authenticate("wrong-pass"); err == nil {
		t.Fatal("authenticate accepted a wrong passphrase")
	}
	if ctl.authed || ctl.pass != "" {
		t.Errorf("controller cached a wrong passphrase: authed=%v pass=%q", ctl.authed, ctl.pass)
	}
	// The correct passphrase still authenticates and caches cleanly.
	if err := ctl.authenticate("test-pass"); err != nil {
		t.Fatalf("correct passphrase rejected: %v", err)
	}
	if !ctl.authed || ctl.pass != "test-pass" {
		t.Errorf("correct passphrase not cached: authed=%v pass=%q", ctl.authed, ctl.pass)
	}
}

// TestVerifyPassphraseEnvelopeAndHost covers the envelope-keyslot branch
// (openSession authenticates against a slot verifier) and the host-keychain
// branch (no passphrase to verify).
func TestVerifyPassphraseEnvelopeAndHost(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk-proj\n", "add", "--from-stdin", "proj:apikey"); code != 0 {
		t.Fatalf("add proj:apikey: %s", e)
	}
	// The first password add migrates the vault to envelope mode; the admin
	// passphrase stays "test-pass".
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")
	s, err := requireStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasPasswordSlots() {
		t.Fatal("expected password slots after migration")
	}
	if err := verifyPassphrase(s, home, "test-pass"); err != nil {
		t.Errorf("envelope: correct admin passphrase rejected: %v", err)
	}
	if err := verifyPassphrase(s, home, "totally-wrong"); err == nil {
		t.Error("envelope: wrong passphrase accepted")
	}

	// A host-keychain vault has no passphrase to verify — the OS guards
	// access — so verification is a no-op accept.
	if err := verifyPassphrase(&Store{Backend: BackendKeychain}, home, "anything"); err != nil {
		t.Errorf("host keychain: verify should be a no-op, got %v", err)
	}
}
