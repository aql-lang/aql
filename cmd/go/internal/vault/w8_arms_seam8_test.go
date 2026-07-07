package vault

// w8_arms_seam8_test.go — wave-8 coverage for the defensive arms wired through
// the new w8_seam8.go seams: exec's read-scope gate, WriteSecret's two write
// steps, and the "alias resolved but gone from the locked reload" guards in
// expiry/grant/rotate.

import (
	"io"
	"strings"
	"testing"
)

// TestW8_ExecRequireScopeDenied drives exec's read-scope gate error arm via the
// w8requireScope seam (every real session tier satisfies OpRead).
func TestW8_ExecRequireScopeDenied(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := w8requireScope
	t.Cleanup(func() { w8requireScope = old })
	w8requireScope = func(*Session, Op) error { return errW8Boom }
	if code, _, _ := runVault(t, "", "exec", "k", "--", "sh", "-c", "true"); code != 1 {
		t.Errorf("exec with a denied read scope should return 1, got %d", code)
	}
}

// TestW8_WriteSecretWriteAndMutateErrors drives WriteSecret's keyring-write and
// store-mutate error arms via the w8writeSecret / w8mutateStore seams.
func TestW8_WriteSecretWriteAndMutateErrors(t *testing.T) {
	t.Run("write-error", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		old := w8writeSecret
		t.Cleanup(func() { w8writeSecret = old })
		w8writeSecret = func(string, *Session, string, string, string) error { return errW8Boom }
		if err := WriteSecret(home, "k", "v", "src", strings.NewReader(""), io.Discard); err == nil {
			t.Error("WriteSecret should surface the keyring-write error")
		}
	})
	t.Run("mutate-error", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		old := w8mutateStore
		t.Cleanup(func() { w8mutateStore = old })
		w8mutateStore = func(string, func(*Store) error) error { return errW8Boom }
		if err := WriteSecret(home, "k", "v", "src", strings.NewReader(""), io.Discard); err == nil {
			t.Error("WriteSecret should surface the store-mutate error")
		}
	})
}

// TestW8_ResolvedAliasVanishesBeforeMutate drives the "alias resolved but absent
// from the freshly-reloaded store" guard in expiry set/clear, grant, and rotate,
// via the w8findAliasRef seam handing back a name the store never had.
func TestW8_ResolvedAliasVanishesBeforeMutate(t *testing.T) {
	ghost := func(*Store, string) (*Alias, string, error) { return nil, "w8-ghost", nil }
	swap := func(t *testing.T) {
		old := w8findAliasRef
		w8findAliasRef = ghost
		t.Cleanup(func() { w8findAliasRef = old })
	}
	seed := func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed add: %s", e)
		}
	}

	t.Run("expiry-set", func(t *testing.T) {
		seed(t)
		swap(t)
		if code, _, _ := runVault(t, "", "expiry", "set", "k", "2030-01-01"); code != 1 {
			t.Errorf("expiry set on a vanished alias should return 1, got %d", code)
		}
	})
	t.Run("expiry-clear", func(t *testing.T) {
		seed(t)
		swap(t)
		if code, _, _ := runVault(t, "", "expiry", "clear", "k"); code != 1 {
			t.Errorf("expiry clear on a vanished alias should return 1, got %d", code)
		}
	})
	t.Run("grant", func(t *testing.T) {
		seed(t)
		swap(t)
		if code, _, _ := runVault(t, "", "grant", "--agent=ci", "k"); code != 1 {
			t.Errorf("grant on a vanished alias should return 1, got %d", code)
		}
	})
	t.Run("rotate", func(t *testing.T) {
		seed(t)
		swap(t)
		if code, _, _ := runVault(t, "v2\n", "rotate", "--from-stdin", "--yes", "k"); code != 1 {
			t.Errorf("rotate on a vanished alias should return 1, got %d", code)
		}
	})
}
