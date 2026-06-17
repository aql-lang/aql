package vault

// tui_model.go — the root bubbletea model for `aql vault -i`.
//
// The root owns a stack of screens (push to drill in, esc to pop) and draws
// a fixed 3-zone chrome around the active screen: a header showing the
// active vault + breadcrumb, the body (the top screen's view), and a footer
// that ALWAYS lists the valid keys for the current screen (rendered from the
// screen's own key bindings, so it can never go stale). A ':' command
// palette and a '?' full-help overlay layer on top. Vault switching is a
// global action reachable from any screen via ctrl+o.

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// screen is one view in the navigation stack.
type screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (screen, tea.Cmd)
	View(width, height int) string
	Title() string             // breadcrumb segment
	shortHelp() []key.Binding  // contextual footer keys
	fullHelp() [][]key.Binding // grouped keys for the ? overlay
	capturesInput() bool       // true while editing text (form / active filter)
	reload() tea.Cmd           // re-fetch data after a mutation (may be nil)
	helpInfo() string          // one-line description for the ? help screen
	cliCommand() string        // equivalent CLI command for the current state ("" = none)
}

// --- navigation & app messages ---------------------------------------------

type pushMsg struct{ s screen } // push a new screen
type popMsg struct{}            // pop the top screen
type popToRootMsg struct{}      // pop back to Home
type setStatusMsg struct {      // set the transient status line
	text string
	err  bool
}
type vaultChangedMsg struct{}                 // active vault changed: refresh header + top screen
type reloadTopMsg struct{}                    // ask the top screen to re-fetch its data
type switchedToVaultMsg struct{ name string } // switched/created a vault: reset to its Home

func pushScreen(s screen) tea.Cmd   { return func() tea.Msg { return pushMsg{s} } }
func popScreen() tea.Cmd            { return func() tea.Msg { return popMsg{} } }
func status(text string) tea.Cmd    { return func() tea.Msg { return setStatusMsg{text, false} } }
func statusErr(text string) tea.Cmd { return func() tea.Msg { return setStatusMsg{text, true} } }
func reloadTop() tea.Cmd            { return func() tea.Msg { return reloadTopMsg{} } }

// grantedMsg carries the one-time grant output (incl. the bearer token) so
// the root can show it on a pager before refreshing.
type grantedMsg struct{ output string }

// chromeHeight is the number of terminal lines the header+footer consume,
// leaving the rest for the body.
const chromeTopLines = 5 // title, vault, blank, breadcrumb, blank
const chromeBotLines = 4 // command builder, keys, status bar, message area

type rootModel struct {
	ctl    *tuiController
	stack  []screen
	width  int
	height int

	vault   vaultStatus
	vaultOK bool

	statusLine  string
	statusIsErr bool

	help     help.Model
	showHelp bool
	helpVP   viewport.Model

	palette     textinput.Model
	paletteOpen bool

	theme        themeMode
	detectedDark bool

	lastAdd addDefaults // remembered add-form context (never the alias/value)

	quitting bool
}

// addDefaults remembers the non-secret context of the last "add secret" so a
// run of adds doesn't re-type the provider/namespace/expiry. The alias and the
// value itself are always blank for a fresh add.
type addDefaults struct {
	provider  string
	namespace string
	expiry    string
}

// cycleTheme advances dark/light/auto, applies it, and persists the choice.
func (m *rootModel) cycleTheme() tea.Cmd {
	m.theme = m.theme.next()
	applyTheme(m.theme, m.detectedDark)
	_ = saveTUIPrefs(m.ctl.homeDir, tuiPrefs{Theme: m.theme.String()})
	return status("theme: " + m.theme.String())
}

func newRootModel(ctl *tuiController) *rootModel {
	// Footer help: wider separation between terms and high-contrast styling.
	h := help.New()
	h.ShortSeparator = "  •  "
	h.FullSeparator = "      "
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	h.Styles.ShortKey = tuiKeyStyle
	h.Styles.ShortDesc = tuiDimStyle
	h.Styles.ShortSeparator = sepStyle
	h.Styles.FullKey = tuiKeyStyle
	h.Styles.FullDesc = tuiDimStyle
	h.Styles.FullSeparator = sepStyle

	pal := textinput.New()
	pal.Prompt = ": "
	pal.Placeholder = "type a command — secrets, audit, add, grant, folder …"
	pal.PromptStyle = tuiKeyStyle
	pal.TextStyle = tuiTextStyle
	pal.CharLimit = 64
	return &rootModel{ctl: ctl, help: h, palette: pal, helpVP: viewport.New(0, 0)}
}

func (m *rootModel) Init() tea.Cmd {
	m.refreshVault()
	var first screen
	if m.vaultOK {
		first = m.buildHome()
	} else {
		// No initialized vault at the active location: open the picker so the
		// user can switch to another vault or create one.
		first = m.buildVaultPicker()
	}
	m.stack = []screen{first}
	return first.Init()
}

// --- stack helpers ---------------------------------------------------------

func (m *rootModel) top() screen {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

func (m *rootModel) push(s screen) tea.Cmd {
	m.stack = append(m.stack, s)
	return tea.Batch(s.Init(), m.resizeTop())
}

func (m *rootModel) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *rootModel) updateTop(msg tea.Msg) tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}
	i := len(m.stack) - 1
	s, cmd := m.stack[i].Update(msg)
	m.stack[i] = s
	return cmd
}

// bodySize is the width/height available to the active screen's body.
func (m *rootModel) bodySize() (int, int) {
	h := m.height - chromeTopLines - chromeBotLines
	if h < 1 {
		h = 1
	}
	return m.width, h
}

// resizeTop forwards the current body size directly to the top screen so it
// can resize its inner components.
//
// It must NOT emit a tea.WindowSizeMsg back into the program: that message
// would re-enter the root's WindowSizeMsg handler, which would treat the body
// height as a new terminal height and shrink it by the chrome size on every
// cycle — an infinite feedback loop that collapses the body to a single line.
// Instead it delivers the sized message straight to the active screen and
// returns that screen's command.
func (m *rootModel) resizeTop() tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}
	w, h := m.bodySize()
	return m.updateTop(tea.WindowSizeMsg{Width: w, Height: h})
}

func (m *rootModel) refreshVault() {
	st, ok, _ := m.ctl.status()
	m.vault, m.vaultOK = st, ok
}

// --- update ----------------------------------------------------------------

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.palette.Width = m.paletteInputWidth()
		if m.showHelp {
			m.helpVP.Width = msg.Width
			m.helpVP.Height = maxInt(1, msg.Height-1)
		}
		return m, m.resizeTop()
	case pushMsg:
		return m, m.push(msg.s)
	case popMsg:
		m.pop()
		return m, reloadTop()
	case popToRootMsg:
		for len(m.stack) > 1 {
			m.pop()
		}
		return m, reloadTop()
	case setStatusMsg:
		m.statusLine, m.statusIsErr = msg.text, msg.err
		return m, nil
	case vaultChangedMsg:
		m.refreshVault()
		return m, reloadTop()
	case reloadTopMsg:
		if s := m.top(); s != nil {
			return m, s.reload()
		}
		return m, nil
	case opResultMsg:
		if msg.err != nil {
			m.statusLine, m.statusIsErr = msg.err.Error(), true
		} else {
			m.statusLine, m.statusIsErr = msg.okText, false
		}
		m.refreshVault()
		return m, reloadTop()
	case switchedToVaultMsg:
		m.refreshVault()
		home := m.buildHome()
		m.stack = []screen{home}
		return m, tea.Batch(home.Init(), m.resizeTop(), status("switched to "+msg.name))
	case grantedMsg:
		m.refreshVault()
		return m, tea.Sequence(pushScreen(m.textPager("granted", msg.output)), status("capability granted — copy the token now (shown once)"))
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, m.updateTop(msg)
}

// helpScrollKeys are the keys that scroll the ? help instead of closing it.
var helpScrollKeys = map[string]bool{
	"up": true, "down": true, "j": true, "k": true,
	"pgup": true, "pgdown": true, " ": true, "ctrl+u": true, "ctrl+d": true,
	"home": true, "end": true,
}

func (m *rootModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the ? help is open: ctrl+c still quits, scroll keys scroll, any
	// other key closes it.
	if m.showHelp {
		if k.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if helpScrollKeys[k.String()] {
			var cmd tea.Cmd
			m.helpVP, cmd = m.helpVP.Update(k)
			return m, cmd
		}
		m.showHelp = false
		return m, nil
	}
	// Command palette captures keys until it closes.
	if m.paletteOpen {
		return m.updatePalette(k)
	}
	// Input-capturing screens (forms, active list filter) get first refusal,
	// including esc / ctrl+c which they interpret as cancel.
	if s := m.top(); s != nil && s.capturesInput() {
		return m, m.updateTop(k)
	}

	if k.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	switch {
	case key.Matches(k, keyHelpTog):
		m.openHelp()
		return m, nil
	case key.Matches(k, keyPalette):
		m.openPalette()
		return m, textinput.Blink
	case key.Matches(k, keySwitch):
		// Don't stack a second picker when one is already on top.
		if m.top() != nil && m.top().Title() == "vaults" {
			return m, nil
		}
		return m, m.push(m.buildVaultPicker())
	case key.Matches(k, keyTheme):
		return m, m.cycleTheme()
	case key.Matches(k, keyBack):
		// esc clears a committed list filter before it counts as "back".
		if ls, ok := m.top().(*listScreen); ok && ls.hasAppliedFilter() {
			return m, m.updateTop(k)
		}
		if len(m.stack) > 1 {
			m.pop()
			return m, reloadTop()
		}
		return m, nil
	case key.Matches(k, keyQuit):
		if len(m.stack) == 1 {
			m.quitting = true
			return m, tea.Quit
		}
		return m, m.updateTop(k)
	}
	return m, m.updateTop(k)
}

// openHelp populates and sizes the scrollable help viewport.
func (m *rootModel) openHelp() {
	m.showHelp = true
	m.helpVP.Width = m.width
	m.helpVP.Height = maxInt(1, m.height-1) // leave a line for the close hint
	m.helpVP.SetContent(m.helpContent())
	m.helpVP.GotoTop()
}

// --- command palette -------------------------------------------------------

// paletteHint is the compact, always-shown run/cancel affordance.
const paletteHint = "  enter run · esc cancel"

// paletteInputWidth bounds the input so bubbles renders the placeholder and
// scrolls long input, while leaving room on the line for paletteHint.
func (m *rootModel) paletteInputWidth() int {
	return maxInt(10, m.width-lipgloss.Width(m.palette.Prompt)-len(paletteHint)-1)
}

func (m *rootModel) openPalette() {
	m.paletteOpen = true
	m.palette.SetValue("")
	m.palette.Width = m.paletteInputWidth()
	m.palette.Focus()
}

func (m *rootModel) closePalette() {
	m.paletteOpen = false
	m.palette.Blur()
}

func (m *rootModel) updatePalette(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.closePalette()
		return m, nil
	case "enter":
		cmd := strings.TrimSpace(m.palette.Value())
		m.closePalette()
		return m, m.runPalette(cmd)
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(k)
	return m, cmd
}

// --- view ------------------------------------------------------------------

func (m *rootModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "loading…"
	}
	// The ? help takes over the whole screen with real, readable content.
	if m.showHelp {
		return m.helpScreen()
	}

	var b strings.Builder

	// Header block (chromeTopLines lines): title, vault detail, blank,
	// breadcrumb, blank.
	b.WriteString(m.titleLine())
	b.WriteByte('\n')
	b.WriteString(m.vaultLine())
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(m.breadcrumbView())
	b.WriteByte('\n')
	b.WriteByte('\n')

	// Body.
	w, h := m.bodySize()
	body := ""
	if s := m.top(); s != nil {
		body = s.View(w, h)
	}
	b.WriteString(padBody(body, h))
	b.WriteByte('\n')

	// Bottom chrome (chromeBotLines lines): command builder, keys (or palette),
	// status bar, message area.
	b.WriteString(m.commandBar())
	b.WriteByte('\n')
	if m.paletteOpen {
		b.WriteString(m.paletteBar())
	} else {
		b.WriteString(m.footerView())
	}
	b.WriteByte('\n')
	b.WriteString(m.statusBar())
	b.WriteByte('\n')
	b.WriteString(m.statusView())
	return b.String()
}

// titleLine is the top chrome line: the app name + version.
func (m *rootModel) titleLine() string {
	s := tuiHeaderBar.Render("aql vault")
	if v := headerVersion(); v != "" {
		s += "  " + tuiDimStyle.Render(v)
	}
	return s
}

// vaultLine shows the active vault: name, backend, lock state, and FOLDER.
// It is truncated to the terminal width so a long folder path can never wrap
// and desync the fixed-height header.
func (m *rootModel) vaultLine() string {
	if !m.vaultOK {
		line := tuiVaultStyle.Render("▸ (no vault)") + tuiDimStyle.Render("   press ") +
			tuiKeyStyle.Render("o") + tuiDimStyle.Render(" to choose or create one")
		return ansi.Truncate(line, m.width, "…")
	}
	name := m.ctl.suffix
	if name == "" {
		name = "default"
	}
	lock := tuiOpenStyle.Render("unlocked")
	if m.vault.Locked {
		lock = tuiLockedStyle.Render("LOCKED")
	}
	sep := tuiDimStyle.Render("    ")
	line := tuiVaultStyle.Render("▸ "+name) + sep +
		tuiDimStyle.Render(dash(m.vault.Backend)) + sep +
		lock + sep +
		tuiPathStyle.Render(shortenHome(m.ctl.folder, m.homeDir()))
	return ansi.Truncate(line, m.width, "…")
}

// homeDir is the user's home directory, used to shorten paths with ~.
func (m *rootModel) homeDir() string { return m.ctl.homeDir }

// shortenHome replaces a leading home directory with ~ for a compact path.
func shortenHome(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// headerVersion renders the build version for the header (e.g. "v0.1.0-dev"),
// trimming the trailing VCS stamp so it stays compact. Empty when no version
// was injected (e.g. in unit tests).
func headerVersion() string {
	v := cliVersion
	if v == "" {
		return ""
	}
	if i := strings.Index(v, " (git"); i >= 0 {
		v = v[:i]
	}
	return "v" + v
}

func (m *rootModel) breadcrumbView() string {
	parts := []string{"vault"}
	for _, s := range m.stack {
		if t := s.Title(); t != "" {
			parts = append(parts, t)
		}
	}
	return tuiCrumbStyle.Render(strings.Join(parts, " ▸ "))
}

// commandBar (the command builder) shows the CLI command equivalent to the
// current screen / in-progress form input, so the UI teaches the CLI.
func (m *rootModel) commandBar() string {
	cmd := ""
	if s := m.top(); s != nil {
		cmd = s.cliCommand()
	}
	if cmd == "" {
		return tuiDimStyle.Render("⌘ —")
	}
	return tuiDimStyle.Render("⌘ ") + tuiPathStyle.Render(ansi.Truncate(cmd, maxInt(1, m.width-2), "…"))
}

// statusBar shows compact context: the current screen, item count, and the
// vault lock state.
func (m *rootModel) statusBar() string {
	parts := []string{m.breadcrumbTitle()}
	switch s := m.top().(type) {
	case *tableScreen:
		parts = append(parts, itemCount(s.count))
	case *listScreen:
		parts = append(parts, itemCount(len(s.list.Items())))
	}
	if m.vaultOK {
		if m.vault.Locked {
			parts = append(parts, "locked")
		} else {
			parts = append(parts, "unlocked")
		}
	}
	return tuiDimStyle.Render(strings.Join(parts, "  ·  "))
}

func itemCount(n int) string {
	if n == 1 {
		return "1 item"
	}
	return strconv.Itoa(n) + " items"
}

func (m *rootModel) statusView() string {
	if m.statusLine == "" {
		return ""
	}
	if m.statusIsErr {
		return tuiErrStyle.Render("! " + m.statusLine)
	}
	return tuiOKStyle.Render("✓ " + m.statusLine)
}

func (m *rootModel) footerView() string {
	// Lead with the escape hatches (help + back/quit). bubbles/help truncates
	// from the RIGHT on a narrow terminal, so the keys that must never vanish
	// — '?' (which reveals everything) and quit/back — go FIRST. The screen's
	// own actions follow, then the remaining globals.
	binds := []key.Binding{keyHelpTog}
	if len(m.stack) > 1 {
		binds = append(binds, keyBack)
	} else {
		binds = append(binds, keyQuit)
	}
	if s := m.top(); s != nil {
		binds = append(binds, s.shortHelp()...)
	}
	binds = append(binds, keySwitch, keyPalette)
	return m.help.ShortHelpView(binds)
}

// paletteBar renders the ':' command input as a bottom bar (replacing the
// footer while open) so the typed command is clearly visible. The essential
// "enter run · esc cancel" affordance comes right after the input so it
// survives truncation; the longer examples are appended last.
func (m *rootModel) paletteBar() string {
	bar := m.palette.View() // ":<typed>" with cursor (width-bounded, scrolls)
	return ansi.Truncate(bar+tuiDimStyle.Render(paletteHint), m.width, "…")
}

// helpScreen renders the scrollable ? help: the viewport content plus a pinned
// footer hint, so the close affordance is never clipped on short terminals.
func (m *rootModel) helpScreen() string {
	hint := tuiDimStyle.Render("↑/↓ scroll  ·  any other key closes")
	return m.helpVP.View() + "\n" + hint
}

// helpContent builds the ? help body: a description of the current screen plus
// its keys and the global keys, with real explanations.
func (m *rootModel) helpContent() string {
	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("Help — " + m.breadcrumbTitle()))
	b.WriteString("\n\n")
	b.WriteString(tuiDimStyle.Render("Move with ↑/↓ (or j/k); act with the keys below; esc goes back."))
	b.WriteString("\n\n")
	// Per-screen detailed help: what each action actually does.
	if s := m.top(); s != nil && s.helpInfo() != "" {
		b.WriteString(tuiSectionStyle.Render("On this screen"))
		b.WriteByte('\n')
		b.WriteString(tuiTextStyle.Render(s.helpInfo()))
		b.WriteString("\n\n")
	}
	b.WriteString(tuiSectionStyle.Render("Anywhere"))
	b.WriteByte('\n')
	b.WriteString(renderKeyList([]key.Binding{
		keySwitch, keyPalette, keyTheme, keyHelpTog, keyQuit,
		key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit immediately")),
	}))
	b.WriteString("\n\n")
	b.WriteString(tuiDimStyle.Render("Tip: press ") + tuiKeyStyle.Render(":") +
		tuiDimStyle.Render(" then type a command (e.g. audit) to jump anywhere."))
	return b.String()
}

// renderKeyList formats key bindings as aligned "key   description" rows.
func renderKeyList(binds []key.Binding) string {
	keyCell := tuiKeyStyle.Width(12)
	var b strings.Builder
	first := true
	for _, bind := range binds {
		h := bind.Help()
		if h.Key == "" {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString("   ")
		b.WriteString(keyCell.Render(h.Key))
		b.WriteString(tuiTextStyle.Render(h.Desc))
	}
	return b.String()
}

func (m *rootModel) breadcrumbTitle() string {
	if s := m.top(); s != nil && s.Title() != "" {
		return s.Title()
	}
	return "home"
}

// --- small layout helpers --------------------------------------------------

// padBody pads or truncates body to exactly h lines so the footer stays
// pinned to the bottom.
func padBody(body string, h int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
