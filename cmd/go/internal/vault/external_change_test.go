package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSnapshotThenBlock: on an envelope vault, a mutation that finds
// vault.jsonic edited outside aql quarantines the offending file and
// refuses; `vault restore` then recovers and mutations resume.
func TestSnapshotThenBlock(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	good, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	goodGen := good.Generation

	// Simulate an external edit: change the bytes without updating the
	// sidecar (the file still parses).
	data, err := os.ReadFile(StorePath(home))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(home), append(append([]byte(nil), data...), ' '), 0o600); err != nil {
		t.Fatal(err)
	}

	// A mutation is blocked and quarantines the file.
	code, _, errOut := runVault(t, "v2\n", "add", "--yes", "--from-stdin", "proj:k2")
	if code == 0 {
		t.Fatal("mutation should be blocked after an external edit")
	}
	if !strings.Contains(errOut, "changed outside aql") {
		t.Errorf("error = %q, want a 'changed outside aql' message", errOut)
	}
	q, _ := filepath.Glob(StorePath(home) + ".external.*")
	if len(q) == 0 {
		t.Error("no quarantine snapshot was written")
	}

	// restore recovers (it does not go through the blocked mutate path).
	if code, _, e := runVault(t, "", "restore", "--yes", "--generation", itoa(goodGen)); code != 0 {
		t.Fatalf("restore after external edit: %s", e)
	}
	// Mutations resume.
	if code, _, e := runVault(t, "v2\n", "add", "--yes", "--from-stdin", "proj:k2"); code != 0 {
		t.Fatalf("add after restore: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || strings.TrimSpace(out) != "v1" {
		t.Fatalf("original secret after recovery = %d/%q, want v1", code, out)
	}
}
