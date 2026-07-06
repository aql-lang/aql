// Wave-4 coverage for the serve package tails: the config loader's
// invalid-jsonic arm and the factory error arms (registry folder
// missing, vault-proxy without an initialized vault).
package serve

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestW4LoadConfigInvalidJsonic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "svc.jsonic")
	if err := os.WriteFile(path, []byte("{{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "invalid jsonic") {
		t.Errorf("loadConfig = %v, want invalid-jsonic error", err)
	}
}

func TestW4RegistryFactoryFolderMissing(t *testing.T) {
	_, err := registryFactory([]string{"-r", filepath.Join(t.TempDir(), "zz-missing")}, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("registryFactory = %v, want folder-not-found error", err)
	}
}

func TestW4VaultProxyFactoryUninitialized(t *testing.T) {
	// A home dir with no vault: NewProxyService fails and the factory
	// strips the "vault-proxy: " prefix.
	home := t.TempDir()
	_, err := vaultProxyFactory([]string{"-listen", "127.0.0.1:0", "-home", home}, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("vaultProxyFactory with no vault succeeded, want error")
	}
	if strings.HasPrefix(err.Error(), "vault-proxy: ") {
		t.Errorf("error prefix not stripped: %q", err.Error())
	}
}

func TestW4VaultProxyFactoryBadFlag(t *testing.T) {
	if _, err := vaultProxyFactory([]string{"-zz-bad"}, nil, io.Discard, io.Discard); err == nil {
		t.Error("vaultProxyFactory(bad flag) succeeded, want error")
	}
}
