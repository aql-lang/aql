package vault

import (
	"os"
	"strings"
	"testing"
)

func journalHome(t *testing.T) string {
	t.Helper()
	t.Setenv(EnvSuffix, "")
	t.Setenv(EnvFolder, "")
	return t.TempDir()
}

func TestJournalAppendReadHead(t *testing.T) {
	dir := journalHome(t)
	ik := mustRandom(t, 32)
	if err := appendJournal(dir, ik, 1, "user:a", "vault.add", "sha1", &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	if err := appendJournal(dir, ik, 2, "user:a", "vault.rm", "sha2", &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	recs, err := readJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("read %d records, want 2", len(recs))
	}
	head, _ := journalHead(dir)
	if head == nil || head.Generation != 2 || head.Action != "vault.rm" {
		t.Errorf("head = %+v, want generation 2 / vault.rm", head)
	}
	if g, _ := findJournalGeneration(dir, 1); g == nil || g.SHA256 != "sha1" {
		t.Errorf("findJournalGeneration(1) = %+v", g)
	}
}

func TestJournalChainVerifies(t *testing.T) {
	dir := journalHome(t)
	ik := mustRandom(t, 32)
	for i := int64(1); i <= 4; i++ {
		if err := appendJournal(dir, ik, i, "u", "vault.add", "sha", &Store{Version: storeVersion}); err != nil {
			t.Fatal(err)
		}
	}
	ok, bad, err := verifyJournalChain(dir, ik)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("chain should verify, broke at gen %d", bad)
	}
	// A record appended WITHOUT the key breaks the chain there.
	if err := appendJournal(dir, nil, 5, "u", "vault.add", "sha", &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	ok, bad, _ = verifyJournalChain(dir, ik)
	if ok || bad != 5 {
		t.Errorf("keyless record: ok=%v bad=%d, want ok=false bad=5", ok, bad)
	}
}

func TestJournalTamperDetected(t *testing.T) {
	dir := journalHome(t)
	ik := mustRandom(t, 32)
	for i := int64(1); i <= 3; i++ {
		if err := appendJournal(dir, ik, i, "u", "vault.add", "sha-orig", &Store{Version: storeVersion}); err != nil {
			t.Fatal(err)
		}
	}
	// Tamper the middle record's recorded sha by rewriting the file.
	raw, _ := os.ReadFile(journalPath(dir))
	tampered := strings.Replace(string(raw), `"sha256":"sha-orig"`, `"sha256":"sha-evil"`, 1)
	if tampered == string(raw) {
		t.Fatal("test setup: no replacement made")
	}
	if err := os.WriteFile(journalPath(dir), []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := verifyJournalChain(dir, ik); ok {
		t.Error("chain should not verify after tampering a record's sha256")
	}
}

func TestJournalPruneKeepsNewest(t *testing.T) {
	dir := journalHome(t)
	ik := mustRandom(t, 32)
	for i := int64(1); i <= 6; i++ {
		if err := appendJournal(dir, ik, i, "u", "vault.add", "sha", &Store{Version: storeVersion}); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneJournal(dir, 2); err != nil {
		t.Fatal(err)
	}
	recs, _ := readJournal(dir)
	if len(recs) != 2 || recs[0].Generation != 5 || recs[1].Generation != 6 {
		t.Errorf("after prune got %d records (gens %v), want gens [5 6]", len(recs), genList(recs))
	}
}

func genList(recs []journalRecord) []int64 {
	out := make([]int64, len(recs))
	for i, r := range recs {
		out[i] = r.Generation
	}
	return out
}

func TestRedactStoreStripsSecrets(t *testing.T) {
	s := &Store{
		Version:   storeVersion,
		VaultSalt: "vaultsalt",
		Config:    map[string]any{"token": "leaked"},
		Passwords: []PasswordSlot{{
			Name: "reader", Scope: ScopeRead, Namespaces: []string{"proj"},
			PublicKey: "pub", Salt: "s", Verifier: "v", PubMAC: "p", EncPrivKey: "e",
			WrappedKeys: map[string]string{"proj#aa": "w"}, CreatedAt: "t",
		}},
	}
	r := redactStore(s)

	if r.Config != nil {
		t.Error("redacted store still carries Config")
	}
	if r.VaultSalt != "" {
		t.Error("redacted store still carries VaultSalt")
	}
	p := r.Passwords[0]
	if p.Salt != "" || p.Verifier != "" || p.PubMAC != "" || p.EncPrivKey != "" || p.WrappedKeys != nil {
		t.Errorf("redacted slot still carries crypto material: %+v", p)
	}
	// Non-secret metadata preserved for history/restore display.
	if p.Name != "reader" || p.Scope != ScopeRead || p.PublicKey != "pub" || len(p.Namespaces) != 1 {
		t.Errorf("redaction dropped non-secret metadata: %+v", p)
	}
	// Original must be untouched.
	if s.Passwords[0].Salt != "s" || s.Config == nil || s.VaultSalt != "vaultsalt" {
		t.Error("redactStore mutated the original store")
	}
}
