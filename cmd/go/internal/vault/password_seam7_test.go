package vault

// password_seam7_test.go — seam7 coverage for password.go's crypto/entropy/
// KDF/persistence error arms. These live deep inside buildSlot,
// migrateLegacyAndAdd, addSlotToEnvelope, resolveGrant, sealWrapped,
// rekeyRemoveSlot, sealNDKToAdmins, and the under-lock reload closures of
// add/assign/set. They are reached by direct helper calls (and a few
// runPassword* dispatches) with the randRead / p7boxRand / p7scryptKey /
// p7ioReadFull / p7loadStore / osRename seams swapped (restored via
// t.Cleanup, no t.Parallel).

import (
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"
)

// --- extra seam-swap + counting helpers ------------------------------------

func p7swapLoadStore(t *testing.T, fn func(string) (*Store, error)) {
	t.Helper()
	old := p7loadStore
	p7loadStore = fn
	t.Cleanup(func() { p7loadStore = old })
}

func p7swapOsRename(t *testing.T, fn func(string, string) error) {
	t.Helper()
	old := osRename
	osRename = fn
	t.Cleanup(func() { osRename = old })
}

// p7renameFailSuffix fails osRename when the destination path ends with
// suffix (so store writes and keyring writes can be failed independently),
// delegating to the real rename otherwise.
func p7renameFailSuffix(suffix string) func(string, string) error {
	return func(oldpath, newpath string) error {
		if strings.HasSuffix(newpath, suffix) {
			return p7boom()
		}
		return os.Rename(oldpath, newpath)
	}
}

// p7countScrypt returns a p7scryptKey seam that runs the real derivation for
// the first ok calls and then fails, so an earlier authentication succeeds
// while a specific later derivation fails.
func p7countScrypt(ok int) func(string, []byte) ([]byte, error) {
	n := 0
	return func(pass string, salt []byte) ([]byte, error) {
		n++
		if n > ok {
			return nil, p7boom()
		}
		return scryptKey(pass, salt)
	}
}

// p7countReadFull returns a p7ioReadFull seam that runs the real read for
// the first ok calls and then fails.
func p7countReadFull(ok int) func(io.Reader, []byte) (int, error) {
	n := 0
	return func(r io.Reader, b []byte) (int, error) {
		n++
		if n > ok {
			return 0, p7boom()
		}
		return io.ReadFull(r, b)
	}
}

// p7storeWithout loads the real store and drops the named slot, so a
// swapped p7loadStore can hand a closure a store that no longer contains it.
func p7storeWithout(t *testing.T, home, name string) *Store {
	t.Helper()
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	kept := make([]PasswordSlot, 0, len(s.Passwords))
	for _, p := range s.Passwords {
		if p.Name != name {
			kept = append(kept, p)
		}
	}
	s.Passwords = kept
	return s
}

// p7buildSlotArgs returns valid, minimal arguments for a direct buildSlot
// call whose only failure is the swapped seam.
func p7buildSlotArgs(t *testing.T) (salt []byte, grant map[string][]byte, cur map[string]string, ik []byte) {
	t.Helper()
	return mustRandom(t, keyringSaltLen),
		map[string][]byte{"ns": mustRandom(t, ndkLen)},
		map[string]string{"ns": "aabbccddaabbccdd"},
		mustRandom(t, 32)
}

// --- buildSlot arms --------------------------------------------------------

func TestP7_BuildSlotArms(t *testing.T) {
	t.Run("master KEK error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapScryptKey(t, p7failScrypt)
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a master-KEK failure")
		}
	})
	t.Run("salt entropy error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapRandRead(t, p7failRead)
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a slot-salt entropy failure")
		}
	})
	t.Run("slot KEK error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapIOReadFull(t, func(io.Reader, []byte) (int, error) { return 0, p7boom() })
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a slot-KEK failure")
		}
	})
	t.Run("keypair error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapBoxRand(t, p7failReader{})
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a keypair-generation failure")
		}
	})
	t.Run("seal priv key error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapRandRead(t, p7countRead(1)) // slot salt ok, priv-key nonce fails
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a sealPrivKey nonce failure")
		}
	})
	t.Run("seal NDK error", func(t *testing.T) {
		salt, grant, cur, ik := p7buildSlotArgs(t)
		p7swapBoxRand(t, &p7byteReader{left: 32}) // keypair ok, sealNDK fails
		if _, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, cur, ik, ""); err == nil {
			t.Fatal("buildSlot should surface a sealNDK failure")
		}
	})
}

// --- migrateLegacyAndAdd arms ----------------------------------------------

// p7legacyVault initialises a legacy (no-slots) file vault and optionally
// seeds one secret. Returns home and the loaded store.
func p7legacyVault(t *testing.T, secret string) (string, *Store) {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if secret != "" {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", secret); code != 0 {
			t.Fatalf("seed %q: %s", secret, e)
		}
	}
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	return home, s
}

func TestP7_MigrateLegacyAndAddArms(t *testing.T) {
	star := []string{"*"}
	t.Run("vault salt entropy", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapRandRead(t, p7countRead(0))
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface a vault-salt entropy failure")
		}
	})
	t.Run("integrity NDK", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapRandRead(t, p7countRead(1)) // vsalt ok, integrity newNDK fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the integrity newNDK failure")
		}
	})
	t.Run("integrity NDK id", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapRandRead(t, p7countRead(2)) // vsalt, newNDK ok; newNDKID fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the integrity newNDKID failure")
		}
	})
	t.Run("root NDK", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapRandRead(t, p7countRead(3)) // integrity done; root newNDK fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the root newNDK failure")
		}
	})
	t.Run("entry NDK", func(t *testing.T) {
		home, s := p7legacyVault(t, "proj:k")
		p7swapRandRead(t, p7countRead(5)) // integrity+root done; entry proj newNDK fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the entry-namespace newNDK failure")
		}
	})
	t.Run("requested-namespace NDK", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapRandRead(t, p7countRead(5)) // integrity+root done; newNS "extra" newNDK fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, []string{"extra"}, "np", ""); err == nil {
			t.Fatal("migrate should surface the requested-namespace newNDK failure")
		}
	})
	t.Run("admin slot build", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapBoxRand(t, p7failReader{}) // buildSlot("admin") keypair fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the admin-slot build failure")
		}
	})
	t.Run("requested slot build", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapScryptKey(t, p7countScrypt(1)) // admin buildSlot ok, req buildSlot master-KEK fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface the requested-slot build failure")
		}
	})
	t.Run("store save", func(t *testing.T) {
		home, s := p7legacyVault(t, "")
		p7swapOsRename(t, p7renameFailSuffix("vault.jsonic"))
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface a store-save failure")
		}
	})
	t.Run("reseal value", func(t *testing.T) {
		home, s := p7legacyVault(t, "k")
		p7swapRandRead(t, p7countRead(9)) // through both buildSlots; phase-5 sealValue nonce fails
		if err := migrateLegacyAndAdd(s, home, "test-pass", "n", ScopeRead, star, "np", ""); err == nil {
			t.Fatal("migrate should surface a value re-seal failure")
		}
	})
}

// --- addSlotToEnvelope arms ------------------------------------------------

func TestP7_AddSlotToEnvelopeArms(t *testing.T) {
	fresh := func(t *testing.T, home string) *Store {
		s, err := LoadStore(home)
		if err != nil || s == nil {
			t.Fatalf("LoadStore: %v", err)
		}
		return s
	}
	t.Run("new namespace NDK", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		p7swapRandRead(t, p7failRead)
		if err := addSlotToEnvelope(fresh(t, home), home, "test-pass", "x", ScopeRead, []string{"brandnew"}, "pw", "", false); err == nil {
			t.Fatal("addSlotToEnvelope should surface a new-namespace newNDK failure")
		}
	})
	t.Run("new namespace NDK id", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		p7swapRandRead(t, p7countRead(1)) // newNDK ok, newNDKID fails
		if err := addSlotToEnvelope(fresh(t, home), home, "test-pass", "x", ScopeRead, []string{"brandnew"}, "pw", "", false); err == nil {
			t.Fatal("addSlotToEnvelope should surface a new-namespace newNDKID failure")
		}
	})
	t.Run("slot build", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		p7swapBoxRand(t, p7failReader{})
		if err := addSlotToEnvelope(fresh(t, home), home, "test-pass", "x", ScopeRead, []string{"proj"}, "pw", "", false); err == nil {
			t.Fatal("addSlotToEnvelope should surface a buildSlot failure")
		}
	})
}

// --- sealWrapped / resolveGrant / sealNDKToAdmins arms ---------------------

func TestP7_SealWrappedBoxFailure(t *testing.T) {
	pub, _, err := newSlotKeypair()
	if err != nil {
		t.Fatal(err)
	}
	p7swapBoxRand(t, p7failReader{})
	grant := map[string][]byte{"ns": mustRandom(t, ndkLen)}
	if _, err := sealWrapped(grant, map[string]string{"ns": "aabbccddaabbccdd"}, pub); err == nil {
		t.Fatal("sealWrapped should surface a sealNDK failure")
	}
}

func TestP7_ResolveGrantEntropyArms(t *testing.T) {
	t.Run("new namespace NDK", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		st, sess := w4AdminSession(t, home)
		p7swapRandRead(t, p7failRead)
		if _, err := resolveGrant(st, sess, []string{"brandnew"}); err == nil {
			t.Fatal("resolveGrant should surface a new-namespace newNDK failure")
		}
	})
	t.Run("new namespace NDK id", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		st, sess := w4AdminSession(t, home)
		p7swapRandRead(t, p7countRead(1)) // newNDK ok, newNDKID fails
		if _, err := resolveGrant(st, sess, []string{"brandnew"}); err == nil {
			t.Fatal("resolveGrant should surface a new-namespace newNDKID failure")
		}
	})
}

func TestP7_SealNDKToAdminsBoxFailure(t *testing.T) {
	ik := mustRandom(t, 32)
	ndk := mustRandom(t, ndkLen)
	pub, _, err := newSlotKeypair()
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])
	slot := PasswordSlot{Name: "a", Scope: ScopeAdmin, PublicKey: pubB64}
	slot.PubMAC = derivePubMAC(ik, "a", pubB64, ScopeAdmin, nil)
	st := &Store{Passwords: []PasswordSlot{slot}}
	p7swapBoxRand(t, p7failReader{})
	if err := sealNDKToAdmins(st, "ns", "0011223344556677", ndk, ik); err == nil {
		t.Fatal("sealNDKToAdmins should surface a sealNDK failure")
	}
}

// --- rekeyRemoveSlot arms --------------------------------------------------

func TestP7_RekeyRemoveSlotArms(t *testing.T) {
	t.Run("phase A newNDK", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapRandRead(t, p7failRead)
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-A newNDK failure")
		}
	})
	t.Run("phase A newNDKID", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapRandRead(t, p7countRead(1))
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-A newNDKID failure")
		}
	})
	t.Run("phase A sealNDK", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapBoxRand(t, p7failReader{})
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-A sealNDK failure")
		}
	})
	t.Run("phase A store save", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapOsRename(t, p7renameFailSuffix("vault.jsonic"))
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-A store-save failure")
		}
	})
	t.Run("phase B reseal value", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapRandRead(t, p7countRead(2)) // phase A newNDK+newNDKID ok; phase B sealValue nonce fails
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-B value re-seal failure")
		}
	})
	t.Run("phase B keyring set", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		p7swapOsRename(t, p7renameFailSuffix("vault.keyring"))
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil {
			t.Fatal("rekey should surface a phase-B keyring-write failure")
		}
	})
}

// --- runPasswordAdd / Assign / Set under-lock reload arms ------------------

func p7discard() (io.Reader, *strings.Builder, *strings.Builder) {
	return strings.NewReader(""), &strings.Builder{}, &strings.Builder{}
}

func TestP7_PasswordAddReloadArms(t *testing.T) {
	t.Run("reload error", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		t.Setenv(EnvNewPassphrase, "np")
		p7swapLoadStore(t, func(string) (*Store, error) { return nil, p7boom() })
		in, out, errb := p7discard()
		if code := runPasswordAdd([]string{"newp"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("add reload error = %d, %q", code, errb.String())
		}
	})
	t.Run("reload nil store", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		t.Setenv(EnvNewPassphrase, "np")
		p7swapLoadStore(t, func(string) (*Store, error) { return nil, nil })
		in, out, errb := p7discard()
		if code := runPasswordAdd([]string{"newp"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "not initialized") {
			t.Fatalf("add reload nil = %d, %q", code, errb.String())
		}
	})
}

func TestP7_PasswordAssignReloadArms(t *testing.T) {
	t.Run("reload error", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		p7swapLoadStore(t, func(string) (*Store, error) { return nil, p7boom() })
		in, out, errb := p7discard()
		if code := runPasswordAssign([]string{"--yes", "--namespaces=proj", "reader"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("assign reload error = %d, %q", code, errb.String())
		}
	})
	t.Run("slot vanished", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		without := p7storeWithout(t, home, "reader")
		p7swapLoadStore(t, func(string) (*Store, error) { return without, nil })
		in, out, errb := p7discard()
		if code := runPasswordAssign([]string{"--yes", "--namespaces=proj", "reader"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "not found") {
			t.Fatalf("assign slot vanished = %d, %q", code, errb.String())
		}
	})
	t.Run("seal wrapped failure", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		p7swapBoxRand(t, p7failReader{})
		in, out, errb := p7discard()
		if code := runPasswordAssign([]string{"--yes", "--namespaces=proj", "reader"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("assign seal-wrapped failure = %d, %q", code, errb.String())
		}
	})
}

func TestP7_PasswordSetReloadArms(t *testing.T) {
	setup := func(t *testing.T) string {
		home := w4EnvelopeVault(t)
		t.Setenv(EnvPassphrase, "test-pass")
		t.Setenv(EnvNewPassphrase, "np")
		return home
	}
	t.Run("reload error", func(t *testing.T) {
		home := setup(t)
		p7swapLoadStore(t, func(string) (*Store, error) { return nil, p7boom() })
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("set reload error = %d, %q", code, errb.String())
		}
	})
	t.Run("slot vanished", func(t *testing.T) {
		home := setup(t)
		without := p7storeWithout(t, home, "admin")
		p7swapLoadStore(t, func(string) (*Store, error) { return without, nil })
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "not found") {
			t.Fatalf("set slot vanished = %d, %q", code, errb.String())
		}
	})
	t.Run("master KEK error", func(t *testing.T) {
		home := setup(t)
		p7swapScryptKey(t, p7countScrypt(1)) // openSession ok, closure master-KEK fails
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("set master-KEK error = %d, %q", code, errb.String())
		}
	})
	t.Run("new salt entropy", func(t *testing.T) {
		home := setup(t)
		p7swapRandRead(t, p7failRead)
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("set new-salt entropy = %d, %q", code, errb.String())
		}
	})
	t.Run("slot KEK error", func(t *testing.T) {
		home := setup(t)
		p7swapIOReadFull(t, p7countReadFull(1)) // openSession slotKEK ok, closure slotKEK fails
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("set slot-KEK error = %d, %q", code, errb.String())
		}
	})
	t.Run("seal priv key error", func(t *testing.T) {
		home := setup(t)
		p7swapRandRead(t, p7countRead(1)) // new salt ok, sealPrivKey nonce fails
		in, out, errb := p7discard()
		if code := runPasswordSet([]string{"admin"}, home, in, out, errb); code == 0 ||
			!strings.Contains(errb.String(), "p7 seam boom") {
			t.Fatalf("set seal-priv-key error = %d, %q", code, errb.String())
		}
	})
}
