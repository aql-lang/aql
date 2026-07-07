package vault

import (
	"testing"
)

// TestT7_ControllerUninitErrors drives every controller read/mutation against a
// home with no initialized vault, exercising the requireStore/run error arms.
func TestT7_ControllerUninitErrors(t *testing.T) {
	ctl := t7uninitCtl(t)

	// requireStore-backed reads: error arms.
	if _, err := ctl.listAliases(); err == nil {
		t.Error("listAliases on uninitialized vault should error")
	}
	if _, ok := ctl.alias("x"); ok {
		t.Error("alias on uninitialized vault should report not-found")
	}
	if _, _, err := ctl.listCapabilities(); err == nil {
		t.Error("listCapabilities on uninitialized vault should error")
	}
	if _, err := ctl.listPasswordSlots(); err == nil {
		t.Error("listPasswordSlots on uninitialized vault should error")
	}
	// locked(): LoadStore returns (nil,nil) for a missing store -> (false,nil).
	if locked, err := ctl.locked(); locked || err != nil {
		t.Errorf("locked() on missing store = (%v,%v), want (false,nil)", locked, err)
	}
	// authenticate(): requireStore error arm.
	if err := ctl.authenticate("whatever"); err == nil {
		t.Error("authenticate on uninitialized vault should error")
	}

	// run* mutation error arms.
	if err := ctl.removeSecret("k"); err == nil {
		t.Error("removeSecret on uninitialized vault should error")
	}
	if err := ctl.lock(); err == nil {
		t.Error("lock on uninitialized vault should error")
	}
	if err := ctl.unlock(); err == nil {
		t.Error("unlock on uninitialized vault should error")
	}
	if err := ctl.configSet("namespace.default", "x"); err == nil {
		t.Error("configSet on uninitialized vault should error")
	}
	if err := ctl.configUnset("namespace.default"); err == nil {
		t.Error("configUnset on uninitialized vault should error")
	}

	// addSecret with namespace + expiry set: exercises both optional-flag arms
	// before the run itself fails.
	if err := ctl.addSecret("a", "v", "", "proj", "90d"); err == nil {
		t.Error("addSecret on uninitialized vault should error")
	}
	// rotateSecret with revoke-caps + expiry set: both optional-flag arms.
	if err := ctl.rotateSecret("a", "v", true, "90d"); err == nil {
		t.Error("rotateSecret on uninitialized vault should error")
	}
	// grant with every optional flag set: all flag arms + the run error arm.
	if _, err := ctl.grant("a", "ci", "api.example", "GET,POST", "2h", 5, 10, true); err == nil {
		t.Error("grant on uninitialized vault should error")
	}

	// informational panels: the non-zero / empty-output arms.
	_ = ctl.historyText() // code != 0 -> out+errb
	_ = ctl.auditText()   // empty stdout -> trimmed stderr
}

// TestT7_ControllerCorruptStore drives the LoadStore error arms (a metadata
// file that exists but does not parse).
func TestT7_ControllerCorruptStore(t *testing.T) {
	ctl := t7corruptStoreCtl(t)
	if _, ok, err := ctl.status(); ok || err == nil {
		t.Errorf("status on corrupt store = (ok=%v,err=%v), want (false, error)", ok, err)
	}
	if err := ctl.switchVault(ctl.folder, ""); err == nil {
		t.Error("switchVault into a corrupt store should surface the load error")
	}
}

// TestT7_ControllerAuthEmptyPass drives authenticate's authenticateWith error
// arm: a real file-backend store rejects an empty passphrase.
func TestT7_ControllerAuthEmptyPass(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.authenticate(""); err == nil {
		t.Error("authenticate with an empty passphrase should error (file backend)")
	}
}

// TestT7_ControllerClipboard drives copyToClipboard's copy-success and
// copy-error arms via the t7detectClipboard seam (no real OS clipboard needed).
func TestT7_ControllerClipboard(t *testing.T) {
	ctl, _ := newTestController(t)

	// Success: a fake clipboard whose copy command reads stdin and exits 0.
	orig := t7detectClipboard
	t.Cleanup(func() { t7detectClipboard = orig })
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return &clipboardCmd{label: "t7-fake", copyArgv: []string{"cat"}}, nil
	}
	label, err := ctl.copyToClipboard("hello")
	if err != nil || label != "t7-fake" {
		t.Fatalf("copyToClipboard success = (%q,%v), want (t7-fake,nil)", label, err)
	}

	// Copy error: a fake clipboard with no copy command supported.
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return &clipboardCmd{label: "t7-nocopy"}, nil
	}
	if _, err := ctl.copyToClipboard("hello"); err == nil {
		t.Error("copyToClipboard with an unsupported copy command should error")
	}
}
