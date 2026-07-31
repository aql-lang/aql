package vault

// tui_model_seam8_test.go — wave-8 coverage for the controller/store-driven
// arms of the concrete TUI screens: vault-picker markers/states, capability
// status cells, the secret-detail reveal-error / copy-success paths, the
// grant-error path, password-slot rows, and the restore-form validators. Every
// screen is driven synchronously through the committed pump/collect helpers;
// store state is seeded through the public Store API + SaveStore.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestW8_SecretDetailRevealErrorAndCopySuccess drives secretDetailScreen.Update
// reveal error arm (revealSecret fails for a missing alias) and the copy
// success arm (a fake clipboard whose copy command succeeds).
func TestW8_SecretDetailRevealErrorAndCopySuccess(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()

	// Reveal error: an authed controller runs the reveal, but the alias does
	// not exist so `get --reveal` fails -> statusErr.
	s := m.buildSecretDetail("ghost").(*secretDetailScreen)
	_, cmd := s.Update(keyMsg("r"))
	msgs := collectMsgs(cmd)
	var sawErr bool
	for _, msg := range msgs {
		if st, ok := msg.(setStatusMsg); ok && st.err {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("reveal of a missing alias should emit an error status, got %+v", msgs)
	}

	// Copy success: a fake clipboard whose copy command reads stdin and exits 0.
	orig := t7detectClipboard
	t.Cleanup(func() { t7detectClipboard = orig })
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return &clipboardCmd{label: "w8-fake", copyArgv: []string{"cat"}}, nil
	}
	_, cmd = s.Update(keyMsg("c"))
	msgs = collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("copy should produce one status, got %+v", msgs)
	}
	st, ok := msgs[0].(setStatusMsg)
	if !ok || st.err || !strings.Contains(st.text, "copied to clipboard") {
		t.Errorf("copy success status = %+v, want a non-error 'copied to clipboard'", msgs[0])
	}
}

// TestW8_AuditTextEmptyStdoutFallsBackToStderr drives auditText's empty-stdout
// arm: an unknown audit flag makes the command fail during flag parsing, which
// writes usage to stderr and nothing to stdout, so auditText returns the
// trimmed stderr rather than the (empty) stdout.
func TestW8_AuditTextEmptyStdoutFallsBackToStderr(t *testing.T) {
	ctl, _ := newTestController(t)
	got := ctl.auditText("--w8-bogus-flag")
	if strings.TrimSpace(got) == "" {
		t.Error("auditText with a failing command should return the trimmed stderr, got empty")
	}
}

// TestW8_SecretsEmptyEnterNoDetail drives buildSecrets' enter action on an
// empty table: aliasAt returns "" (out of range) and the action returns nil.
func TestW8_SecretsEmptyEnterNoDetail(t *testing.T) {
	ctl, _ := newTestController(t) // fresh vault: no secrets
	m := newRootModel(ctl)
	m.Init()
	secrets := m.buildSecrets()
	secrets.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, cmd := secrets.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Errorf("enter on an empty secrets table should push nothing, got %v", pushedTitle(cmd))
	}
}

// TestW8_AccessCapStatusCells drives accessTable's revoked and expired status
// cells by seeding a revoked capability and an already-expired one.
func TestW8_AccessCapStatusCells(t *testing.T) {
	ctl, home := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	base := len(s.Capabilities)
	if _, _, err := s.NewCapability("k", "revoked-agent", nil, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.NewCapability("k", "expired-agent", nil, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	s.Capabilities[base].Revoked = true
	s.Capabilities[base+1].ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	m := newRootModel(ctl)
	_, rows := m.accessTable()
	var sawRevoked, sawExpired bool
	for _, r := range rows {
		for _, cell := range r {
			switch cell {
			case "revoked":
				sawRevoked = true
			case "expired":
				sawExpired = true
			}
		}
	}
	if !sawRevoked || !sawExpired {
		t.Errorf("accessTable status cells: revoked=%v expired=%v, rows=%+v", sawRevoked, sawExpired, rows)
	}
}

// TestW8_AccessEmptyRevokeNoForm drives buildAccess' revoke ("D") action on an
// empty table: idAt returns "" and the action returns nil.
func TestW8_AccessEmptyRevokeNoForm(t *testing.T) {
	ctl, _ := newTestController(t) // no capabilities
	m := newRootModel(ctl)
	m.Init()
	access := m.buildAccess()
	access.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, cmd := access.Update(keyMsg("D")); cmd != nil {
		t.Errorf("D on an empty access table should push nothing, got %q", pushedTitle(cmd))
	}
}

// TestW8_GrantFormSubmitError drives buildGrantForm's submit closure error arm:
// granting for a missing alias fails, yielding an opResultMsg carrying the err.
func TestW8_GrantFormSubmitError(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	// Prefill an alias that does not exist so grant() fails at submit.
	fs := m.buildGrantForm("nope").(*formScreen)
	msgs := completeForm(t, fs)
	op, ok := firstOpResult(msgs)
	if !ok || op.err == nil {
		t.Errorf("grant for a missing alias should error: %+v", msgs)
	}
}

// TestW8_PasswordSlotsRowsAndRemove drives passwordsTable's row loop, the
// buildPasswords nameAt hit, and the remove ("D") action pushing the remove
// form — all requiring at least one slot to be present.
func TestW8_PasswordSlotsRowsAndRemove(t *testing.T) {
	ctl, home := newTestController(t)
	s, err := LoadStore(home)
	if err != nil || s == nil {
		t.Fatalf("LoadStore: %v", err)
	}
	s.Passwords = append(s.Passwords, PasswordSlot{
		Name:       "reader",
		Scope:      ScopeRead,
		Namespaces: []string{"proj"},
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	m := newRootModel(ctl)
	// passwordsTable row loop.
	_, rows := m.passwordsTable()
	if len(rows) != 1 || rows[0][0] != "reader" {
		t.Fatalf("passwordsTable rows = %+v, want one 'reader' row", rows)
	}
	// buildPasswords: 'D' on the slot -> nameAt returns "reader" -> remove form.
	pw := m.buildPasswords()
	pw.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, cmd := pw.Update(keyMsg("D"))
	if got := pushedTitle(cmd); got == "" {
		t.Errorf("D on a populated password table should push the remove form, pushed %q", got)
	}
}

// TestW8_VaultItemsMarkersStatesAndSwitchError drives vaultItems' ★ (non-active
// default) marker, MISSING (stale index entry) and locked state cells, plus the
// per-item switch action's error arm for a vault whose store is absent.
func TestW8_VaultItemsMarkersStatesAndSwitchError(t *testing.T) {
	ctl, home := newTestController(t) // active default vault at ~/.boru
	m := newRootModel(ctl)

	// A second, suffixed vault in the same folder, marked default. Since it is
	// not the active vault, its row gets the ★ marker.
	if code, _, e := runVault(t, "", "--suffix=alt", "init", "--backend=file"); code != 0 {
		t.Fatalf("init alt vault: %s", e)
	}
	if err := setDefaultVault(home, vaultFolder(home), "alt"); err != nil {
		t.Fatal(err)
	}
	// Restore the active vault (runVault promoted --suffix into the env).
	ctl.setActiveVault(vaultFolder(home), "")

	// A stale index entry pointing at a folder with no vault -> MISSING.
	idx, err := loadVaultIndex(home)
	if err != nil {
		t.Fatal(err)
	}
	ghostFolder := "/nonexistent/w8-ghost"
	idx.upsert(vaultRef{Folder: ghostFolder, Suffix: "ghost"})
	if err := saveVaultIndex(home, idx); err != nil {
		t.Fatal(err)
	}

	// Lock the active vault so its row shows "locked".
	if err := ctl.lock(); err != nil {
		t.Fatal(err)
	}

	items := m.vaultItems()
	var sawDefault, sawMissing, sawLocked bool
	var ghostAct func() tea.Cmd
	for _, it := range items {
		li := it.(listItem)
		if strings.HasPrefix(li.name, "★ ") {
			sawDefault = true
		}
		if strings.Contains(li.desc, "MISSING") {
			sawMissing = true
			ghostAct = li.act
		}
		if strings.Contains(li.desc, "locked") {
			sawLocked = true
		}
	}
	if !sawDefault || !sawMissing || !sawLocked {
		t.Errorf("vaultItems markers/states: default=%v missing=%v locked=%v", sawDefault, sawMissing, sawLocked)
	}

	// The missing vault's enter action switches to it, which fails (no store)
	// and yields an opResultMsg carrying the error.
	if ghostAct == nil {
		t.Fatal("no MISSING vault row to drive the switch-error arm")
	}
	msg := ghostAct()()
	if op, ok := msg.(opResultMsg); !ok || op.err == nil {
		t.Errorf("switching to a missing vault should error, got %#v", msg)
	}
}

// TestW8_AddFormCancel drives buildAddForm's onSubmit cancel arm: the final
// confirm is toggled to Cancel, so the form completes with submit=false.
func TestW8_AddFormCancel(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	fs := m.buildAddForm().(*formScreen)
	_, seen := pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("ky"),                   // Alias
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Value
		keyMsg("v"),                    // Value
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Provider
		tea.KeyMsg{Type: tea.KeyEnter}, // accept (none) -> Namespace
		tea.KeyMsg{Type: tea.KeyEnter}, // Namespace empty -> Expiry
		tea.KeyMsg{Type: tea.KeyEnter}, // Expiry empty -> Confirm
		tea.KeyMsg{Type: tea.KeyLeft},  // toggle Submit -> Cancel
		tea.KeyMsg{Type: tea.KeyEnter}, // submit (completed with submit=false)
	)
	var sawCancelled bool
	for _, msg := range seen {
		if st, ok := msg.(setStatusMsg); ok && strings.Contains(st.text, "cancelled") {
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Errorf("cancel should set a 'cancelled' status; msgs=%+v", seen)
	}
	// The secret must not have been stored.
	if _, err := ctl.revealSecret("ky"); err == nil {
		t.Error("cancelled add must not store the secret")
	}
}

// TestW8_RestoreFormValidators drives buildRestoreForm's Generation validator
// (non-numeric -> error) and the confirm validator (mismatch -> error).
func TestW8_RestoreFormValidators(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)

	// Non-numeric generation: the Generation field validator rejects it.
	fs := m.buildRestoreForm().(*formScreen)
	pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("x"),
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	// Mismatched confirmation: a valid generation, then a different confirm
	// value, so the confirm field validator rejects it.
	fs2 := m.buildRestoreForm().(*formScreen)
	pumpScreen(t, fs2,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("5"),
		tea.KeyMsg{Type: tea.KeyEnter},
		keyMsg("6"),
		tea.KeyMsg{Type: tea.KeyEnter},
	)
}
