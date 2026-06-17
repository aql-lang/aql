package vault

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(t *testing.T) *rootModel {
	t.Helper()
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	// Give it a width so the help footer renders without truncation.
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	return nm.(*rootModel)
}

func keyMsg(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func feed(t *testing.T, m *rootModel, msg tea.Msg) *rootModel {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(*rootModel)
}

func TestModelStartsOnHome(t *testing.T) {
	m := newTestModel(t)
	if len(m.stack) != 1 {
		t.Fatalf("stack depth = %d, want 1", len(m.stack))
	}
	if _, ok := m.top().(*listScreen); !ok {
		t.Errorf("home should be a listScreen, got %T", m.top())
	}
}

func TestModelPaletteNavigation(t *testing.T) {
	m := newTestModel(t)
	cmd := m.runPalette("secrets")
	if cmd == nil {
		t.Fatal("runPalette(secrets) returned nil")
	}
	msg := cmd()
	pm, ok := msg.(pushMsg)
	if !ok {
		t.Fatalf("expected pushMsg, got %T", msg)
	}
	if pm.s.Title() != "secrets" {
		t.Errorf("palette pushed %q, want secrets", pm.s.Title())
	}
	// Unknown command surfaces an error status, not a panic.
	if cmd := m.runPalette("bogus"); cmd == nil {
		t.Errorf("unknown palette command should return a status cmd")
	} else if _, ok := cmd().(setStatusMsg); !ok {
		t.Errorf("unknown command should yield a setStatusMsg")
	}
}

func TestModelFooterAlwaysListsKeys(t *testing.T) {
	m := newTestModel(t)
	// Home footer: the global keys are always present.
	home := m.footerView()
	for _, want := range []string{"open", "switch", "command", "help", "quit"} {
		if !strings.Contains(home, want) {
			t.Errorf("home footer missing %q: %q", want, home)
		}
	}
	// Drill into Secrets: its action keys appear, plus back + globals.
	m = feed(t, m, pushMsg{m.buildSecrets()})
	sec := m.footerView()
	for _, want := range []string{"details", "add", "back", "switch", "help"} {
		if !strings.Contains(sec, want) {
			t.Errorf("secrets footer missing %q: %q", want, sec)
		}
	}
}

func TestModelEscPopsAndRootNoop(t *testing.T) {
	m := newTestModel(t)
	m = feed(t, m, pushMsg{m.buildSecrets()})
	if len(m.stack) != 2 {
		t.Fatalf("stack depth = %d, want 2", len(m.stack))
	}
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.stack) != 1 {
		t.Fatalf("esc should pop to depth 1, got %d", len(m.stack))
	}
	// esc at root is a no-op (does not pop below Home).
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.stack) != 1 {
		t.Errorf("esc at root changed depth to %d", len(m.stack))
	}
}

func TestModelHelpOverlayToggles(t *testing.T) {
	m := newTestModel(t)
	m = feed(t, m, keyMsg("?"))
	if !m.showHelp {
		t.Fatal("? should open the help overlay")
	}
	m = feed(t, m, keyMsg("x"))
	if m.showHelp {
		t.Error("any key should close the help overlay")
	}
}

func TestModelQuitAtRoot(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(keyMsg("q"))
	m = nm.(*rootModel)
	if !m.quitting {
		t.Error("q at root should set quitting")
	}
	if cmd == nil {
		t.Error("q at root should return a quit command")
	}
}

func TestModelResizeNoFeedbackLoop(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	nm, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(*rootModel)
	if m.width != 80 || m.height != 24 {
		t.Fatalf("dims = %dx%d, want 80x24", m.width, m.height)
	}
	// The resize command must NOT re-emit a WindowSizeMsg — that would
	// re-enter the resize handler and shrink the height every cycle.
	if cmd != nil {
		if ws, ok := cmd().(tea.WindowSizeMsg); ok {
			t.Fatalf("resize re-emitted WindowSizeMsg %+v (feedback loop)", ws)
		}
	}
	// The body renders at full height (not collapsed to the footer).
	if v := m.View(); !strings.Contains(v, "Secrets") {
		t.Errorf("home body did not render after resize: %q", v)
	}
}

func TestModelViewRenders(t *testing.T) {
	m := newTestModel(t)
	home := m.View()
	if !strings.Contains(home, "aql vault") {
		t.Errorf("home view missing header: %q", home)
	}
	for _, want := range []string{"Secrets", "Access", "Passwords", "Maintenance", "Settings"} {
		if !strings.Contains(home, want) {
			t.Errorf("home view missing category %q", want)
		}
	}
	// With a secret present, drilling into Secrets renders the table header.
	if err := m.ctl.addSecret("k", "v", "github", "", ""); err != nil {
		t.Fatal(err)
	}
	m = feed(t, m, pushMsg{m.buildSecrets()})
	m = feed(t, m, tea.WindowSizeMsg{Width: 200, Height: 40})
	sec := m.View()
	if !strings.Contains(sec, "ALIAS") || !strings.Contains(sec, "k") {
		t.Errorf("secrets view missing table/row: %q", sec)
	}
}

func TestVaultPickerEnterSwitches(t *testing.T) {
	m := newTestModel(t)
	picker, ok := m.buildVaultPicker().(*listScreen)
	if !ok {
		t.Fatalf("picker is %T", m.buildVaultPicker())
	}
	if _, ok := picker.selected(); !ok {
		t.Fatal("picker has no selectable vault")
	}
	// Enter on a vault must DO something (switch), not be a silent no-op.
	_, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a vault produced no command (silent no-op)")
	}
	switch msg := cmd().(type) {
	case switchedToVaultMsg, opResultMsg:
		// a switch attempt (success or surfaced error) — not a no-op
	default:
		t.Errorf("enter should switch the vault, got %T", msg)
	}
	// With the picker active, the footer lists its keys once (no duplicate
	// "enter open") plus the per-row actions.
	m = feed(t, m, pushMsg{m.buildVaultPicker()})
	f := m.footerView()
	if c := strings.Count(f, "open"); c != 1 {
		t.Errorf("footer should list 'open' exactly once, got %d: %q", c, f)
	}
	for _, want := range []string{"new", "default", "prune"} {
		if !strings.Contains(f, want) {
			t.Errorf("picker footer missing %q: %q", want, f)
		}
	}
}

func TestModelHelpScreenIsHelpful(t *testing.T) {
	m := newTestModel(t)
	if err := m.ctl.addSecret("k", "v", "github", "", ""); err != nil {
		t.Fatal(err)
	}
	// The per-secret detail page is where reveal + the explained actions live.
	m = feed(t, m, pushMsg{m.buildSecretDetail("k")})
	m.openHelp()
	h := m.View() // renders the scrollable help screen
	for _, want := range []string{
		"Help", "one secret", // title + description
		"On this screen", "Reveal", // section + an action
		"Rotate", "replace the value with a new one", // rotate is actually explained
		"Anywhere", "switch", "command", "theme", // global keys
		"closes", // pinned close hint
	} {
		if !strings.Contains(h, want) {
			t.Errorf("help screen missing %q", want)
		}
	}
}

func TestModelPaletteBarShowsTypedCommand(t *testing.T) {
	m := newTestModel(t)
	m = feed(t, m, keyMsg(":"))
	for _, r := range "audit" {
		m = feed(t, m, keyMsg(string(r)))
	}
	v := m.View() // footer area is now the palette bar
	if !strings.Contains(v, "audit") {
		t.Errorf("palette bar should show the typed command: %q", v)
	}
	if !strings.Contains(v, "enter run") {
		t.Errorf("palette bar should show a run/cancel hint: %q", v)
	}
}

func TestModelHeaderShowsVersion(t *testing.T) {
	old := cliVersion
	defer SetVersion(old)
	SetVersion("9.9.9-test (git abc123, dirty)")
	m := newTestModel(t)
	h := m.titleLine()
	if !strings.Contains(h, "v9.9.9-test") {
		t.Errorf("title line missing version: %q", h)
	}
	if strings.Contains(h, "git") {
		t.Errorf("title line should trim the VCS stamp: %q", h)
	}
	if !strings.Contains(h, "aql vault") {
		t.Errorf("title line missing app name: %q", h)
	}
	// The folder is shown on the vault line (home shortened to ~).
	if vl := m.vaultLine(); !strings.Contains(vl, ".aql") {
		t.Errorf("vault line should show the folder: %q", vl)
	}
}

func TestModelSwitchKeyOpensPicker(t *testing.T) {
	m := newTestModel(t)
	// 'o' (no ctrl) opens the picker.
	m = feed(t, m, keyMsg("o"))
	if m.top().Title() != "vaults" {
		t.Errorf("o should open the vault picker, got %q", m.top().Title())
	}
	depth := len(m.stack)
	// Pressing 'o' again must NOT stack a second picker.
	m = feed(t, m, keyMsg("o"))
	if len(m.stack) != depth {
		t.Errorf("o on the picker should be a no-op, depth %d -> %d", depth, len(m.stack))
	}
	// ctrl+o is still accepted as an alias from a normal screen.
	m2 := newTestModel(t)
	m2 = feed(t, m2, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m2.top().Title() != "vaults" {
		t.Errorf("ctrl+o alias should still open the picker, got %q", m2.top().Title())
	}
}

func TestPaletteWidthIsBounded(t *testing.T) {
	m := newTestModel(t)
	m = feed(t, m, keyMsg(":"))
	if m.palette.Width <= 0 {
		t.Errorf("palette width should be set on open (got %d) so the placeholder renders", m.palette.Width)
	}
}

func TestHelpScrollAndCtrlCStillQuits(t *testing.T) {
	m := newTestModel(t)
	m.openHelp()
	// A scroll key does not close the help.
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if !m.showHelp {
		t.Error("scroll key should not close help")
	}
	// ctrl+c quits even from the help screen.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !nm.(*rootModel).quitting || cmd == nil {
		t.Error("ctrl+c should quit from the help screen")
	}
}
