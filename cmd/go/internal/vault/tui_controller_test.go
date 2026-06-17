package vault

import (
	"path/filepath"
	"strings"
	"testing"
)

// newTestController returns a controller authenticated against a fresh
// file-backend vault at the default location.
func newTestController(t *testing.T) (*tuiController, string) {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	ctl := newTUIController(home)
	ctl.setActiveVault(vaultFolder(home), "")
	if err := ctl.authenticate("test-pass"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return ctl, home
}

func TestControllerAddRevealRemove(t *testing.T) {
	ctl, home := newTestController(t)

	if err := ctl.addSecret("github_token", "ghp-secret", "github", "", ""); err != nil {
		t.Fatalf("addSecret: %v", err)
	}
	aliases, err := ctl.listAliases()
	if err != nil || len(aliases) != 1 || aliases[0].Name != "github_token" {
		t.Fatalf("listAliases after add = %+v, %v", aliases, err)
	}
	if aliases[0].Provider != "github" {
		t.Errorf("provider not recorded: %+v", aliases[0])
	}

	// The controller must drive the real path: the audit log records it.
	events, _ := ReadAudit(home)
	if !hasAudit(events, "vault.add", "github_token") {
		t.Errorf("expected a vault.add audit event for github_token")
	}

	got, err := ctl.revealSecret("github_token")
	if err != nil || got != "ghp-secret" {
		t.Fatalf("revealSecret = %q, %v; want ghp-secret", got, err)
	}

	// Negative: revealing a missing alias errors rather than returning "".
	if _, err := ctl.revealSecret("nope"); err == nil {
		t.Errorf("revealSecret(missing) should error")
	}

	if err := ctl.removeSecret("github_token"); err != nil {
		t.Fatalf("removeSecret: %v", err)
	}
	aliases, _ = ctl.listAliases()
	if len(aliases) != 0 {
		t.Errorf("alias not removed: %+v", aliases)
	}
}

func TestControllerRotate(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v1", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := ctl.rotateSecret("k", "v2", false, ""); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	got, _ := ctl.revealSecret("k")
	if got != "v2" {
		t.Errorf("after rotate value = %q, want v2", got)
	}
	// Negative: rotating a missing alias fails.
	if err := ctl.rotateSecret("missing", "x", false, ""); err == nil {
		t.Errorf("rotate(missing) should error")
	}
}

func TestControllerGrantRevoke(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	out, err := ctl.grant("k", "agent1", "", "", "2h", 0, 0, false)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if !strings.Contains(out, "token:") {
		t.Errorf("grant output missing one-time token: %q", out)
	}
	caps, active, _ := ctl.listCapabilities()
	if len(caps) != 1 || !active[caps[0].ID] {
		t.Fatalf("expected 1 active capability, got %+v", caps)
	}
	if err := ctl.revoke(caps[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, active, _ = ctl.listCapabilities()
	if active[caps[0].ID] {
		t.Errorf("capability still active after revoke")
	}
	// Negative: revoking an unknown id errors.
	if err := ctl.revoke("deadbeef"); err == nil {
		t.Errorf("revoke(unknown) should error")
	}
}

func TestControllerLockUnlockAndConfig(t *testing.T) {
	ctl, _ := newTestController(t)

	if err := ctl.lock(); err != nil {
		t.Fatal(err)
	}
	if locked, _ := ctl.locked(); !locked {
		t.Errorf("vault should be locked")
	}
	if err := ctl.unlock(); err != nil {
		t.Fatal(err)
	}
	if locked, _ := ctl.locked(); locked {
		t.Errorf("vault should be unlocked")
	}

	if err := ctl.configSet("namespace.default", "proj"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctl.configText(), "namespace.default=proj") {
		t.Errorf("config not set: %q", ctl.configText())
	}
	if err := ctl.configUnset("namespace.default"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ctl.configText(), "namespace.default") {
		t.Errorf("config not unset: %q", ctl.configText())
	}
}

func TestControllerCreateAndSwitchVault(t *testing.T) {
	ctl, home := newTestController(t)
	if err := ctl.addSecret("indefault", "x", "", "", ""); err != nil {
		t.Fatal(err)
	}

	// Create a second, suffixed vault and switch to it.
	if err := ctl.createVault(vaultFolder(home), "alt", BackendFile, "altpass"); err != nil {
		t.Fatalf("createVault: %v", err)
	}
	if ctl.suffix != "alt" {
		t.Fatalf("active suffix = %q, want alt", ctl.suffix)
	}
	st, ok, _ := ctl.status()
	if !ok || st.Aliases != 0 {
		t.Errorf("new vault should be empty, got ok=%v aliases=%d", ok, st.Aliases)
	}

	// The new vault is discoverable and recorded in the index.
	vs, _ := ctl.listVaults()
	if !containsVault(vs, vaultFolder(home), "alt") {
		t.Errorf("alt vault not enumerated: %+v", vs)
	}

	// Switch back to the default vault; its secret is still there.
	if err := ctl.switchVault(vaultFolder(home), ""); err != nil {
		t.Fatalf("switchVault: %v", err)
	}
	if err := ctl.authenticate("test-pass"); err != nil {
		t.Fatalf("re-auth default: %v", err)
	}
	aliases, _ := ctl.listAliases()
	if len(aliases) != 1 || aliases[0].Name != "indefault" {
		t.Errorf("default vault aliases = %+v, want [indefault]", aliases)
	}

	// Switching to a non-existent vault errors.
	if err := ctl.switchVault(filepath.Join(home, "nope"), ""); err == nil {
		t.Errorf("switchVault to missing vault should error")
	}
}

// --- helpers ---------------------------------------------------------------

func hasAudit(events []AuditEvent, action, alias string) bool {
	for _, e := range events {
		if e.Action == action && e.Alias == alias {
			return true
		}
	}
	return false
}

func containsVault(vs []discoveredVault, folder, suffix string) bool {
	folder = filepath.Clean(folder)
	for _, dv := range vs {
		if dv.Folder == folder && dv.Suffix == suffix {
			return true
		}
	}
	return false
}
