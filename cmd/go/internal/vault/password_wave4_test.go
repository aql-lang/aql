package vault

// password_wave4_test.go — negative / error-arm coverage for password.go:
// flag parse failures, corrupt-fixture stores, tampered slot material
// (PubMAC / public key), rekey phase edges, and direct unit calls into
// the slot-management helpers.

import (
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

// w4CorruptStore overwrites vault.jsonic with bytes that do not parse,
// so LoadStore fails with an error (not the nil,nil "no vault" case).
func w4CorruptStore(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(StorePath(home), []byte("{this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// w4EnvelopeVault builds the standard envelope fixture: admin
// ("test-pass"), a read-scoped "reader" ("reader-pass") on namespace
// proj, and one secret proj:k = "v". Leaves EnvPassphrase = test-pass.
func w4EnvelopeVault(t *testing.T) string {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")
	setPass(t, "test-pass")
	return home
}

// w4AdminSession opens an admin session directly (for integrity-key and
// direct-helper tests).
func w4AdminSession(t *testing.T, home string) (*Store, *Session) {
	t.Helper()
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	sess, err := openSession(s, home, "test-pass")
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return s, sess
}

func TestPasswordFlagParseErrors(t *testing.T) {
	w4EnvelopeVault(t)
	for _, verb := range []string{"list", "add", "rm", "assign", "set"} {
		if code, _, _ := runVault(t, "", "password", verb, "-definitely-not-a-flag"); code == 0 {
			t.Errorf("password %s with a bad flag should fail", verb)
		}
	}
}

func TestNamespacesLabelEmpty(t *testing.T) {
	if got := namespacesLabel(nil); got != "*" {
		t.Errorf("namespacesLabel(nil) = %q, want *", got)
	}
}

func TestPasswordAddErrorArms(t *testing.T) {
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, e := runVault(t, "", "password", "add", "x"); code == 0 || !strings.Contains(e, "parsing") {
			t.Errorf("add on corrupt store = %d, %q", code, e)
		}
	})
	t.Run("non-file backend", func(t *testing.T) {
		dir := t.TempDir()
		stubBin(t, dir, "secret-tool", "case \"$1\" in store) cat >/dev/null; exit 0;; lookup) exit 1;; clear) exit 0;; esac\nexit 0\n")
		prependPath(t, dir)
		testHome(t)
		if code, _, e := runVault(t, "", "init", "--backend=secret-service"); code != 0 {
			t.Fatalf("init secret-service via stub: %s", e)
		}
		if code, _, e := runVault(t, "", "password", "add", "x"); code == 0 ||
			!strings.Contains(e, "require the file backend") {
			t.Errorf("add on secret-service backend = %d, %q", code, e)
		}
	})
	t.Run("no passphrase source", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		t.Setenv(EnvPassphrase, "")
		var out, errb strings.Builder
		if code := runPassword([]string{"add", "x"}, home, nil, &out, &errb); code == 0 ||
			!strings.Contains(errb.String(), "passphrase") {
			t.Errorf("add without passphrase source = %d, %q", code, errb.String())
		}
	})
	t.Run("wrong passphrase envelope", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "totally-wrong")
		t.Setenv(EnvNewPassphrase, "npw")
		if code, _, e := runVault(t, "", "password", "add", "x"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("add with wrong passphrase = %d, %q", code, e)
		}
	})
	t.Run("no new passphrase source", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		t.Setenv(EnvNewPassphrase, "")
		var out, errb strings.Builder
		if code := runPassword([]string{"add", "x"}, home, nil, &out, &errb); code == 0 ||
			!strings.Contains(errb.String(), EnvNewPassphrase) {
			t.Errorf("add without new-pass source = %d, %q", code, errb.String())
		}
	})
	t.Run("tampered admin pubmac blocks new namespace", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := s.FindPasswordSlot("admin")
		s.Passwords[idx].PubMAC = strings.Repeat("00", 32)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvNewPassphrase, "w-pass")
		code, _, e := runVault(t, "", "password", "add", "--scope=read", "--namespaces=brandnew", "w2")
		if code == 0 || !strings.Contains(e, "integrity check") {
			t.Errorf("add with tampered admin PubMAC = %d, %q", code, e)
		}
	})
}

// TestPasswordAddMigratesOverGhostAlias: a legacy store alias with no
// keyring entry is skipped (not fatal) during migration.
func TestPasswordAddMigratesOverGhostAlias(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "real"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	s.Aliases = append(s.Aliases, Alias{Name: "ghost", CreatedAt: "2020-01-01T00:00:00Z"})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "real"); code != 0 || strings.TrimSpace(out) != "v" {
		t.Errorf("real secret survived migration = %d/%q", code, out)
	}
	if code, _, _ := runVault(t, "", "get", "--reveal", "ghost"); code == 0 {
		t.Error("ghost alias should still have no value after migration")
	}
}

// TestMigrateLegacyAndAddWrongPass drives the migration reader with a
// passphrase that cannot decrypt the legacy keyring.
func TestMigrateLegacyAndAddWrongPass(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	err = migrateLegacyAndAdd(s, home, "wrong-pass", "n", ScopeRead, []string{"*"}, "npw", "")
	if err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("migrateLegacyAndAdd with wrong pass = %v", err)
	}
}

func TestPasswordRmErrorArms(t *testing.T) {
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "password", "rm", "--yes", "x"); code == 0 {
			t.Error("rm on corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "password", "rm", "--yes", "reader"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("rm with wrong pass = %d, %q", code, e)
		}
	})
	t.Run("rekey missing slot", func(t *testing.T) {
		w4EnvelopeVault(t)
		if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "nosuch"); code == 0 ||
			!strings.Contains(e, "not found") {
			t.Errorf("rm --rekey missing slot = %d, %q", code, e)
		}
	})
	t.Run("rekey last admin", func(t *testing.T) {
		w4EnvelopeVault(t)
		if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "admin"); code == 0 ||
			!strings.Contains(e, "last admin") {
			t.Errorf("rm --rekey last admin = %d, %q", code, e)
		}
	})
}

// TestPasswordRmRekeyTamperedSurvivor: a surviving slot whose PubMAC was
// tampered aborts the rekey; a slot whose PublicKey is invalid but whose
// PubMAC was recomputed (integrity key in hand) is caught by the decode
// check.
func TestPasswordRmRekeyTamperedSurvivor(t *testing.T) {
	t.Run("bad pubmac", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		mustAddPassword(t, "test-pass", "r2-pass", "reader2", "--scope=read", "--namespaces=proj")
		setPass(t, "test-pass")
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := s.FindPasswordSlot("reader2")
		s.Passwords[idx].PubMAC = strings.Repeat("11", 32)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader"); code == 0 ||
			!strings.Contains(e, "integrity check") {
			t.Errorf("rekey over tampered survivor = %d, %q", code, e)
		}
	})
	t.Run("bad pubkey with recomputed mac", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		mustAddPassword(t, "test-pass", "r2-pass", "reader2", "--scope=read", "--namespaces=proj")
		setPass(t, "test-pass")
		_, sess := w4AdminSession(t, home)
		ik := sess.IntegrityKey()
		if ik == nil {
			t.Fatal("admin session has no integrity key")
		}
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := s.FindPasswordSlot("reader2")
		s.Passwords[idx].PublicKey = "%%%not-base64%%%"
		s.Passwords[idx].PubMAC = derivePubMAC(ik, "reader2", "%%%not-base64%%%",
			s.Passwords[idx].Scope, s.Passwords[idx].Namespaces)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader"); code == 0 ||
			!strings.Contains(e, "invalid public key") {
			t.Errorf("rekey over invalid-pubkey survivor = %d, %q", code, e)
		}
	})
}

// TestPasswordRmRekeyValueEdges: unaffected-namespace aliases are
// skipped, store-only ghosts are skipped, and a value that cannot be
// decrypted aborts the rekey with a re-encrypting error.
func TestPasswordRmRekeyValueEdges(t *testing.T) {
	home := w4EnvelopeVault(t)
	// rootkey lives outside the reader's namespace set.
	if code, _, e := runVault(t, "r\n", "add", "--from-stdin", "rootkey"); code != 0 {
		t.Fatalf("add rootkey: %s", e)
	}
	// proj:ghost exists only in the store (no keyring entry).
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	s.Aliases = append(s.Aliases, Alias{Name: "proj:ghost", Namespace: "proj", CreatedAt: "2020-01-01T00:00:00Z"})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	// First rekey pass succeeds over root (skip) + ghost (skip) + proj:k.
	if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader"); code != 0 {
		t.Fatalf("rekey with skip-edges: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || strings.TrimSpace(out) != "v" {
		t.Errorf("proj:k after rekey = %d/%q, want v", code, out)
	}

	// Now corrupt a value: sealed with a random key under the current id.
	mustAddPassword(t, "test-pass", "r3-pass", "reader3", "--scope=read", "--namespaces=proj")
	setPass(t, "test-pass")
	s, err = LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	idb, err := hex.DecodeString(s.CurrentNDK["proj"])
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealValue(mustRandom(t, 32), idb, "proj", "proj:k", "junk")
	if err != nil {
		t.Fatal(err)
	}
	kr := &envelopeFileKeyring{folder: vaultFolder(home)}
	if err := kr.Set("proj:k", sealed); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader3"); code == 0 ||
		!strings.Contains(e, "re-encrypting proj:k") {
		t.Errorf("rekey over undecryptable value = %d, %q", code, e)
	}
}

// TestPasswordRmRekeyKeyringUnreadable: a keyring file that cannot be
// loaded fails the rekey during phase B.
func TestPasswordRmRekeyKeyringUnreadable(t *testing.T) {
	home := w4EnvelopeVault(t)
	if err := os.WriteFile(vaultFolder(home)+"/"+vaultFileName("keyring"), []byte("garbage-no-magic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader"); code == 0 ||
		!strings.Contains(e, "keyring") {
		t.Errorf("rekey with unreadable keyring = %d, %q", code, e)
	}
}

// TestRekeyRemoveSlotDirect exercises the arms only reachable by direct
// call: a session without the integrity key, and a store that becomes
// unreadable after authentication.
func TestRekeyRemoveSlotDirect(t *testing.T) {
	t.Run("no integrity key", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		sess.ikey = nil
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil ||
			!strings.Contains(err.Error(), "integrity key unavailable") {
			t.Errorf("rekey without integrity key = %v", err)
		}
	})
	t.Run("store unreadable under lock", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		w4CorruptStore(t, home)
		if err := rekeyRemoveSlot(home, sess, "reader"); err == nil ||
			!strings.Contains(err.Error(), "parsing") {
			t.Errorf("rekey with corrupt store = %v", err)
		}
	})
}

func TestReadNewPassphraseEOFArms(t *testing.T) {
	t.Setenv(EnvNewPassphrase, "")
	if _, err := readNewPassphrase(strings.NewReader(""), io.Discard); err == nil {
		t.Error("EOF on first prompt should error")
	}
	if _, err := readNewPassphrase(strings.NewReader("a\n"), io.Discard); err == nil {
		t.Error("EOF on confirmation prompt should error")
	}
}

func TestBuildSlotMissingCurrentID(t *testing.T) {
	salt := mustRandom(t, keyringSaltLen)
	grant := map[string][]byte{"ns": mustRandom(t, ndkLen)}
	_, err := buildSlot("n", ScopeRead, []string{"ns"}, "pw", salt, grant, map[string]string{}, mustRandom(t, 32), "")
	if err == nil || !strings.Contains(err.Error(), "no current id") {
		t.Errorf("buildSlot without current id = %v", err)
	}
}

func TestSealWrappedMissingCurrentID(t *testing.T) {
	pub, _, err := newSlotKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sealWrapped(map[string][]byte{"ns": mustRandom(t, ndkLen)}, map[string]string{}, pub); err == nil ||
		!strings.Contains(err.Error(), "no current id") {
		t.Errorf("sealWrapped without current id = %v", err)
	}
}

func TestAddSlotToEnvelopeDirect(t *testing.T) {
	home := w4EnvelopeVault(t)
	fresh := func() *Store {
		s, err := LoadStore(home)
		if err != nil || s == nil {
			t.Fatalf("LoadStore: %v", err)
		}
		return s
	}
	if err := addSlotToEnvelope(fresh(), home, "wrong", "x", ScopeRead, []string{"*"}, "pw", "", false); err == nil {
		t.Error("wrong passphrase should fail")
	}
	if err := addSlotToEnvelope(fresh(), home, "reader-pass", "x", ScopeRead, []string{"*"}, "pw", "", false); err == nil ||
		!strings.Contains(err.Error(), "admin") {
		t.Errorf("non-admin add = %v", err)
	}
	if err := addSlotToEnvelope(fresh(), home, "test-pass", "reader", ScopeRead, []string{"*"}, "pw", "", false); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate add = %v", err)
	}
	// A store whose integrity NDK id is gone yields a session without the
	// integrity key.
	s := fresh()
	delete(s.CurrentNDK, integrityNamespace)
	if err := addSlotToEnvelope(s, home, "test-pass", "x", ScopeRead, []string{"*"}, "pw", "", false); err == nil ||
		!strings.Contains(err.Error(), "integrity key unavailable") {
		t.Errorf("missing integrity NDK = %v", err)
	}
	// A namespace whose current NDK id the admin session cannot resolve.
	s = fresh()
	s.CurrentNDK["ghostns"] = "00112233aabbccdd"
	if err := addSlotToEnvelope(s, home, "test-pass", "x", ScopeAdmin, []string{"*"}, "pw", "", false); err == nil ||
		!strings.Contains(err.Error(), "lacks the data key") {
		t.Errorf("ghost namespace id = %v", err)
	}
}

func TestResolveGrantDirect(t *testing.T) {
	home := w4EnvelopeVault(t)
	_, sess := w4AdminSession(t, home)

	t.Run("no integrity key", func(t *testing.T) {
		s2, sess2 := w4AdminSession(t, home)
		sess2.ikey = nil
		if _, err := resolveGrant(s2, sess2, []string{"proj"}); err == nil ||
			!strings.Contains(err.Error(), "integrity key unavailable") {
			t.Errorf("resolveGrant without ik = %v", err)
		}
	})
	t.Run("session lacks key", func(t *testing.T) {
		st, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		st.CurrentNDK["ghostns"] = "aabbccdd00112233"
		if _, err := resolveGrant(st, sess, []string{"ghostns"}); err == nil ||
			!strings.Contains(err.Error(), "lacks the data key") {
			t.Errorf("resolveGrant ghost id = %v", err)
		}
	})
	t.Run("nil CurrentNDK is initialized", func(t *testing.T) {
		st, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		st.CurrentNDK = nil
		grant, err := resolveGrant(st, sess, []string{"brandnew"})
		if err != nil {
			t.Fatalf("resolveGrant new ns: %v", err)
		}
		if st.CurrentNDK["brandnew"] == "" || grant["brandnew"] == nil {
			t.Error("brand-new namespace NDK not minted")
		}
	})
	t.Run("tampered admin blocks minting", func(t *testing.T) {
		st, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := st.FindPasswordSlot("admin")
		st.Passwords[idx].PubMAC = strings.Repeat("22", 32)
		if _, err := resolveGrant(st, sess, []string{"brandnew2"}); err == nil ||
			!strings.Contains(err.Error(), "integrity check") {
			t.Errorf("resolveGrant with tampered admin = %v", err)
		}
	})
}

func TestSealNDKToAdminsDirect(t *testing.T) {
	ik := mustRandom(t, 32)
	ndk := mustRandom(t, ndkLen)
	t.Run("invalid pubkey with recomputed mac", func(t *testing.T) {
		slot := PasswordSlot{Name: "a", Scope: ScopeAdmin, PublicKey: "%%%bad%%%"}
		slot.PubMAC = derivePubMAC(ik, "a", "%%%bad%%%", ScopeAdmin, nil)
		st := &Store{Passwords: []PasswordSlot{slot}}
		if err := sealNDKToAdmins(st, "ns", "0011223344556677", ndk, ik); err == nil ||
			!strings.Contains(err.Error(), "invalid public key") {
			t.Errorf("sealNDKToAdmins invalid pub = %v", err)
		}
	})
	t.Run("nil WrappedKeys initialized", func(t *testing.T) {
		pub, _, err := newSlotKeypair()
		if err != nil {
			t.Fatal(err)
		}
		pubB64 := base64.StdEncoding.EncodeToString(pub[:])
		slot := PasswordSlot{Name: "a", Scope: ScopeAdmin, PublicKey: pubB64}
		slot.PubMAC = derivePubMAC(ik, "a", pubB64, ScopeAdmin, nil)
		st := &Store{Passwords: []PasswordSlot{slot}}
		if err := sealNDKToAdmins(st, "ns", "0011223344556677", ndk, ik); err != nil {
			t.Fatalf("sealNDKToAdmins: %v", err)
		}
		if st.Passwords[0].WrappedKeys["ns#0011223344556677"] == "" {
			t.Error("wrap not recorded on admin slot")
		}
	})
}

func TestPasswordAssignErrorArms(t *testing.T) {
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "x"); code == 0 {
			t.Error("assign on corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "reader"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("assign with wrong pass = %d, %q", code, e)
		}
	})
	t.Run("tampered target pubmac", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := s.FindPasswordSlot("reader")
		s.Passwords[idx].PubMAC = strings.Repeat("33", 32)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "reader"); code == 0 ||
			!strings.Contains(e, "integrity check") {
			t.Errorf("assign to tampered slot = %d, %q", code, e)
		}
	})
	t.Run("invalid target pubkey", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		_, sess := w4AdminSession(t, home)
		ik := sess.IntegrityKey()
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		_, idx := s.FindPasswordSlot("reader")
		s.Passwords[idx].PublicKey = "%%%bad%%%"
		s.Passwords[idx].PubMAC = derivePubMAC(ik, "reader", "%%%bad%%%",
			s.Passwords[idx].Scope, s.Passwords[idx].Namespaces)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "reader"); code == 0 ||
			!strings.Contains(e, "invalid slot public key") {
			t.Errorf("assign to invalid-pubkey slot = %d, %q", code, e)
		}
	})
	t.Run("resolveGrant failure surfaces", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.CurrentNDK["ghostns"] = "8899aabbccddeeff"
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=ghostns", "reader"); code == 0 ||
			!strings.Contains(e, "lacks the data key") {
			t.Errorf("assign granting ghost ns = %d, %q", code, e)
		}
	})
}

func TestPasswordSetErrorArms(t *testing.T) {
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "password", "set", "x"); code == 0 {
			t.Error("set on corrupt store should fail")
		}
	})
	t.Run("no current passphrase source", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		t.Setenv(EnvPassphrase, "")
		var out, errb strings.Builder
		if code := runPassword([]string{"set", "reader"}, home, nil, &out, &errb); code == 0 ||
			!strings.Contains(errb.String(), "passphrase") {
			t.Errorf("set without pass source = %d, %q", code, errb.String())
		}
	})
	t.Run("no new passphrase source", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		setPass(t, "reader-pass")
		t.Setenv(EnvNewPassphrase, "")
		var out, errb strings.Builder
		if code := runPassword([]string{"set", "reader"}, home, nil, &out, &errb); code == 0 ||
			!strings.Contains(errb.String(), EnvNewPassphrase) {
			t.Errorf("set without new-pass source = %d, %q", code, errb.String())
		}
	})
}
