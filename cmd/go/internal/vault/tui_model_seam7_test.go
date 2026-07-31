package vault

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// TestT7_ModelStackHelpersEmpty drives the empty-stack guards of top,
// updateTop, resizeTop, bodySize, and the reloadTopMsg/reloadTop cmd.
func TestT7_ModelStackHelpersEmpty(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl) // Init NOT called -> empty stack

	if m.top() != nil {
		t.Error("top() on an empty stack should be nil")
	}
	if cmd := m.updateTop(nil); cmd != nil {
		t.Error("updateTop() on an empty stack should be nil")
	}
	if cmd := m.resizeTop(); cmd != nil {
		t.Error("resizeTop() on an empty stack should be nil")
	}
	// reloadTopMsg with a nil top -> (m, nil).
	if _, cmd := m.Update(reloadTopMsg{}); cmd != nil {
		t.Error("reloadTopMsg with a nil top should return no command")
	}
	// bodySize clamps to at least one line.
	m.height = 3
	if _, h := m.bodySize(); h != 1 {
		t.Errorf("bodySize height = %d, want clamped to 1", h)
	}
	// reloadTop()'s inner closure yields a reloadTopMsg.
	if _, ok := reloadTop()().(reloadTopMsg); !ok {
		t.Error("reloadTop() should produce a reloadTopMsg")
	}
}

// TestT7_ModelInitOpensPickerWhenNoVault drives Init's else (picker) arm.
func TestT7_ModelInitOpensPickerWhenNoVault(t *testing.T) {
	ctl := t7uninitCtl(t)
	m := newRootModel(ctl)
	m.Init()
	if got := m.top().Title(); got != "vaults" {
		t.Errorf("no vault should open the picker, got %q", got)
	}
}

// TestT7_ModelViewEarlyReturns drives View's quitting and width==0 arms.
func TestT7_ModelViewEarlyReturns(t *testing.T) {
	ctl, _ := newTestController(t)
	// width==0 -> loading.
	m := newRootModel(ctl)
	m.Init()
	if v := m.View(); v != "loading…" {
		t.Errorf("zero-width view = %q, want loading…", v)
	}
	// quitting -> empty.
	m2 := newTestModel(t)
	m2.quitting = true
	if v := m2.View(); v != "" {
		t.Errorf("quitting view = %q, want empty", v)
	}
}

// TestT7_ModelWindowSizeWithHelpOpen drives the resize arm that also sizes the
// help viewport.
func TestT7_ModelWindowSizeWithHelpOpen(t *testing.T) {
	m := newTestModel(t)
	m.openHelp()
	m = feed(t, m, tea.WindowSizeMsg{Width: 123, Height: 40})
	if m.helpVP.Width != 123 {
		t.Errorf("help viewport width = %d, want 123", m.helpVP.Width)
	}
}

// TestT7_ModelHandleKeyBranches drives handleKey's capturesInput, ctrl+c,
// deep-quit, and unmatched-key arms.
func TestT7_ModelHandleKeyBranches(t *testing.T) {
	// capturesInput: a form on top forwards keys.
	m := newTestModel(t)
	m = feed(t, m, pushMsg{m.buildAddForm()})
	if !m.top().capturesInput() {
		t.Fatal("add form should capture input")
	}
	m = feed(t, m, keyMsg("x")) // routed to the form (capturesInput arm)
	if len(m.stack) != 2 {
		t.Errorf("key to a capturing form should not change the stack: depth %d", len(m.stack))
	}

	// ctrl+c on a normal screen quits.
	m2 := newTestModel(t)
	nm, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !nm.(*rootModel).quitting || cmd == nil {
		t.Error("ctrl+c on a normal screen should quit")
	}

	// 'q' at depth > 1 forwards to the top screen instead of quitting.
	m3 := newTestModel(t)
	m3 = feed(t, m3, pushMsg{m3.buildSecrets()})
	m3 = feed(t, m3, keyMsg("q"))
	if m3.quitting {
		t.Error("q below the root should not quit")
	}
	if len(m3.stack) != 2 {
		t.Errorf("q below the root should not change depth: %d", len(m3.stack))
	}

	// An unmatched key falls through to the top screen.
	m4 := newTestModel(t)
	m4 = feed(t, m4, keyMsg("z"))
	if len(m4.stack) != 1 {
		t.Errorf("unmatched key should be a no-op at home: depth %d", len(m4.stack))
	}
}

// TestT7_ModelEscClearsAppliedFilter drives handleKey's keyBack arm that clears
// a committed list filter instead of treating esc as "back".
func TestT7_ModelEscClearsAppliedFilter(t *testing.T) {
	m := newTestModel(t)
	// Start filtering, type a query that matches an item, apply it.
	m = feed(t, m, keyMsg("/"))
	m = feed(t, m, keyMsg("S")) // matches "Secrets"/"Settings"
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	ls, ok := m.top().(*listScreen)
	if !ok || !ls.hasAppliedFilter() {
		t.Fatalf("expected a listScreen with an applied filter, got %T (applied=%v)", m.top(), ok && ls.hasAppliedFilter())
	}
	depth := len(m.stack)
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // clears the filter, does not pop
	if len(m.stack) != depth {
		t.Errorf("esc on an applied filter should not pop: depth %d -> %d", depth, len(m.stack))
	}
}

// TestT7_ModelLockedHeaderAndStatus drives the Locked arms of vaultLine and
// statusBar.
func TestT7_ModelLockedHeaderAndStatus(t *testing.T) {
	m := newTestModel(t)
	if err := m.ctl.lock(); err != nil {
		t.Fatal(err)
	}
	m.refreshVault()
	if !strings.Contains(m.vaultLine(), "LOCKED") {
		t.Errorf("vaultLine should show LOCKED: %q", m.vaultLine())
	}
	if !strings.Contains(m.statusBar(), "locked") {
		t.Errorf("statusBar should show locked: %q", m.statusBar())
	}
}

// TestT7_ModelExecPrefixNonDefault drives execPrefix's location-flags arm.
func TestT7_ModelExecPrefixNonDefault(t *testing.T) {
	m := newTestModel(t)
	m.ctl.folder = filepath.Join(m.ctl.homeDir, ".vxgboru")
	m.ctl.suffix = "sdk"
	if got := m.execPrefix(); !strings.HasPrefix(got, "boru vault --folder=") {
		t.Errorf("execPrefix for a non-default vault = %q", got)
	}
}

// TestT7_ModelRenderKeyListSkipsEmpty drives renderKeyList's empty-key skip.
func TestT7_ModelRenderKeyListSkipsEmpty(t *testing.T) {
	out := renderKeyList([]key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "act")),
		key.NewBinding(), // no help -> skipped
	})
	if !strings.Contains(out, "act") {
		t.Errorf("renderKeyList dropped the described binding: %q", out)
	}
}

// TestT7_ModelPadBodyTruncates drives padBody's over-length truncation arm.
func TestT7_ModelPadBodyTruncates(t *testing.T) {
	got := padBody("a\nb\nc\nd", 2)
	if lines := strings.Count(got, "\n") + 1; lines != 2 {
		t.Errorf("padBody(4 lines, h=2) produced %d lines: %q", lines, got)
	}
}
