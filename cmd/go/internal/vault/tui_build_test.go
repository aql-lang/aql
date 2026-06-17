package vault

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pushed runs a command and returns the pushed screen's title, or "".
func pushedTitle(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	if pm, ok := cmd().(pushMsg); ok {
		return pm.s.Title()
	}
	return ""
}

func TestSecretsAddAuthGating(t *testing.T) {
	// Authenticated file vault: 'a' opens the add form even on an empty table
	// (the old empty-table guard used to block this).
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	secrets := m.buildSecrets()
	_, cmd := secrets.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "add" {
		t.Errorf("authed add should open the add form, pushed %q", got)
	}
}

func TestSecretsAddRequiresUnlock(t *testing.T) {
	// Unauthenticated file vault: 'a' must prompt to unlock first, NOT fail at
	// submit time with "no input".
	home := testHome(t)
	mustInit(t)
	ctl := newTUIController(home)
	ctl.setActiveVault(vaultFolder(home), "") // not authenticated
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	secrets := m.buildSecrets()
	_, cmd := secrets.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "unlock" {
		t.Errorf("unauthed add should prompt to unlock, pushed %q", got)
	}
}

func TestAddCommandPreview(t *testing.T) {
	got := addCommandPreview("proj:k", "github", "proj", "90d")
	for _, want := range []string{"aql vault add", "--provider=github", "--namespace=proj", "--expiry=90d", "proj:k"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview %q missing %q", got, want)
		}
	}
	empty := addCommandPreview("", "", "", "")
	if strings.Contains(empty, "--provider") {
		t.Errorf("empty preview should carry no flags: %q", empty)
	}
	if !strings.Contains(empty, "<alias>") {
		t.Errorf("empty preview should show the <alias> placeholder: %q", empty)
	}
}

func TestAddFormValidators(t *testing.T) {
	// alias
	if validateAliasField("") == nil {
		t.Error("empty alias should be invalid")
	}
	if validateAliasField("good_name") != nil || validateAliasField("ns:name") != nil {
		t.Error("valid aliases rejected")
	}
	if validateAliasField("bad name") == nil {
		t.Error("alias with a space should be invalid")
	}
	// expiry (optional)
	if validateExpiryField("") != nil || validateExpiryField("90d") != nil {
		t.Error("empty/duration expiry should be valid")
	}
	if validateExpiryField("not-a-date") == nil {
		t.Error("garbage expiry should be invalid")
	}
	// namespace (optional)
	if validateNamespaceField("") != nil || validateNamespaceField("proj") != nil {
		t.Error("empty/valid namespace rejected")
	}
	if validateNamespaceField("bad ns") == nil {
		t.Error("namespace with a space should be invalid")
	}
}

func TestCommandBarAndStatusBar(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = feed(t, m, pushMsg{m.buildSecrets()})

	if cb := m.commandBar(); !strings.Contains(cb, "aql vault list") {
		t.Errorf("command bar should show the CLI equivalent: %q", cb)
	}
	sb := m.statusBar()
	if !strings.Contains(sb, "secrets") || !strings.Contains(sb, "1 item") {
		t.Errorf("status bar should show screen + count: %q", sb)
	}
}
