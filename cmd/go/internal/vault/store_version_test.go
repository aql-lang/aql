package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStoreFile drops raw bytes at the store path under a fresh temp
// home and returns that home dir.
func writeStoreFile(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".aql"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(dir), data, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLoadMigratesV1Golden loads a checked-in v1 store and asserts it
// migrates cleanly: version bumped, alias preserved, and the legacy
// (pre-hash) capability explicitly revoked rather than silently broken.
func TestLoadMigratesV1Golden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "vault.v1.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeStoreFile(t, data)

	s, err := LoadStore(dir)
	if err != nil {
		t.Fatalf("LoadStore: %s", err)
	}
	if s.Version != storeVersion {
		t.Errorf("version = %d after migration, want %d", s.Version, storeVersion)
	}
	if a, _ := s.FindAlias("openai"); a == nil {
		t.Errorf("alias lost during migration")
	}
	if len(s.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(s.Capabilities))
	}
	if !s.Capabilities[0].Revoked {
		t.Errorf("legacy capability (no token hash) should be revoked by v1->v2 migration")
	}
	if len(s.ActiveCapabilities(nowUTC())) != 0 {
		t.Errorf("migrated legacy capability should not be active")
	}
}

// TestLoadMigratesV2Golden loads a checked-in v2 store and asserts the
// v2->v3 bump is a clean no-op: version advances, aliases survive with
// no expiry conjured onto them, and a v2 (token-hashed) capability stays
// valid rather than being revoked like the pre-hash v1 ones.
func TestLoadMigratesV2Golden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "vault.v2.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeStoreFile(t, data)

	s, err := LoadStore(dir)
	if err != nil {
		t.Fatalf("LoadStore: %s", err)
	}
	if s.Version != storeVersion {
		t.Errorf("version = %d after migration, want %d", s.Version, storeVersion)
	}
	a, _ := s.FindAlias("openai")
	if a == nil {
		t.Fatalf("alias lost during migration")
	}
	if a.ExpiresAt != "" {
		t.Errorf("migration invented an expiry %q on a v2 alias", a.ExpiresAt)
	}
	if len(s.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(s.Capabilities))
	}
	if s.Capabilities[0].Revoked {
		t.Errorf("v2 token-hashed capability should not be revoked by v2->v3")
	}
	if len(s.ActiveCapabilities(nowUTC())) != 1 {
		t.Errorf("v2 capability should remain active after migration")
	}
}

// TestLoadMigratesV3Golden loads a checked-in v3 store and asserts the
// v3->v4 bump is a clean no-op: version advances, the alias expiry is
// preserved (not invented or dropped), no password slots are conjured,
// and the v3 capability stays valid. (Golden-per-version, ADR-009/006.)
func TestLoadMigratesV3Golden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "vault.v3.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeStoreFile(t, data)

	s, err := LoadStore(dir)
	if err != nil {
		t.Fatalf("LoadStore: %s", err)
	}
	if s.Version != storeVersion {
		t.Errorf("version = %d after migration, want %d", s.Version, storeVersion)
	}
	a, _ := s.FindAlias("openai")
	if a == nil {
		t.Fatalf("alias lost during migration")
	}
	if a.ExpiresAt != "2099-06-01T00:00:00Z" {
		t.Errorf("v3->v4 changed the alias expiry: %q", a.ExpiresAt)
	}
	if s.HasPasswordSlots() {
		t.Errorf("v3->v4 conjured password slots: %d", len(s.Passwords))
	}
	if len(s.ActiveCapabilities(nowUTC())) != 1 {
		t.Errorf("v3 capability should remain active after migration")
	}
}

// TestLoadRejectsFutureVersion ensures a store written by a newer aql
// is refused, not parsed leniently (which would silently strip unknown
// fields on the next save).
func TestLoadRejectsFutureVersion(t *testing.T) {
	data, _ := json.Marshal(Store{Version: storeVersion + 1, Backend: "file"})
	dir := writeStoreFile(t, data)

	_, err := LoadStore(dir)
	if err == nil {
		t.Fatal("expected error loading a future-version store")
	}
	if !strings.Contains(err.Error(), "upgrade aql") {
		t.Errorf("error should advise upgrading aql, got %q", err.Error())
	}
}

// TestLoadRejectsFutureVersionViaStatus checks the gate surfaces at the
// CLI: `vault status` on a future-version store fails cleanly.
func TestLoadRejectsFutureVersionViaStatus(t *testing.T) {
	data, _ := json.Marshal(Store{Version: storeVersion + 1, Backend: "file"})
	dir := writeStoreFile(t, data)
	t.Setenv(EnvHome, dir)
	t.Setenv(EnvPassphrase, "test-pass")

	code, _, errOut := runVault(t, "", "status")
	if code == 0 {
		t.Fatal("expected non-zero exit for future-version store")
	}
	if !strings.Contains(errOut, "upgrade aql") {
		t.Errorf("status error should advise upgrading aql, got %q", errOut)
	}
}

// TestStoreSaveLoadRoundTripStable verifies that save -> load -> save
// is byte-stable for the current version, so a no-op load/save never
// rewrites the file in a way that would churn diffs or lose data.
func TestStoreSaveLoadRoundTripStable(t *testing.T) {
	dir := t.TempDir()
	s := &Store{
		Version: storeVersion,
		Backend: BackendFile,
		Config:  map[string]any{"audit.enabled": "true", "z": "1", "a": "2"},
	}
	s.UpsertAlias(Alias{Name: "k", Provider: "openai"})
	if _, _, err := s.NewCapability("k", "agent", []string{"api.openai.com"}, []string{"POST"}, hour()); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(dir, loaded); err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Errorf("save/load/save not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", b1, b2)
	}
}

// TestMigrationRegistryLength guards the invariant tying the migration
// table to the schema version, so bumping one without the other fails
// fast.
func TestMigrationRegistryLength(t *testing.T) {
	if len(storeMigrations) != storeVersion-1 {
		t.Errorf("storeMigrations has %d entries, want storeVersion-1 = %d", len(storeMigrations), storeVersion-1)
	}
}

// TestPolicyRejectsFutureVersion ensures `vault policy apply` refuses a
// policy file declaring a newer schema than this binary understands.
func TestPolicyRejectsFutureVersion(t *testing.T) {
	testHome(t)
	mustInit(t)
	path := filepath.Join(t.TempDir(), "policy.json")
	body, _ := json.Marshal(Policy{Version: policyVersion + 1})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runVault(t, "", "policy", "apply", path)
	if code == 0 {
		t.Fatal("expected non-zero exit for future-version policy")
	}
	if !strings.Contains(errOut, "upgrade aql") {
		t.Errorf("policy error should advise upgrading aql, got %q", errOut)
	}
}
