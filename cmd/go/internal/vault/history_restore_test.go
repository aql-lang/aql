package vault

import (
	"strings"
	"testing"
)

// TestHistoryTracksGenerations confirms mutations on an envelope vault
// advance the content generation and are recorded in the journal that
// `history` reads. (Feature B activates once a vault has password slots.)
func TestHistoryTracksGenerations(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "k1"); code != 0 {
		t.Fatalf("add k1: %s", e)
	}
	if code, _, e := runVault(t, "v2\n", "add", "--from-stdin", "k2"); code != 0 {
		t.Fatalf("add k2: %s", e)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Generation < 2 {
		t.Errorf("generation = %d, want >= 2", s.Generation)
	}
	code, out, e := runVault(t, "", "history")
	if code != 0 {
		t.Fatalf("history: %s", e)
	}
	if strings.Count(out, "aliases") < 2 {
		t.Errorf("history shows too few generations:\n%s", out)
	}
}

// TestRestoreRecoversMetadata is the compelling case: an external edit
// drops an alias from vault.jsonic while the keyring value survives;
// restore reinstates the metadata and the secret reads again.
func TestRestoreRecoversMetadata(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "k1"); code != 0 {
		t.Fatalf("add k1: %s", e)
	}
	if code, _, e := runVault(t, "v2\n", "add", "--from-stdin", "k2"); code != 0 {
		t.Fatalf("add k2: %s", e)
	}

	good, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	goodGen := good.Generation // a generation where both aliases exist

	// Simulate an external edit that loses k2's metadata but NOT its
	// keyring value (e.g. a hand-edit of vault.jsonic).
	good.RemoveAlias("k2")
	if err := SaveStore(home, good); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := runVault(t, "", "list"); strings.Contains(out, "k2") {
		t.Fatalf("k2 should be gone from metadata after the edit:\n%s", out)
	}

	// Restore the metadata from the good generation (admin).
	if code, _, e := runVault(t, "", "restore", "--yes", "--generation", itoa(goodGen)); code != 0 {
		t.Fatalf("restore: %s", e)
	}
	if _, out, _ := runVault(t, "", "list"); !strings.Contains(out, "k2") {
		t.Fatalf("k2 metadata not restored:\n%s", out)
	}
	// The secret value, which lived in the keyring all along, reads again.
	if code, out, _ := runVault(t, "", "get", "--reveal", "k2"); code != 0 || strings.TrimSpace(out) != "v2" {
		t.Fatalf("get k2 after restore = %d/%q, want v2", code, out)
	}
}

// TestRestoreRequiresGenerationOrList checks the argument guard.
func TestRestoreRequiresGenerationOrList(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "", "restore"); code == 0 {
		t.Errorf("restore with no --generation/--list should fail (%s)", e)
	}
}

// itoa renders an int64 without importing strconv into the test.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
