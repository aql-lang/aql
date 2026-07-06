package vault

// journal_more_test.go — journal retention config, capability merge on
// restore, quarantine pruning, and the history/restore CLI edges.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalKeepFromConfig(t *testing.T) {
	cases := []struct {
		name string
		s    *Store
		want int
	}{
		{"nil store", nil, defaultJournalKeep},
		{"nil config", &Store{}, defaultJournalKeep},
		{"unset", &Store{Config: map[string]any{}}, defaultJournalKeep},
		{"float", &Store{Config: map[string]any{"journal.keep": float64(7)}}, 7},
		{"float zero", &Store{Config: map[string]any{"journal.keep": float64(0)}}, defaultJournalKeep},
		{"int", &Store{Config: map[string]any{"journal.keep": 9}}, 9},
		{"int negative", &Store{Config: map[string]any{"journal.keep": -1}}, defaultJournalKeep},
		{"string", &Store{Config: map[string]any{"journal.keep": "12"}}, 12},
		{"string junk", &Store{Config: map[string]any{"journal.keep": "junk"}}, defaultJournalKeep},
		{"string zero", &Store{Config: map[string]any{"journal.keep": "0"}}, defaultJournalKeep},
	}
	for _, tc := range cases {
		if got := journalKeepFromConfig(tc.s); got != tc.want {
			t.Errorf("%s: journalKeepFromConfig = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestMergeCapabilitiesMonotonic(t *testing.T) {
	target := []Capability{
		{ID: "a", UsedCalls: 1, UsedCostCents: 10},
		{ID: "b"},
	}
	cur := []Capability{
		{ID: "a", Revoked: true, UsedCalls: 5, UsedCostCents: 3},
		{ID: "c", Revoked: true}, // added after target, revoked since
		{ID: "d"},                // added after target, still live -> dropped
	}
	got := mergeCapabilities(cur, target)
	byID := map[string]Capability{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if len(got) != 3 {
		t.Fatalf("merged %d caps, want 3 (a, b, c): %+v", len(got), got)
	}
	a := byID["a"]
	if !a.Revoked {
		t.Error("a revoked now must stay revoked after restore")
	}
	if a.UsedCalls != 5 || a.UsedCostCents != 10 {
		t.Errorf("meters must never roll backward: %+v", a)
	}
	if _, ok := byID["b"]; !ok {
		t.Error("b from the target generation should be reinstated")
	}
	if c := byID["c"]; !c.Revoked {
		t.Error("c (post-target, revoked) must be kept revoked, not resurrected")
	}
	if _, ok := byID["d"]; ok {
		t.Error("d (post-target, live) must not survive the merge")
	}
}

func TestQuarantinePruneKeepsNewest(t *testing.T) {
	home := journalHome(t)
	if err := os.MkdirAll(vaultFolder(home), 0o700); err != nil {
		t.Fatal(err)
	}
	base := StorePath(home)
	var paths []string
	for i := 0; i < 5; i++ {
		p := base + ".external." + string(rune('a'+i))
		if err := os.WriteFile(p, []byte("snap"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Distinct mtimes, newest last.
		mt := time.Now().Add(time.Duration(i-5) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	quarantinePrune(home, 2)
	left, _ := filepath.Glob(quarantineGlob(home))
	if len(left) != 2 {
		t.Fatalf("kept %d snapshots, want 2: %v", len(left), left)
	}
	for _, want := range paths[3:] {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("newest snapshot %s was pruned", want)
		}
	}
	// Under the keep limit: a no-op.
	quarantinePrune(home, 10)
	if left, _ = filepath.Glob(quarantineGlob(home)); len(left) != 2 {
		t.Errorf("no-op prune changed the set: %v", left)
	}
}

func TestHistoryCLIEdges(t *testing.T) {
	testHome(t)
	// Before init: history requires a store.
	if code, _, errOut := runVault(t, "", "history"); code == 0 || !strings.Contains(errOut, "not initialized") {
		t.Errorf("history before init: %q", errOut)
	}
	mustInit(t)
	// Legacy vault, no journal: an explicit empty-history message.
	code, out, _ := runVault(t, "", "history")
	if code != 0 || !strings.Contains(out, "no history") {
		t.Errorf("empty history = %d %q", code, out)
	}
	// Envelope mode records generations; --limit trims the output.
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	for _, k := range []string{"k1", "k2", "k3"} {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", k); code != 0 {
			t.Fatalf("add %s: %s", k, e)
		}
	}
	code, out, e := runVault(t, "", "history", "--limit=1")
	if code != 0 {
		t.Fatalf("history --limit: %s", e)
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 1 { // header + 1 row
		t.Errorf("--limit=1 should print one row:\n%s", out)
	}
}

func TestRestoreCLIEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}

	// --list prints the same table as history.
	code, out, e := runVault(t, "", "restore", "--list")
	if code != 0 || !strings.Contains(out, "GEN") {
		t.Errorf("restore --list = %d %q (%s)", code, out, e)
	}
	// A generation that never existed is refused.
	if code, _, errOut := runVault(t, "", "restore", "--yes", "--generation=424242"); code == 0 ||
		!strings.Contains(errOut, "no journal snapshot") {
		t.Errorf("bogus generation: %q", errOut)
	}
	// --dry-run reports the delta without writing.
	s, _ := LoadStore(os.Getenv(EnvHome))
	gen := s.Generation
	code, out, errOut := runVault(t, "", "restore", "--dry-run", "--generation="+itoa(gen))
	if code != 0 || !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run = %d %q (%s)", code, out, errOut)
	}
	// Without --yes there is no terminal to confirm on: refused.
	if code, _, errOut := runVault(t, "", "restore", "--generation="+itoa(gen)); code == 0 ||
		!strings.Contains(errOut, "confirmation") {
		t.Errorf("restore without --yes: %q", errOut)
	}
	// A non-admin (reader) password cannot restore.
	setPass(t, "reader-pass")
	if code, _, errOut := runVault(t, "", "restore", "--yes", "--generation="+itoa(gen)); code == 0 ||
		!strings.Contains(errOut, "admin") {
		t.Errorf("reader restore: %q", errOut)
	}
	// restore --list on an empty journal (fresh legacy vault) says so.
	dir2 := t.TempDir()
	initVaultAt(t, dir2, "p2")
	if code, out, _ := runVault(t, "", "restore", "--list"); code != 0 || !strings.Contains(out, "no restorable") {
		t.Errorf("empty restore --list = %d %q", code, out)
	}
}

func TestVerifyJournalChainNeedsKeyAndDetectsSplice(t *testing.T) {
	dir := journalHome(t)
	ik := mustRandom(t, 32)
	for i := int64(1); i <= 3; i++ {
		if err := appendJournal(dir, ik, i, "u", "vault.add", "sha", &Store{Version: storeVersion}); err != nil {
			t.Fatal(err)
		}
	}
	// No key: verification is impossible, not silently OK.
	if _, _, err := verifyJournalChain(dir, nil); err == nil {
		t.Error("verify without the integrity key must error")
	}
	// Splice: drop the middle record; the prev-chain breaks.
	recs, _ := readJournal(dir)
	if len(recs) != 3 {
		t.Fatal("setup")
	}
	raw, _ := os.ReadFile(journalPath(dir))
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	spliced := lines[0] + "\n" + lines[2] + "\n"
	if err := os.WriteFile(journalPath(dir), []byte(spliced), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, bad, err := verifyJournalChain(dir, ik)
	if err != nil {
		t.Fatal(err)
	}
	if ok || bad != 3 {
		t.Errorf("splice detection: ok=%v bad=%d, want ok=false bad=3", ok, bad)
	}
}

func TestReadJournalCorruptRecord(t *testing.T) {
	dir := journalHome(t)
	if err := os.MkdirAll(vaultFolder(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath(dir), []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readJournal(dir); err == nil || !strings.Contains(err.Error(), "corrupt journal") {
		t.Errorf("corrupt journal error = %v", err)
	}
	if _, err := findJournalGeneration(dir, 1); err == nil {
		t.Error("findJournalGeneration should propagate the corrupt-journal error")
	}
}
