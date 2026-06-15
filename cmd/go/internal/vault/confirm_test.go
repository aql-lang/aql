package vault

import (
	"strings"
	"testing"
)

// TestDestructiveConfirmation verifies the cross-cutting policy: deleting
// or overwriting a secret non-interactively is refused without --yes, a
// fresh add needs no confirmation, and --yes proceeds.
func TestDestructiveConfirmation(t *testing.T) {
	testHome(t)
	mustInit(t)

	// A fresh add (new alias) needs no confirmation.
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add new alias: %s", e)
	}

	// rm without --yes (non-interactive, no TTY) is refused; the secret survives.
	if code, _, _ := runVault(t, "", "rm", "k"); code == 0 {
		t.Error("rm without --yes should be refused non-interactively")
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "k"); code != 0 || strings.TrimSpace(out) != "v1" {
		t.Fatalf("k should survive a refused rm = %d/%q", code, out)
	}

	// add over an EXISTING alias without --yes is refused; value unchanged.
	if code, _, _ := runVault(t, "v2\n", "add", "--from-stdin", "k"); code == 0 {
		t.Error("overwriting add without --yes should be refused")
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); strings.TrimSpace(out) != "v1" {
		t.Errorf("value changed despite a refused overwrite: %q", out)
	}

	// --yes proceeds with the overwrite.
	if code, _, e := runVault(t, "v2\n", "add", "--yes", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add --yes overwrite: %s", e)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); strings.TrimSpace(out) != "v2" {
		t.Errorf("overwrite did not apply: %q", out)
	}

	// rm --yes proceeds.
	if code, _, e := runVault(t, "", "rm", "--yes", "k"); code != 0 {
		t.Fatalf("rm --yes: %s", e)
	}
	if code, _, _ := runVault(t, "", "get", "k"); code == 0 {
		t.Error("k should be gone after rm --yes")
	}
}
