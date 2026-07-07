package vault

// store_seam7_test.go — migration, LoadStore/SaveStore, alias/capability,
// and entropy-failure arms in store.go, driven via disk fixtures and the
// createTemp / s7randRead seams (design/TEST-SEAMS.10.md).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestS7_MigrateStoreZeroVersion(t *testing.T) {
	s := &Store{Version: 0}
	if err := migrateStore(s); err != nil {
		t.Fatal(err)
	}
	if s.Version != storeVersion {
		t.Errorf("migrateStore(v0) → Version = %d, want %d", s.Version, storeVersion)
	}
}

func TestS7_MigrateStoreMigrationError(t *testing.T) {
	old := storeMigrations[0]
	storeMigrations[0] = func(*Store) error { return errS7Boom }
	t.Cleanup(func() { storeMigrations[0] = old })
	err := migrateStore(&Store{Version: 1})
	if err == nil || !strings.Contains(err.Error(), "migrating store") {
		t.Fatalf("migrateStore err = %v, want a wrapped migration failure", err)
	}
}

func TestS7_LoadStoreTooManySlots(t *testing.T) {
	home := testHome(t)
	if err := os.MkdirAll(vaultFolder(home), 0700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"version":4,"passwords":[`)
	for i := 0; i <= maxPasswordSlots; i++ { // maxPasswordSlots+1 slots
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":"s%d"}`, i)
	}
	b.WriteString(`]}`)
	if err := os.WriteFile(StorePath(home), []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStore(home); err == nil || !strings.Contains(err.Error(), "exceeding the maximum") {
		t.Fatalf("LoadStore err = %v, want too-many-slots", err)
	}
}

func TestS7_SaveStoreSetsVersion(t *testing.T) {
	home := testHome(t)
	s := &Store{Backend: BackendFile} // Version 0
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	if s.Version != storeVersion {
		t.Errorf("SaveStore left Version = %d, want %d", s.Version, storeVersion)
	}
	got, err := LoadStore(home)
	if err != nil || got == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if got.Version != storeVersion {
		t.Errorf("persisted version = %d, want %d", got.Version, storeVersion)
	}
}

func TestS7_SaveStoreMkdirError(t *testing.T) {
	home := testHome(t)
	blocker := filepath.Join(home, "s7blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvFolder, filepath.Join(blocker, "sub"))
	if err := SaveStore(home, &Store{Version: storeVersion, Backend: BackendFile}); err == nil {
		t.Fatal("expected MkdirAll error saving under a file")
	}
}

func TestS7_SaveStoreLegacyMarshalError(t *testing.T) {
	home := testHome(t)
	s := &Store{Version: storeVersion, Backend: BackendFile, Config: map[string]any{"c": make(chan int)}}
	if err := SaveStore(home, s); err == nil {
		t.Fatal("expected marshal error for an unmarshalable Config (legacy path)")
	}
}

func TestS7_SaveStoreEnvelopeMarshalError(t *testing.T) {
	home := testHome(t)
	s := &Store{
		Version:   storeVersion,
		Backend:   BackendFile,
		Passwords: []PasswordSlot{{Name: "s7admin", Scope: ScopeAdmin}},
		Config:    map[string]any{"c": make(chan int)},
	}
	if err := SaveStore(home, s); err == nil {
		t.Fatal("expected marshal error for an unmarshalable Config (envelope path)")
	}
}

func TestS7_SaveStoreEnvelopeWriteError(t *testing.T) {
	home := testHome(t)
	old := createTemp
	createTemp = func(string, string) (*os.File, error) { return nil, errS7Boom }
	t.Cleanup(func() { createTemp = old })
	s := &Store{
		Version:   storeVersion,
		Backend:   BackendFile,
		Passwords: []PasswordSlot{{Name: "s7admin", Scope: ScopeAdmin}},
	}
	if err := SaveStore(home, s); !errors.Is(err, errS7Boom) {
		t.Fatalf("SaveStore envelope write arm err = %v, want boom", err)
	}
}

func TestS7_UpsertAliasUpdatesExpiry(t *testing.T) {
	s := &Store{}
	s.UpsertAlias(Alias{Name: "k"})
	s.UpsertAlias(Alias{Name: "k", ExpiresAt: "2030-01-02T03:04:05Z"})
	a, _ := s.FindAlias("k")
	if a == nil || a.ExpiresAt != "2030-01-02T03:04:05Z" {
		t.Fatalf("expiry not updated on existing alias: %+v", a)
	}
}

func TestS7_RenameAliasMissing(t *testing.T) {
	s := &Store{}
	if n, ok := s.RenameAlias("nope", "x", false); ok || n != 0 {
		t.Fatalf("RenameAlias(missing) = %d, %v; want 0, false", n, ok)
	}
}

func TestS7_FindCapabilityExactNoMatch(t *testing.T) {
	s := &Store{Capabilities: []Capability{{ID: "abc"}}}
	if c, i := s.FindCapabilityExact("nope"); c != nil || i != -1 {
		t.Fatalf("FindCapabilityExact(missing) = %v, %d; want nil, -1", c, i)
	}
}

func TestS7_RecordCapabilityUseMissing(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if err := recordCapabilityUse(home, "s7-no-such-cap", 5); err != nil {
		t.Fatalf("recordCapabilityUse(missing) = %v, want nil (no-op)", err)
	}
}

func TestS7_RandomIDEntropyError(t *testing.T) {
	old := s7randRead
	s7randRead = func([]byte) (int, error) { return 0, errS7Boom }
	t.Cleanup(func() { s7randRead = old })
	s := &Store{}
	if _, _, err := s.NewCapability("a", "agent", nil, nil, 0); !errors.Is(err, errS7Boom) {
		t.Fatalf("NewCapability id-entropy err = %v, want boom", err)
	}
}

func TestS7_RandomTokenEntropyError(t *testing.T) {
	old := s7randRead
	calls := 0
	s7randRead = func(b []byte) (int, error) {
		calls++
		if calls == 1 {
			return len(b), nil // randomID succeeds
		}
		return 0, errS7Boom // randomToken fails
	}
	t.Cleanup(func() { s7randRead = old })
	s := &Store{}
	if _, _, err := s.NewCapability("a", "agent", nil, nil, 0); !errors.Is(err, errS7Boom) {
		t.Fatalf("NewCapability token-entropy err = %v, want boom", err)
	}
}
