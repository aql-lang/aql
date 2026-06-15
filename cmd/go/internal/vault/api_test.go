package vault

import (
	"io"
	"strings"
	"testing"
)

// TestReadWriteSecretRoundTrip exercises the exported ReadSecret/
// WriteSecret helpers that aql login/publish use, on a legacy vault and
// then on an envelope (scoped-password) vault.
func TestReadWriteSecretRoundTrip(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	if err := WriteSecret(home, "regtok", "secret-123", "test", strings.NewReader(""), io.Discard); err != nil {
		t.Fatalf("WriteSecret (legacy): %v", err)
	}
	got, err := ReadSecret(home, "regtok", strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatalf("ReadSecret (legacy): %v", err)
	}
	if got != "secret-123" {
		t.Errorf("legacy round-trip = %q, want secret-123", got)
	}

	// Migrate to an envelope vault; the same helpers must keep working
	// under the admin session.
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if err := WriteSecret(home, "regtok", "secret-456", "test", strings.NewReader(""), io.Discard); err != nil {
		t.Fatalf("WriteSecret (envelope): %v", err)
	}
	got, err = ReadSecret(home, "regtok", strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatalf("ReadSecret (envelope): %v", err)
	}
	if got != "secret-456" {
		t.Errorf("envelope round-trip = %q, want secret-456", got)
	}

	// A missing alias is an error, not an empty string.
	if _, err := ReadSecret(home, "nope", strings.NewReader(""), io.Discard); err == nil {
		t.Error("ReadSecret of a missing alias should error")
	}
}
