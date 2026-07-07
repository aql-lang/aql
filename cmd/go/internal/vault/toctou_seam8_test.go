package vault

// toctou_seam8_test.go — wave-8 coverage for the under-lock re-check guards
// that defend a mutate against a writer racing in after the command's
// pre-check. Each test lands the racing change (a lock, or a colliding
// destination alias) from inside an intermediate seam that runs after the
// pre-check but before the locked mutate reloads the store, so the guard fires
// deterministically without a second process.

import (
	"testing"
)

// TestW8_AddLockRaceUnderMutate drives runAdd's under-lock locked-store guard:
// the vault is unlocked at the pre-check, then a "concurrent" lock lands during
// the keyring write, so the reloaded store is locked.
func TestW8_AddLockRaceUnderMutate(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	old := w8writeSecret
	t.Cleanup(func() { w8writeSecret = old })
	w8writeSecret = func(hd string, sess *Session, alias, ns, value string) error {
		st, err := LoadStore(home)
		if err == nil && st != nil {
			st.Locked = true
			_ = SaveStore(home, st)
		}
		return old(hd, sess, alias, ns, value)
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 1 {
		t.Errorf("add racing a lock should refuse under the mutate (code 1), got %d", code)
	}
}

// TestW8_GrantLockRaceUnderMutate drives runGrant's under-lock locked-store
// guard: a lock lands during alias resolution, between the pre-check and the
// locked mutate.
func TestW8_GrantLockRaceUnderMutate(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := w8findAliasRef
	t.Cleanup(func() { w8findAliasRef = old })
	w8findAliasRef = func(s *Store, ref string) (*Alias, string, error) {
		st, err := LoadStore(home)
		if err == nil && st != nil {
			st.Locked = true
			_ = SaveStore(home, st)
		}
		return old(s, ref)
	}
	if code, _, _ := runVault(t, "", "grant", "--agent=ci", "k"); code != 1 {
		t.Errorf("grant racing a lock should refuse under the mutate (code 1), got %d", code)
	}
}

// TestW8_MvDestinationCollisionUnderMutate drives runMv's under-lock
// destination-collision guard: the destination alias does not exist at the
// pre-check, then a "concurrent" writer creates it during the copy phase, so
// the locked rename aborts.
func TestW8_MvDestinationCollisionUnderMutate(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := c7sessGetValue
	t.Cleanup(func() { c7sessGetValue = old })
	landed := false
	c7sessGetValue = func(sess *Session, alias, ns string) (string, error) {
		if !landed {
			landed = true
			st, err := LoadStore(home)
			if err == nil && st != nil {
				st.UpsertAlias(Alias{Name: "k2"})
				_ = SaveStore(home, st)
			}
		}
		return old(sess, alias, ns)
	}
	if code, _, _ := runVault(t, "", "mv", "k", "k2"); code != 1 {
		t.Errorf("mv racing a destination collision should abort under the lock (code 1), got %d", code)
	}
}
