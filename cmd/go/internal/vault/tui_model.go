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
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
const chromeBotLines = 2 // status line + help line

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

	palette     textinput.Model
	paletteOpen bool

	quitting bool
}

func newRootModel(ctl *tuiController) *rootModel {
	// Footer help: wider separation between terms and high-contrast styling.
	h := help.New()
	h.ShortSeparator = "   •   "
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
	return &rootModel{ctl: ctl, help: h, palette: pal}
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

func (m *rootModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Full-help overlay swallows the next key (closing it).
	if m.showHelp {
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
		m.showHelp = true
		return m, nil
	case key.Matches(k, keyPalette):
		m.openPalette()
		return m, textinput.Blink
	case key.Matches(k, keySwitch):
		return m, m.push(m.buildVaultPicker())
	case key.Matches(k, keyBack):
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

// --- command palette -------------------------------------------------------

func (m *rootModel) openPalette() {
	m.paletteOpen = true
	m.palette.SetValue("")
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

	// Status line.
	b.WriteString(m.statusView())
	b.WriteByte('\n')

	// Footer — the command palette replaces it while open, so the typed
	// command is unmistakably visible at the bottom.
	if m.paletteOpen {
		b.WriteString(m.paletteBar())
	} else {
		b.WriteString(m.footerView())
	}
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
func (m *rootModel) vaultLine() string {
	if !m.vaultOK {
		return tuiVaultStyle.Render("▸ (no vault)") + tuiDimStyle.Render("   press ") +
			tuiVaultStyle.Render("o") + tuiDimStyle.Render(" to choose or create one")
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
	return tuiVaultStyle.Render("▸ "+name) + sep +
		tuiDimStyle.Render(dash(m.vault.Backend)) + sep +
		lock + sep +
		tuiPathStyle.Render(m.ctl.folder)
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
	var binds []key.Binding
	if s := m.top(); s != nil {
		binds = append(binds, s.shortHelp()...)
	}
	// Global keys, always visible.
	if len(m.stack) > 1 {
		binds = append(binds, keyBack)
	}
	binds = append(binds, keySwitch, keyPalette, keyHelpTog)
	if len(m.stack) == 1 {
		binds = append(binds, keyQuit)
	}
	return m.help.ShortHelpView(binds)
}

// paletteBar renders the ':' command input as a bottom bar (replacing the
// footer while open) so the typed command is clearly visible.
func (m *rootModel) paletteBar() string {
	bar := m.palette.View() // ":<typed>" with cursor
	hint := tuiDimStyle.Render("    enter run · esc cancel · e.g. secrets · audit · add · grant · folder")
	return ansi.Truncate(bar+hint, m.width, "…")
}

// helpScreen is the full-screen ? help: a description of the current screen
// plus its keys and the global keys, with real explanations.
func (m *rootModel) helpScreen() string {
	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("Help — " + m.breadcrumbTitle()))
	b.WriteString("\n\n")
	if s := m.top(); s != nil && s.helpInfo() != "" {
		b.WriteString(tuiTextStyle.Render(s.helpInfo()))
		b.WriteString("\n\n")
	}
	b.WriteString(tuiDimStyle.Render("Move with ↑/↓ (or j/k), open with enter, go back with esc."))
	b.WriteString("\n\n")
	if s := m.top(); s != nil {
		if keys := s.shortHelp(); len(keys) > 0 {
			b.WriteString(tuiSectionStyle.Render("On this screen"))
			b.WriteByte('\n')
			b.WriteString(renderKeyList(keys))
			b.WriteString("\n\n")
		}
	}
	b.WriteString(tuiSectionStyle.Render("Anywhere"))
	b.WriteByte('\n')
	b.WriteString(renderKeyList([]key.Binding{
		keySwitch, keyPalette, keyHelpTog, keyQuit,
		key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit immediately")),
	}))
	b.WriteString("\n\n")
	b.WriteString(tuiDimStyle.Render("Tip: press ") + tuiKeyStyle.Render(":") +
		tuiDimStyle.Render(" then type a command (e.g. audit) to jump anywhere."))
	b.WriteString("\n\n")
	b.WriteString(tuiDimStyle.Render("press any key to close"))
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
