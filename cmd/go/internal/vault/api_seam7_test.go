package vault

// api_seam7_test.go — seam-7 coverage for api.go: the ReadSecret and
// WriteSecret error arms (locked vault, auth failure, scope/namespace
// denial, invalid alias, write failure).

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// c7lockVault flips the on-disk store's Locked flag so the api entry
// points hit their errLocked arms.
func c7lockVault(t *testing.T, home string) {
	t.Helper()
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	s.Locked = true
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
}

func TestC7ReadSecretLocked(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	c7lockVault(t, home)
	if _, err := ReadSecret(home, "k", strings.NewReader(""), io.Discard); !errors.Is(err, errLocked) {
		t.Fatalf("ReadSecret locked = %v, want errLocked", err)
	}
}

func TestC7ReadSecretAuthError(t *testing.T) {
	home := w4EnvelopeVault(t)
	setPass(t, "wrong-pass")
	if _, err := ReadSecret(home, "proj:k", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("ReadSecret with wrong passphrase should fail")
	}
}

func TestC7ReadSecretScopeError(t *testing.T) {
	home := w4EnvelopeVault(t)
	// Admin adds a secret in a namespace the reader does not cover.
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "other:k"); code != 0 {
		t.Fatalf("add other:k: %s", e)
	}
	setPass(t, "reader-pass")
	_, err := ReadSecret(home, "other:k", strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("ReadSecret out-of-scope = %v, want namespace denial", err)
	}
}

func TestC7WriteSecretLocked(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	c7lockVault(t, home)
	if err := WriteSecret(home, "k", "v", "src", strings.NewReader(""), io.Discard); !errors.Is(err, errLocked) {
		t.Fatalf("WriteSecret locked = %v, want errLocked", err)
	}
}

func TestC7WriteSecretResolveError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if err := WriteSecret(home, "bad name!", "v", "src", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("WriteSecret with invalid alias should fail")
	}
}

func TestC7WriteSecretAuthError(t *testing.T) {
	home := w4EnvelopeVault(t)
	setPass(t, "wrong-pass")
	if err := WriteSecret(home, "proj:k", "v2", "src", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("WriteSecret with wrong passphrase should fail")
	}
}

func TestC7WriteSecretScopeError(t *testing.T) {
	home := w4EnvelopeVault(t)
	setPass(t, "reader-pass")
	err := WriteSecret(home, "proj:k", "v2", "src", strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("WriteSecret read-only = %v, want scope denial", err)
	}
}
