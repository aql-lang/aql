package vault

// tui_build_seam8_test.go — wave-8 coverage for the residual tui_build.go /
// tui_screens.go arms: pure preview/format helpers, the reload/cmd closures
// that only run when a built screen is reloaded or its command preview is
// read, and the generic-screen empty-selection guards. Every screen is driven
// synchronously (no real terminal) via the committed pump/collect helpers.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// pushedScreenW8 runs a pushScreen command and returns the pushed screen.
func pushedScreenW8(t *testing.T, cmd tea.Cmd) screen {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil command, expected a pushMsg")
	}
	pm, ok := cmd().(pushMsg)
	if !ok {
		t.Fatalf("command did not push a screen")
	}
	return pm.s
}

// TestW8_ExpiryCountdownUnparseable drives expiryCountdown's time.Parse error
// arm: a non-empty but unparseable timestamp yields "".
func TestW8_ExpiryCountdownUnparseable(t *testing.T) {
	if got := expiryCountdown("not-a-timestamp"); got != "" {
		t.Errorf("expiryCountdown(garbage) = %q, want empty", got)
	}
}

// TestW8_ExpiryCellFutureAndUnset drives expiryCell's non-empty countdown arm
// (a real future timestamp) and its "-" fallback (unset).
func TestW8_ExpiryCellFutureAndUnset(t *testing.T) {
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	if got := expiryCell(future); got == "-" || got == "" {
		t.Errorf("expiryCell(future) = %q, want a countdown", got)
	}
	if got := expiryCell(""); got != "-" {
		t.Errorf("expiryCell(unset) = %q, want -", got)
	}
}

// TestW8_RenameCommandPreviewRevoke drives renameCommandPreview's revoke arm.
func TestW8_RenameCommandPreviewRevoke(t *testing.T) {
	got := renameCommandPreview("old", "new", true)
	if !strings.Contains(got, "--revoke-caps") {
		t.Errorf("renameCommandPreview(revoke) = %q, want --revoke-caps", got)
	}
}

// TestW8_GrantCommandPreviewMaxCost drives grantCommandPreview's max-cost arm
// (a positive cents value appends --max-cost-cents).
func TestW8_GrantCommandPreviewMaxCost(t *testing.T) {
	got := grantCommandPreview("k", "", "", "", "", "0", "500", false)
	if !strings.Contains(got, "--max-cost-cents=500") {
		t.Errorf("grantCommandPreview(maxCost) = %q, want --max-cost-cents=500", got)
	}
}

// TestW8_SecretDetailBodyShowsExpiryCountdown drives secretDetailBody's
// countdown arm: an alias with a parseable ExpiresAt renders "(<countdown>)".
func TestW8_SecretDetailBodyShowsExpiryCountdown(t *testing.T) {
	m := newTestModel(t)
	if err := m.ctl.addSecret("tok", "v", "", "", "90d"); err != nil {
		t.Fatalf("addSecret: %v", err)
	}
	a, ok := m.ctl.alias("tok")
	if !ok || a.ExpiresAt == "" {
		t.Fatalf("alias tok has no expiry: %+v", a)
	}
	cd := expiryCountdown(a.ExpiresAt)
	if cd == "" {
		t.Fatalf("expiryCountdown for %q was empty", a.ExpiresAt)
	}
	body := m.secretDetailBody("tok", false, "", nil, 0)
	if !strings.Contains(body, "("+cd+")") {
		t.Errorf("detail body missing countdown %q:\n%s", "("+cd+")", body)
	}
}

// TestW8_PagerReloadClosures drives the reload closures of the verify, history
// and config pagers — they only run when the built pager is reloaded.
func TestW8_PagerReloadClosures(t *testing.T) {
	m := newTestModel(t)
	for name, s := range map[string]screen{
		"verify":  m.buildVerifyPager(),
		"history": m.buildHistoryPager(),
		"config":  m.buildConfigPager(),
	} {
		if cmd := s.reload(); cmd != nil {
			t.Errorf("%s pager reload should return no command", name)
		}
	}
}

// TestW8_AuditPagerReloadClosures drives the audit pager reload closures built
// inside the Maintenance menu action and the ':' palette (both re-read
// auditText on reload).
func TestW8_AuditPagerReloadClosures(t *testing.T) {
	m := newTestModel(t)

	// Maintenance -> Audit item act builds the audit pager.
	acts := listActs(t, m.buildMaintenance())
	auditAct := acts["Audit"]
	if auditAct == nil {
		t.Fatal("maintenance has no Audit item")
	}
	pager := pushedScreenW8(t, auditAct())
	if pager.Title() != "audit" {
		t.Fatalf("Audit item pushed %q, want audit", pager.Title())
	}
	pager.reload() // runs func() string { return m.ctl.auditText() }

	// The ':' palette "audit" command builds the same pager.
	p2 := pushedScreenW8(t, m.runPalette("audit"))
	if p2.Title() != "audit" {
		t.Fatalf("palette audit pushed %q, want audit", p2.Title())
	}
	p2.reload()
}

// TestW8_ConfigUnsetPreview drives buildConfigUnsetForm's cmdFn closure.
func TestW8_ConfigUnsetPreview(t *testing.T) {
	m := newTestModel(t)
	fs := m.buildConfigUnsetForm().(*formScreen)
	if got := fs.cliCommand(); !strings.Contains(got, "config --unset") {
		t.Errorf("config unset preview = %q, want to contain 'config --unset'", got)
	}
}

// TestW8_SettingsLockAndThemeActs drives the Settings menu's Lock/Unlock and
// Theme item act closures (their bodies call toggleLock / cycleTheme).
func TestW8_SettingsLockAndThemeActs(t *testing.T) {
	m := newTestModel(t)
	acts := listActs(t, m.buildSettings())
	if acts["Lock / Unlock"] == nil || acts["Theme"] == nil {
		t.Fatalf("settings missing Lock/Unlock or Theme item: %v", acts)
	}
	// Lock/Unlock -> toggleLock: on an unlocked vault this yields a "vault
	// locked" status once the op runs.
	if msgs := collectMsgs(acts["Lock / Unlock"]()); len(msgs) == 0 {
		t.Error("Lock/Unlock act produced no messages")
	}
	// Theme -> cycleTheme yields a theme status message.
	if msgs := collectMsgs(acts["Theme"]()); len(msgs) == 0 {
		t.Error("Theme act produced no messages")
	}
}

// TestW8_ToggleLockError drives toggleLock's error arm: locked() fails when the
// active store metadata is unparseable.
func TestW8_ToggleLockError(t *testing.T) {
	ctl := t7corruptStoreCtl(t)
	m := newRootModel(ctl)
	msgs := collectMsgs(m.toggleLock())
	var sawErr bool
	for _, msg := range msgs {
		if st, ok := msg.(setStatusMsg); ok && st.err {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("toggleLock on a corrupt store should emit an error status, got %+v", msgs)
	}
}

// notAListItemW8 is a list.Item that is not a listItem, to drive
// arrowDelegate.Render's type-assert failure arm.
type notAListItemW8 struct{}

func (notAListItemW8) FilterValue() string { return "" }

// TestW8_ArrowDelegateNonListItem drives arrowDelegate.Render's non-listItem
// early return (it writes nothing).
func TestW8_ArrowDelegateNonListItem(t *testing.T) {
	var buf bytes.Buffer
	lm := list.New(nil, arrowDelegate{}, 10, 4)
	arrowDelegate{}.Render(&buf, lm, 0, notAListItemW8{})
	if buf.Len() != 0 {
		t.Errorf("Render of a non-listItem should write nothing, got %q", buf.String())
	}
}

// TestW8_ListScreenEmptySelectionGuards drives listScreen.Update's two
// no-selection guards: enter and an action key on a screen with no items.
func TestW8_ListScreenEmptySelectionGuards(t *testing.T) {
	act := listKeyAction{binding: kNewVault, run: func(listItem) tea.Cmd {
		t.Fatal("action must not run when nothing is selected")
		return nil
	}}
	s := newListScreen("empty", nil, []listKeyAction{act}, nil)
	s.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	// Enter with no items -> selected() reports !ok -> return s, nil.
	if _, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("enter on an empty list should return no command")
	}
	// Action key with no items -> selected() reports !ok -> return s, nil.
	if _, cmd := s.Update(keyMsg("n")); cmd != nil {
		t.Error("action key on an empty list should return no command")
	}
}
