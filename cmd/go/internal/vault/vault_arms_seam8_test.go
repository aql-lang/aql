package vault

// vault_arms_seam8_test.go — wave-8 coverage for residual non-TUI error/guard
// arms across the vault command layer. Failure arms that a healthy filesystem
// or the sealed payload shape never reaches are driven through the committed
// package seams (jsonMarshal / fileWrite / c7scryptKey / c7loadStore /
// c7findJournalGeneration / c7newCapability / c7sess*Value / isTerminal /
// openTTY), each swapped with a t.Cleanup restore.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var errW8Boom = errors.New("w8 boom")

// w8swapC7loadStore swaps c7loadStore with restore.
func w8swapC7loadStore(t *testing.T, fn func(string) (*Store, error)) {
	t.Helper()
	old := c7loadStore
	c7loadStore = fn
	t.Cleanup(func() { c7loadStore = old })
}

// w8swapC7newCapability swaps c7newCapability with restore.
func w8swapC7newCapability(t *testing.T, fn func(*Store, string, string, []string, []string, time.Duration) (*Capability, string, error)) {
	t.Helper()
	old := c7newCapability
	c7newCapability = fn
	t.Cleanup(func() { c7newCapability = old })
}

func w8newCapErr(*Store, string, string, []string, []string, time.Duration) (*Capability, string, error) {
	return nil, "", errW8Boom
}

// w8writePolicy writes a policy JSON file under home and returns its path.
func w8writePolicy(t *testing.T, home string, pol Policy) string {
	t.Helper()
	path := filepath.Join(home, "w8-pol.json")
	b, err := json.Marshal(pol)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestW8_RunRestoreFindGenerationError drives runRestore's c7findJournalGeneration
// error arm.
func TestW8_RunRestoreFindGenerationError(t *testing.T) {
	testHome(t)
	mustInit(t)
	old := c7findJournalGeneration
	t.Cleanup(func() { c7findJournalGeneration = old })
	c7findJournalGeneration = func(string, int64) (*journalRecord, error) { return nil, errW8Boom }
	if code, _, _ := runVault(t, "", "restore", "--generation=1"); code != 1 {
		t.Errorf("restore with a generation-lookup error should return 1, got %d", code)
	}
}

// TestW8_RunRestoreReloadErrorAndNil drives runRestore's under-lock c7loadStore
// error and nil arms. A fake generation record (non-nil Store) carries the flow
// past confirmation to the locked reload.
func TestW8_RunRestoreReloadErrorAndNil(t *testing.T) {
	fakeGen := func(string, int64) (*journalRecord, error) {
		return &journalRecord{Store: &Store{Version: storeVersion}}, nil
	}

	t.Run("reload-error", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		old := c7findJournalGeneration
		t.Cleanup(func() { c7findJournalGeneration = old })
		c7findJournalGeneration = fakeGen
		w8swapC7loadStore(t, func(string) (*Store, error) { return nil, errW8Boom })
		if code, _, _ := runVault(t, "", "restore", "--generation=1", "--yes"); code != 1 {
			t.Errorf("restore reload error should return 1, got %d", code)
		}
	})

	t.Run("reload-nil", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		old := c7findJournalGeneration
		t.Cleanup(func() { c7findJournalGeneration = old })
		c7findJournalGeneration = fakeGen
		w8swapC7loadStore(t, func(string) (*Store, error) { return nil, nil })
		if code, _, _ := runVault(t, "", "restore", "--generation=1", "--yes"); code != 1 {
			t.Errorf("restore reload nil should return 1, got %d", code)
		}
	})
}

// TestW8_PolicyApplyReloadArms drives runPolicyApply's under-lock c7loadStore
// error, nil, and locked arms (the early pre-lock check sees the real,
// unlocked store, so the swapped seam only affects the locked reload).
func TestW8_PolicyApplyReloadArms(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) (*Store, error)
	}{
		{"reload-error", func(string) (*Store, error) { return nil, errW8Boom }},
		{"reload-nil", func(string) (*Store, error) { return nil, nil }},
		{"reload-locked", func(string) (*Store, error) { return &Store{Version: storeVersion, Locked: true}, nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := testHome(t)
			mustInit(t)
			path := w8writePolicy(t, home, Policy{Version: 1, Aliases: []PolicyAlias{{Name: "k"}}})
			w8swapC7loadStore(t, tc.fn)
			if code, _, _ := runVault(t, "", "policy", "apply", path); code != 1 {
				t.Errorf("policy apply %s should return 1, got %d", tc.name, code)
			}
		})
	}
}

// TestW8_PolicyApplyGrantError drives runPolicyApply's c7newCapability error
// arm: a policy capability whose mint fails.
func TestW8_PolicyApplyGrantError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	path := w8writePolicy(t, home, Policy{
		Version:      1,
		Capabilities: []PolicyCapability{{Alias: "k", Agent: "ci", TTL: "1h"}},
	})
	w8swapC7newCapability(t, w8newCapErr)
	if code, _, _ := runVault(t, "", "policy", "apply", path); code != 1 {
		t.Errorf("policy apply with a capability-mint error should return 1, got %d", code)
	}
}

// TestW8_RunGrantNewCapabilityError drives runGrant's c7newCapability error arm.
func TestW8_RunGrantNewCapabilityError(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	w8swapC7newCapability(t, w8newCapErr)
	if code, _, _ := runVault(t, "", "grant", "--agent=ci", "k"); code != 1 {
		t.Errorf("grant with a capability-mint error should return 1, got %d", code)
	}
}

// TestW8_RunInitReCheckExisting drives runInit's under-lock re-check arm: the
// pre-lock LoadStore sees no store (fresh home), but the seamed c7loadStore
// reports an existing store, so init refuses.
func TestW8_RunInitReCheckExisting(t *testing.T) {
	testHome(t) // no vault on disk
	w8swapC7loadStore(t, func(string) (*Store, error) { return &Store{Version: storeVersion}, nil })
	if code, _, _ := runVault(t, "", "init", "--backend=file"); code != 1 {
		t.Errorf("init with an existing store under lock should return 1, got %d", code)
	}
}

// TestW8_MvVerifyReadError drives runMv's copy-verify read error arm: the
// verify read of the destination fails.
func TestW8_MvVerifyReadError(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := c7sessGetValue
	t.Cleanup(func() { c7sessGetValue = old })
	n := 0
	c7sessGetValue = func(s *Session, alias, ns string) (string, error) {
		n++
		if n == 2 { // the destination verify read
			return "", errW8Boom
		}
		return old(s, alias, ns)
	}
	if code, _, _ := runVault(t, "", "mv", "k", "k2"); code != 1 {
		t.Errorf("mv with a verify-read error should return 1, got %d", code)
	}
}

// TestW8_MvVerifyMismatch drives runMv's copy-verify mismatch arm: the
// destination reads back a different value than was written.
func TestW8_MvVerifyMismatch(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := c7sessGetValue
	t.Cleanup(func() { c7sessGetValue = old })
	n := 0
	c7sessGetValue = func(s *Session, alias, ns string) (string, error) {
		n++
		if n == 2 { // destination verify read returns a wrong value
			return "not-the-value", nil
		}
		return old(s, alias, ns)
	}
	if code, _, _ := runVault(t, "", "mv", "k", "k2"); code != 1 {
		t.Errorf("mv with a verify mismatch should return 1, got %d", code)
	}
}

// TestW8_MvAbortDeletesCopies drives runMv's abort rollback loop: a batch
// namespace move where the first copy succeeds and the second fails, so the
// abort path deletes the already-made copy.
func TestW8_MvAbortDeletesCopies(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "va\n", "add", "--from-stdin", "proj:a"); code != 0 {
		t.Fatalf("seed proj:a: %s", e)
	}
	if code, _, e := runVault(t, "vb\n", "add", "--from-stdin", "proj:b"); code != 0 {
		t.Fatalf("seed proj:b: %s", e)
	}
	old := c7sessGetValue
	t.Cleanup(func() { c7sessGetValue = old })
	n := 0
	c7sessGetValue = func(s *Session, alias, ns string) (string, error) {
		n++
		if n == 3 { // second move's source read fails, after the first fully copied
			return "", errW8Boom
		}
		return old(s, alias, ns)
	}
	deleted := 0
	oldDel := c7sessDeleteValue
	t.Cleanup(func() { c7sessDeleteValue = oldDel })
	c7sessDeleteValue = func(s *Session, alias string) error {
		deleted++
		return oldDel(s, alias)
	}
	if code, _, _ := runVault(t, "", "mv", "proj:", "other:"); code != 1 {
		t.Errorf("batch move with a mid-batch failure should return 1, got %d", code)
	}
	if deleted == 0 {
		t.Error("abort should have deleted the already-made copy")
	}
}

// TestW8_MvPhase3DeleteWarning drives runMv's phase-3 delete warning arm: the
// old keyring entry cannot be removed after the store already references the
// new name, so mv warns but still succeeds.
func TestW8_MvPhase3DeleteWarning(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := c7sessDeleteValue
	t.Cleanup(func() { c7sessDeleteValue = old })
	c7sessDeleteValue = func(*Session, string) error { return errW8Boom }
	code, _, errOut := runVault(t, "", "mv", "k", "k2")
	if code != 0 {
		t.Errorf("mv with only a phase-3 delete failure should still succeed (code 0), got %d", code)
	}
	if !strings.Contains(errOut, "warning") {
		t.Errorf("phase-3 delete failure should warn, stderr=%q", errOut)
	}
}

// TestW8_DenialMessageExpired covers denialMessage's "expired" arm.
func TestW8_DenialMessageExpired(t *testing.T) {
	if got := denialMessage("expired"); got != "capability expired" {
		t.Errorf("denialMessage(expired) = %q, want %q", got, "capability expired")
	}
}

// TestW8_RunExportSealError drives runExport's sealExport error arm through the
// c7scryptKey seam (the export envelope KDF), which the bundle read does not
// touch — so the failure lands precisely at the seal step.
func TestW8_RunExportSealError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sekret\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	old := c7scryptKey
	t.Cleanup(func() { c7scryptKey = old })
	c7scryptKey = func(string, []byte) ([]byte, error) { return nil, errW8Boom }

	t.Setenv(EnvExportPassphrase, "bundlepass")
	out := filepath.Join(t.TempDir(), "b.borux")
	var errb strings.Builder
	if code := runExport([]string{"--out=" + out}, home, strings.NewReader(""), &errb, &errb); code != 1 {
		t.Fatalf("seal error should return 1, got %d (stderr=%q)", code, errb.String())
	}
}

// TestW8_AppendJournalMarshalError drives appendJournal's jsonMarshal error arm.
func TestW8_AppendJournalMarshalError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	old := jsonMarshal
	t.Cleanup(func() { jsonMarshal = old })
	jsonMarshal = func(any) ([]byte, error) { return nil, errW8Boom }
	if err := appendJournal(home, nil, 1, "actor", "act", "sha", nil); err == nil {
		t.Error("appendJournal should surface the marshal error")
	}
}

// TestW8_AppendJournalWriteError drives appendJournal's fileWrite error arm
// (the marshal + file open succeed; only the write fails).
func TestW8_AppendJournalWriteError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	old := fileWrite
	t.Cleanup(func() { fileWrite = old })
	fileWrite = func(*os.File, []byte) (int, error) { return 0, errW8Boom }
	if err := appendJournal(home, nil, 1, "actor", "act", "sha", nil); err == nil {
		t.Error("appendJournal should surface the file-write error")
	}
}

// TestW8_PruneJournalMarshalError drives pruneJournal's jsonMarshal error arm:
// with more records than the keep count, the rewrite loop marshals each.
func TestW8_PruneJournalMarshalError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	for i := 0; i < 3; i++ {
		if err := appendJournal(home, nil, int64(i+1), "actor", "act", "sha", nil); err != nil {
			t.Fatalf("seed journal: %v", err)
		}
	}
	old := jsonMarshal
	t.Cleanup(func() { jsonMarshal = old })
	jsonMarshal = func(any) ([]byte, error) { return nil, errW8Boom }
	if err := pruneJournal(home, 1); err == nil {
		t.Error("pruneJournal should surface the marshal error while rewriting")
	}
}

// TestW8_QuarantinePruneStatError drives quarantinePrune's os.Stat error arm: a
// broken symlink matching the quarantine glob is found but cannot be stat'd.
func TestW8_QuarantinePruneStatError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	broken := StorePath(home) + ".external.brokenlink"
	if err := os.Symlink(filepath.Join(home, "does-not-exist"), broken); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// keep=0 with one match means the prune loop runs and hits the stat error.
	quarantinePrune(home, 0)
	// The broken link is left in place (stat failed -> skipped, not removed).
	if _, err := os.Lstat(broken); err != nil {
		t.Errorf("broken quarantine link should be left untouched: %v", err)
	}
}

// TestW8_OpenPromptReaderUsesTTY drives openPromptReader's redirected-stdin
// branch: a non-terminal *os.File stdin routes the prompt to the controlling
// terminal (openTTY seam), and the returned closer releases it.
func TestW8_OpenPromptReaderUsesTTY(t *testing.T) {
	oldIT := isTerminal
	t.Cleanup(func() { isTerminal = oldIT })
	isTerminal = func(int) bool { return false } // stdin is "redirected"

	fakeTTY, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	oldTTY := openTTY
	t.Cleanup(func() { openTTY = oldTTY })
	openTTY = func() (*os.File, error) { return fakeTTY, nil }

	stdin, _ := w8PipeFiles(t) // a real *os.File
	ir, closer, err := openPromptReader(stdin)
	if err != nil || ir == nil || closer == nil {
		t.Fatalf("openPromptReader returned err=%v, reader-nil=%v, closer-nil=%v", err, ir == nil, closer == nil)
	}
	closer() // releases the fake tty handle
}
