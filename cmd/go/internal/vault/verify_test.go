package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testKeyring builds a file keyring pointed at the test vault, for
// manufacturing inconsistencies directly.
func testKeyring(t *testing.T) *fileKeyring {
	t.Helper()
	return &fileKeyring{folder: filepath.Join(os.Getenv(EnvHome), ".boru"), pass: "test-pass"}
}

func TestVerifyHealthyVault(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, out, _ := runVault(t, "", "verify")
	if code != 0 {
		t.Fatalf("verify healthy: code %d, %q", code, out)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected OK, got %q", out)
	}
}

func TestVerifyDanglingMetadata(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Delete the secret directly, leaving the metadata record.
	if err := testKeyring(t).Delete("k"); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runVault(t, "", "verify")
	if code != 2 {
		t.Errorf("verify code = %d, want 2; out=%q", code, out)
	}
	if !strings.Contains(out, "dangling metadata") || !strings.Contains(out, "k") {
		t.Errorf("verify output = %q", out)
	}

	// --prune removes the dangling record.
	if code, out, _ := runVault(t, "", "verify", "--prune"); code != 0 {
		t.Errorf("prune code = %d; out=%q", code, out)
	}
	s, _ := LoadStore(home)
	if a, _ := s.FindAlias("k"); a != nil {
		t.Error("dangling record not pruned")
	}
}

func TestVerifyOrphanedKeyringEntry(t *testing.T) {
	testHome(t)
	mustInit(t)
	// A secret with no metadata record.
	if err := testKeyring(t).Set("ghost", "secret"); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runVault(t, "", "verify")
	if code != 2 || !strings.Contains(out, "orphaned keyring entry") || !strings.Contains(out, "ghost") {
		t.Errorf("verify code=%d out=%q", code, out)
	}
	if code, _, _ := runVault(t, "", "verify", "--prune"); code != 0 {
		t.Fatal("prune")
	}
	if v, err := testKeyring(t).Get("ghost"); err != ErrNotFound {
		t.Errorf("orphan not pruned: %q, %v", v, err)
	}
}

func TestVerifyCapabilityForMissingAlias(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Manufacture a capability that points at an alias with no record.
	s, _ := LoadStore(home)
	if _, _, err := s.NewCapability("ghost", "agent", nil, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runVault(t, "", "verify")
	if code != 2 || !strings.Contains(out, "references missing alias") {
		t.Errorf("verify code=%d out=%q", code, out)
	}
	if code, _, _ := runVault(t, "", "verify", "--prune"); code != 0 {
		t.Fatal("prune")
	}
	s, _ = LoadStore(home)
	if len(s.Capabilities) != 1 || !s.Capabilities[0].Revoked {
		t.Errorf("dead capability not revoked: %+v", s.Capabilities)
	}
}

// TestVerifyTakesVaultLock pins that verify's check-and-repair waits
// for the vault lock: while another holder has it, verify must not
// complete; once released, it finishes normally.
func TestVerifyTakesVaultLock(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}

	release := make(chan struct{})
	held := make(chan struct{})
	go func() {
		_ = withVaultLock(os.Getenv(EnvHome), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	done := make(chan int, 1)
	go func() {
		code, _, _ := runVault(t, "", "verify", "--prune")
		done <- code
	}()

	select {
	case <-done:
		t.Fatal("verify completed while the vault lock was held")
	case <-time.After(250 * time.Millisecond):
		// Still blocked on the lock — expected.
	}
	close(release)
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("verify after lock release: code %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verify did not complete after the lock was released")
	}
}

func TestVerifyStaleTempFile(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	stale := filepath.Join(home, ".boru", ".vault.jsonic.987654.tmp")
	if err := os.WriteFile(stale, []byte("junk"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runVault(t, "", "verify")
	if code != 2 || !strings.Contains(out, "stale temp file") {
		t.Errorf("verify code=%d out=%q", code, out)
	}
	if code, _, _ := runVault(t, "", "verify", "--prune"); code != 0 {
		t.Fatal("prune")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale temp file not removed")
	}
}
