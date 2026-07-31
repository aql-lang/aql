package vault

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// t7writeCorruptIndex drops an unparseable vaults.jsonic at the home index path.
func t7writeCorruptIndex(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(homeBoruDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vaultIndexPath(home), []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestT7_LoadVaultIndexReadError drives loadVaultIndex' non-not-exist ReadFile
// error arm by making the index path a directory (EISDIR, not IsNotExist).
func TestT7_LoadVaultIndexReadError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(vaultIndexPath(home), 0o700); err != nil { // path is a DIR
		t.Fatal(err)
	}
	if _, err := loadVaultIndex(home); err == nil {
		t.Error("loadVaultIndex should error when the index path is a directory")
	}
}

// TestT7_LoadVaultIndexVersionDefault drives the "version 0 -> default" arm.
func TestT7_LoadVaultIndexVersionDefault(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(homeBoruDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vaultIndexPath(home), []byte(`{"vaults":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := loadVaultIndex(home)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Version != vaultIndexVersion {
		t.Errorf("version = %d, want defaulted to %d", idx.Version, vaultIndexVersion)
	}
}

// TestT7_SaveVaultIndexMkdirError drives saveVaultIndex' MkdirAll error arm.
func TestT7_SaveVaultIndexMkdirError(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".boru"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveVaultIndex(home, &vaultIndex{Version: vaultIndexVersion}); err == nil {
		t.Error("saveVaultIndex should fail when ~/.boru is a file")
	}
}

// TestT7_VaultIndexUpsertExistingFields drives upsert's Label and
// CreatedAt-when-empty arms on an existing entry.
func TestT7_VaultIndexUpsertExistingFields(t *testing.T) {
	idx := &vaultIndex{}
	idx.upsert(vaultRef{Folder: "/v", Suffix: "a"}) // new: no Label, no CreatedAt
	idx.upsert(vaultRef{Folder: "/v", Suffix: "a", Label: "My Vault", CreatedAt: "2020-01-01T00:00:00Z"})
	ref, _ := idx.find("/v", "a")
	if ref == nil {
		t.Fatal("entry vanished")
	}
	if ref.Label != "My Vault" {
		t.Errorf("Label not applied on upsert: %q", ref.Label)
	}
	if ref.CreatedAt != "2020-01-01T00:00:00Z" {
		t.Errorf("CreatedAt not applied to empty existing entry: %q", ref.CreatedAt)
	}
}

// TestT7_RecordAndDefaultWithCorruptIndex drives the "corrupt index -> start
// fresh" arms of recordVaultInit and setDefaultVault (both best-effort).
func TestT7_RecordAndDefaultWithCorruptIndex(t *testing.T) {
	home := t.TempDir()
	t7writeCorruptIndex(t, home)
	if err := recordVaultInit(home, "/v", "s", "file"); err != nil {
		t.Errorf("recordVaultInit should recover from a corrupt index: %v", err)
	}
	t7writeCorruptIndex(t, home)
	if err := setDefaultVault(home, "/v", "s"); err != nil {
		t.Errorf("setDefaultVault should recover from a corrupt index: %v", err)
	}
}

// TestT7_RegisterVaultCorruptIndex drives registerVault's loadVaultIndex error
// arm (it propagates rather than recovering).
func TestT7_RegisterVaultCorruptIndex(t *testing.T) {
	home := t.TempDir()
	t7writeCorruptIndex(t, home)
	if err := registerVault(home, "/v", "s", "file"); err == nil {
		t.Error("registerVault should surface a corrupt-index load error")
	}
}

// TestT7_ScanFolderVaultsBadPattern drives scanFolderVaults' Glob error arm.
func TestT7_ScanFolderVaultsBadPattern(t *testing.T) {
	if got := scanFolderVaults("/tmp/bad[glob"); got != nil {
		t.Errorf("scanFolderVaults on a bad glob pattern = %v, want nil", got)
	}
}

// TestT7_EnumerateBackendFill drives both backend-fill arms in enumerateVaults:
// a scanned row whose metadata has no backend but the index does (merge arm),
// and an index-only row whose metadata file supplies a backend (index-only arm).
func TestT7_EnumerateBackendFill(t *testing.T) {
	home := testHome(t)

	// Scanned-and-indexed: metadata without a backend, index with one.
	def := homeBoruDir(home)
	if err := os.MkdirAll(def, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(def, "vault.jsonic"), []byte(`{"version":4}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordVaultInit(home, def, "", "file"); err != nil {
		t.Fatal(err)
	}

	// Index-only (invalid suffix skips the scan) with metadata that has a backend.
	iso := filepath.Join(home, "iso")
	if err := os.MkdirAll(iso, 0o700); err != nil {
		t.Fatal(err)
	}
	// metaFileName(".") == "vault...jsonic"; suffixFromMetaName rejects ".".
	if err := os.WriteFile(filepath.Join(iso, "vault...jsonic"), []byte(`{"version":4,"backend":"keychain"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordVaultInit(home, iso, ".", ""); err != nil {
		t.Fatal(err)
	}

	vs, err := enumerateVaults(home)
	if err != nil {
		t.Fatal(err)
	}
	var sawMerge, sawIndexOnly bool
	for _, dv := range vs {
		if dv.Folder == filepath.Clean(def) && dv.Suffix == "" && dv.Backend == "file" && dv.InScan {
			sawMerge = true
		}
		if dv.Folder == filepath.Clean(iso) && dv.Suffix == "." && dv.Backend == "keychain" && dv.Exists && !dv.InScan {
			sawIndexOnly = true
		}
	}
	if !sawMerge {
		t.Errorf("scanned row did not inherit the index backend: %+v", vs)
	}
	if !sawIndexOnly {
		t.Errorf("index-only row did not read its metadata backend: %+v", vs)
	}
}

// TestT7_RunVaultsParseErrors drives the flag-parse error arms of the three
// `vault folder` subcommands.
func TestT7_RunVaultsParseErrors(t *testing.T) {
	home := t.TempDir()
	discard := func() (io.Writer, io.Writer) { return &bytes.Buffer{}, &bytes.Buffer{} }
	o, e := discard()
	if code := runVaultsList([]string{"--bogus"}, home, o, e); code == 0 {
		t.Error("runVaultsList with a bad flag should exit non-zero")
	}
	o, e = discard()
	if code := runVaultsAdd([]string{"--bogus"}, home, o, e); code == 0 {
		t.Error("runVaultsAdd with a bad flag should exit non-zero")
	}
	o, e = discard()
	if code := runVaultsRemove([]string{"--bogus"}, home, o, e); code == 0 {
		t.Error("runVaultsRemove with a bad flag should exit non-zero")
	}
}

// TestT7_RunVaultsAddRegisterError drives runVaultsAdd's registerVault error arm
// (a valid vault to register, but a corrupt home index).
func TestT7_RunVaultsAddRegisterError(t *testing.T) {
	home := testHome(t)
	t7writeCorruptIndex(t, home)
	folder := filepath.Join(home, "ext")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "vault.jsonic"), []byte(`{"version":4,"backend":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runVaultsAdd([]string{folder}, home, &out, &errb); code == 0 {
		t.Errorf("runVaultsAdd should fail when the index is corrupt: out=%q err=%q", out.String(), errb.String())
	}
}

// TestT7_RunVaultsRemoveLoadError drives runVaultsRemove's loadVaultIndex error
// arm (a corrupt home index).
func TestT7_RunVaultsRemoveLoadError(t *testing.T) {
	home := testHome(t)
	t7writeCorruptIndex(t, home)
	var out, errb bytes.Buffer
	if code := runVaultsRemove([]string{filepath.Join(home, "whatever")}, home, &out, &errb); code == 0 {
		t.Error("runVaultsRemove should fail on a corrupt index")
	}
}

// TestT7_SaveVaultIndexMarshalError drives saveVaultIndex' MarshalIndent error
// arm AND, through it, runVaultsRemove's saveVaultIndex error arm — one seam
// swap covers both.
func TestT7_SaveVaultIndexMarshalError(t *testing.T) {
	home := testHome(t)
	folder := "/srv/t7vault"
	if err := recordVaultInit(home, folder, "", "file"); err != nil { // real marshal, entry exists
		t.Fatal(err)
	}
	orig := t7jsonMarshalIndent
	t.Cleanup(func() { t7jsonMarshalIndent = orig })
	t7jsonMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("t7 marshal boom")
	}
	// Direct saveVaultIndex hits the MarshalIndent arm.
	if err := saveVaultIndex(home, &vaultIndex{Version: vaultIndexVersion}); err == nil {
		t.Error("saveVaultIndex should surface a marshal error")
	}
	// runVaultsRemove loads OK (Unmarshal), removes, then saveVaultIndex fails.
	var out, errb bytes.Buffer
	if code := runVaultsRemove([]string{folder}, home, &out, &errb); code == 0 {
		t.Error("runVaultsRemove should fail when the index cannot be saved")
	}
}
