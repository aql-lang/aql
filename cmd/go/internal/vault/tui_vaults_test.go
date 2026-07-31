package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMeta drops a minimal valid vault metadata file at folder/<suffix>.
func writeMeta(t *testing.T, folder, suffix, backend string) {
	t.Helper()
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":4,"backend":"` + backend + `"}`)
	if err := os.WriteFile(filepath.Join(folder, metaFileName(suffix)), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSuffixFromMetaName(t *testing.T) {
	cases := []struct {
		name   string
		want   string
		wantOK bool
	}{
		{"vault.jsonic", "", true},
		{"vault.work.jsonic", "work", true},
		{"vault.team.prod.jsonic", "team.prod", true},
		// Non-metadata siblings must be rejected.
		{"vault.jsonic.sig", "", false},
		{"vault.jsonic.log", "", false},
		{"vault.keyring", "", false},
		{"vault.audit.jsonl", "", false},
		{"notvault.jsonic", "", false},
		{"vault..jsonic", "", false}, // leading-dot suffix is invalid
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := suffixFromMetaName(tc.name)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("suffixFromMetaName(%q) = (%q,%v), want (%q,%v)", tc.name, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestEnumerateVaultsScanAndIndex(t *testing.T) {
	home := testHome(t)
	defaultFolder := filepath.Join(home, ".boru")

	// Two vaults in the default folder (default + suffixed sibling)...
	writeMeta(t, defaultFolder, "", "file")
	writeMeta(t, defaultFolder, "work", "keychain")
	// ...and one in a custom --folder location, recorded only in the index.
	custom := filepath.Join(home, "ci-vault")
	writeMeta(t, custom, "prod", "file")
	if err := recordVaultInit(home, custom, "prod", "file"); err != nil {
		t.Fatalf("recordVaultInit: %s", err)
	}
	// A stale index entry whose metadata file does not exist.
	if err := recordVaultInit(home, filepath.Join(home, "gone"), "old", "file"); err != nil {
		t.Fatalf("recordVaultInit stale: %s", err)
	}

	vs, err := enumerateVaults(home)
	if err != nil {
		t.Fatalf("enumerateVaults: %s", err)
	}

	byKey := map[string]discoveredVault{}
	for _, dv := range vs {
		byKey[dv.Folder+"|"+dv.Suffix] = dv
	}
	if len(byKey) != 4 {
		t.Fatalf("want 4 distinct vaults, got %d: %+v", len(byKey), vs)
	}

	def := byKey[filepath.Clean(defaultFolder)+"|"]
	if !def.Exists || !def.InScan {
		t.Errorf("default vault should be scanned+existing: %+v", def)
	}
	work := byKey[filepath.Clean(defaultFolder)+"|work"]
	if !work.Exists || work.Backend != "keychain" {
		t.Errorf("work vault wrong: %+v", work)
	}
	prod := byKey[filepath.Clean(custom)+"|prod"]
	if !prod.Exists || !prod.InScan || !prod.InIdx {
		t.Errorf("custom-folder vault should be both scanned and indexed: %+v", prod)
	}
	stale := byKey[filepath.Clean(filepath.Join(home, "gone"))+"|old"]
	if stale.Exists || !stale.InIdx {
		t.Errorf("stale entry should be index-only and non-existent: %+v", stale)
	}
}

func TestRunVaultsAddRegistersExisting(t *testing.T) {
	home := testHome(t)
	// A pre-existing vault in a custom folder, not recorded in any index.
	custom := filepath.Join(home, "external")
	writeMeta(t, custom, "sdk", "file")
	idx, _ := loadVaultIndex(home)
	if _, i := idx.find(custom, "sdk"); i >= 0 {
		t.Fatal("precondition: vault should not be indexed yet")
	}

	// Register it via `vault folder add`; the single suffix is auto-detected.
	code, out, errOut := runVault(t, "", "folder", "add", custom)
	if code != 0 {
		t.Fatalf("folder add: %s", errOut)
	}
	if !strings.Contains(out, "sdk") {
		t.Errorf("output should name the registered vault: %q", out)
	}
	idx, _ = loadVaultIndex(home)
	if ref, _ := idx.find(custom, "sdk"); ref == nil || ref.Backend != "file" {
		t.Errorf("vault not registered with backend: %+v", idx.Vaults)
	}

	// The legacy `vaults` spelling still routes (back-compat alias).
	other := filepath.Join(home, "other")
	writeMeta(t, other, "x", "file")
	if code, _, errOut := runVault(t, "", "vaults", "add", other); code != 0 {
		t.Errorf("`vaults` alias should still work: %s", errOut)
	}

	// Negative: a folder with no vault is rejected.
	empty := filepath.Join(home, "empty")
	if err := os.MkdirAll(empty, 0700); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runVault(t, "", "folder", "add", empty); code == 0 {
		t.Errorf("adding a folder with no vault should fail")
	}

	// Negative: an ambiguous folder (two vaults) needs --suffix.
	multi := filepath.Join(home, "multi")
	writeMeta(t, multi, "a", "file")
	writeMeta(t, multi, "b", "file")
	if code, _, errOut := runVault(t, "", "folder", "add", multi); code == 0 {
		t.Errorf("ambiguous folder should require --suffix")
	} else if !strings.Contains(errOut, "--suffix") {
		t.Errorf("error should mention --suffix: %q", errOut)
	}
	// ...but works with an explicit suffix.
	if code, _, errOut := runVault(t, "", "folder", "add", "--suffix=b", multi); code != 0 {
		t.Errorf("explicit --suffix should succeed: %s", errOut)
	}
}

func TestVaultIndexUpsertDedupAndPrune(t *testing.T) {
	home := testHome(t)
	if err := recordVaultInit(home, "/srv/v", "a", "file"); err != nil {
		t.Fatal(err)
	}
	// Re-record the same (folder, suffix): must update, not duplicate; and
	// must preserve the original CreatedAt.
	idx, _ := loadVaultIndex(home)
	orig := idx.Vaults[0].CreatedAt
	if err := recordVaultOpened(home, "/srv/v", "a"); err != nil {
		t.Fatal(err)
	}
	idx, _ = loadVaultIndex(home)
	if len(idx.Vaults) != 1 {
		t.Fatalf("want 1 entry after re-record, got %d", len(idx.Vaults))
	}
	if idx.Vaults[0].CreatedAt != orig {
		t.Errorf("CreatedAt should be preserved: was %q now %q", orig, idx.Vaults[0].CreatedAt)
	}
	if idx.Vaults[0].LastOpenedAt == "" {
		t.Errorf("LastOpenedAt should be stamped")
	}

	// The index must never contain secret-like material — only locations.
	data, err := os.ReadFile(vaultIndexPath(home))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"verifier", "enc_priv_key", "wrapped_keys", "token_hash", "vault_salt"} {
		if strings.Contains(string(data), banned) {
			t.Errorf("index file leaked secret-bearing field %q", banned)
		}
	}

	if err := pruneVaultEntry(home, "/srv/v", "a"); err != nil {
		t.Fatal(err)
	}
	idx, _ = loadVaultIndex(home)
	if len(idx.Vaults) != 0 {
		t.Fatalf("want 0 entries after prune, got %d", len(idx.Vaults))
	}
	// Pruning a missing entry is a no-op, not an error.
	if err := pruneVaultEntry(home, "/srv/v", "a"); err != nil {
		t.Errorf("prune of missing entry should be a no-op: %s", err)
	}
}
