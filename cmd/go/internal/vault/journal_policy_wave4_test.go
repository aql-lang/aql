package vault

// journal_policy_wave4_test.go — journal.go and policy.go error arms:
// unreadable/corrupt journals, history/restore negatives, capability
// merge edges, and policy apply/show failures.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// w4CorruptJournal writes a non-JSON line into vault.jsonic.log.
func w4CorruptJournal(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(journalPath(home), []byte("this is not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// w4JournalAsDir makes the journal path a directory so opening it as a
// file fails with a non-IsNotExist error.
func w4JournalAsDir(t *testing.T, home string) {
	t.Helper()
	_ = os.Remove(journalPath(home))
	if err := os.Mkdir(journalPath(home), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(journalPath(home)) })
}

func TestJournalHelperArms(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	// redactStore of a slotless store nils the password slice.
	if c := redactStore(&Store{Version: 4}); c.Passwords != nil {
		t.Error("redactStore(slotless) should nil Passwords")
	}

	// appendJournal: a file-blocked folder fails MkdirAll; a directory at
	// the journal path fails the open.
	blocked := filepath.Join(t.TempDir(), "blk")
	if err := os.WriteFile(filepath.Join(t.TempDir(), "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("f"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvFolder, filepath.Join(blocked, "sub"))
	if err := appendJournal(home, nil, 1, "a", "act", "sha", nil); err == nil {
		t.Error("appendJournal under a file-blocked folder should error")
	}
	t.Setenv(EnvFolder, "")
	w4JournalAsDir(t, home)
	if err := appendJournal(home, nil, 1, "a", "act", "sha", nil); err == nil {
		t.Error("appendJournal with a directory at the log path should error")
	}
	if _, err := readJournal(home); err == nil {
		t.Error("readJournal with a directory at the log path should error")
	}
	if err := pruneJournal(home, 0); err == nil {
		t.Error("pruneJournal with an unreadable log should error")
	}
	if _, _, err := verifyJournalChain(home, []byte("k")); err == nil {
		t.Error("verifyJournalChain with an unreadable log should error")
	}
}

func TestJournalReadSkipsBlankLines(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	rec := journalRecord{Generation: 1, SHA256: "s"}
	line, err := json.Marshal(&rec)
	if err != nil {
		t.Fatal(err)
	}
	content := "\n" + string(line) + "\n\n"
	if err := os.WriteFile(journalPath(home), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, err := readJournal(home)
	if err != nil || len(recs) != 1 {
		t.Errorf("readJournal = %d recs, %v; want 1", len(recs), err)
	}
}

func TestHistoryErrorArms(t *testing.T) {
	testHome(t)
	if code, _, _ := runVault(t, "", "history", "-nope"); code == 0 {
		t.Error("history with a bad flag should fail")
	}
	home := testHome(t)
	mustInit(t)
	w4CorruptJournal(t, home)
	if code, _, e := runVault(t, "", "history"); code == 0 || !strings.Contains(e, "corrupt journal") {
		t.Errorf("history over a corrupt journal = %d, %q", code, e)
	}
}

func TestRestoreErrorArms(t *testing.T) {
	t.Run("parse and store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, _ := runVault(t, "", "restore", "-nope"); code == 0 {
			t.Error("restore with a bad flag should fail")
		}
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "restore", "--list"); code == 0 {
			t.Error("restore on a corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "restore", "--list"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("restore wrong pass = %d, %q", code, e)
		}
	})
	t.Run("corrupt journal", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		w4CorruptJournal(t, home)
		if code, _, e := runVault(t, "", "restore", "--list"); code == 0 ||
			!strings.Contains(e, "corrupt journal") {
			t.Errorf("restore over a corrupt journal = %d, %q", code, e)
		}
	})
	t.Run("blocked lock", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		// Learn a restorable generation from history.
		recs, err := readJournal(home)
		if err != nil || len(recs) == 0 {
			t.Fatalf("journal: %d recs, %v", len(recs), err)
		}
		gen := recs[0].Generation
		w4BlockLock(t, home)
		code, _, e := runVault(t, "", "restore", "--yes",
			"--generation="+jsonNumber(gen))
		if code == 0 || e == "" {
			t.Errorf("restore with a blocked lock = %d, %q", code, e)
		}
	})
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestMergeCapabilitiesMeters(t *testing.T) {
	cur := []Capability{{ID: "a", UsedCalls: 5, UsedCostCents: 90, Revoked: true}}
	target := []Capability{{ID: "a", UsedCalls: 1, UsedCostCents: 10}}
	out := mergeCapabilities(cur, target)
	if len(out) != 1 || !out[0].Revoked || out[0].UsedCalls != 5 || out[0].UsedCostCents != 90 {
		t.Errorf("mergeCapabilities = %+v", out)
	}
}

// w4WritePolicy writes a policy JSON document to a temp file.
func w4WritePolicy(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPolicyApplyErrorArms(t *testing.T) {
	t.Run("parse error", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "policy", "apply", "-nope", "p.json"); code == 0 {
			t.Error("policy apply with a bad flag should fail")
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		p := w4WritePolicy(t, `{"version":1}`)
		if code, _, _ := runVault(t, "", "policy", "apply", p); code == 0 {
			t.Error("policy apply on a corrupt store should fail")
		}
	})
	t.Run("locked vault", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		p := w4WritePolicy(t, `{"version":1}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 || !strings.Contains(e, "locked") {
			t.Errorf("policy apply on a locked vault = %d, %q", code, e)
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		p := w4WritePolicy(t, `{"version":1}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("policy apply wrong pass = %d, %q", code, e)
		}
	})
	t.Run("requires admin", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "reader-pass")
		p := w4WritePolicy(t, `{"version":1}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("policy apply by a read password = %d, %q", code, e)
		}
	})
	t.Run("invalid alias name", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		p := w4WritePolicy(t, `{"version":1,"aliases":[{"name":"bad name!"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "invalid alias") {
			t.Errorf("policy with an invalid alias = %d, %q", code, e)
		}
	})
	t.Run("empty FromEnv value", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		t.Setenv("W4_POLICY_EMPTY", "")
		p := w4WritePolicy(t, `{"version":1,"aliases":[{"name":"k","from_env":"W4_POLICY_EMPTY"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "is empty") {
			t.Errorf("policy with an empty FromEnv = %d, %q", code, e)
		}
	})
	t.Run("provision into an uncreated namespace", func(t *testing.T) {
		w4EnvelopeVault(t)
		t.Setenv("W4_POLICY_VAL", "sekrit")
		p := w4WritePolicy(t, `{"version":1,"aliases":[{"name":"newns:k","from_env":"W4_POLICY_VAL"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "storing newns:k") {
			t.Errorf("policy provisioning a brand-new namespace = %d, %q", code, e)
		}
	})
	t.Run("capability for an unknown alias", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		p := w4WritePolicy(t, `{"version":1,"capabilities":[{"alias":"nope","agent":"a"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 ||
			!strings.Contains(e, "unknown alias") {
			t.Errorf("policy cap for a missing alias = %d, %q", code, e)
		}
	})
	t.Run("bad capability ttl", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		p := w4WritePolicy(t, `{"version":1,"capabilities":[{"alias":"k","agent":"a","ttl":"bogus"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code == 0 || e == "" {
			t.Errorf("policy cap with a bad ttl = %d, %q", code, e)
		}
	})
	t.Run("reapply revokes and skips unrelated caps", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		for _, a := range []string{"k", "other"} {
			if code, _, e := runVault(t, "v\n", "add", "--from-stdin", a); code != 0 {
				t.Fatalf("seed %s: %s", a, e)
			}
		}
		// An unrelated live capability must survive the policy reapply.
		if code, _, e := runVault(t, "", "grant", "--agent=bystander", "other"); code != 0 {
			t.Fatalf("grant: %s", e)
		}
		p := w4WritePolicy(t, `{"version":1,"capabilities":[{"alias":"k","agent":"a","ttl":"1h"}]}`)
		if code, out, e := runVault(t, "", "policy", "apply", p); code != 0 ||
			!strings.Contains(out, "+capability k@a") {
			t.Errorf("first apply = %d, %q (%s)", code, out, e)
		}
		code, out, e := runVault(t, "", "policy", "apply", p)
		if code != 0 || !strings.Contains(out, "~capability k@a") {
			t.Errorf("reapply = %d, %q (%s)", code, out, e)
		}
	})
	t.Run("no changes", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		p := w4WritePolicy(t, `{"version":1,"aliases":[{"name":"k"}]}`)
		if code, _, e := runVault(t, "", "policy", "apply", p); code != 0 {
			t.Fatalf("first apply: %s", e)
		}
		code, out, e := runVault(t, "", "policy", "apply", p)
		if code != 0 || !strings.Contains(out, "no changes") {
			t.Errorf("idempotent reapply = %d, %q (%s)", code, out, e)
		}
	})
	t.Run("show corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "policy", "show"); code == 0 {
			t.Error("policy show on a corrupt store should fail")
		}
	})
}
