package vault

// tui_screens.go — the four generic screen types every concrete vault TUI
// screen is built from:
//
//   listScreen  — a bubbles/list menu (Home, category menus, vault picker)
//   tableScreen — a bubbles/table of records with per-row action keys
//   pagerScreen — a scrollable viewport of captured text (status/audit/…)
//   formScreen  — a huh form for collecting input (add/grant/password/…)
//
// Each implements the screen interface from tui_model.go. Concrete screens
// are assembled by the constructors in tui_build.go, which inject the
// controller calls behind the action keys.

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
)

// arrowDelegate renders list rows with a left-hand "▸" cursor on the current
// row (instead of bubbles' default purple left border), for a consistent
// selection indicator across every menu and the vault picker.
type arrowDelegate struct{}

func (arrowDelegate) Height() int                         { return 2 }
func (arrowDelegate) Spacing() int                        { return 0 }
func (arrowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (arrowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(listItem)
	if !ok {
		return
	}
	width := maxInt(1, m.Width())
	cursor, titleStyle := "  ", tuiTextStyle.Bold(true)
	if index == m.Index() {
		cursor, titleStyle = tuiCursorStyle.Render("▸ "), tuiVaultStyle
	}
	title := ansi.Truncate(cursor+titleStyle.Render(it.name), width, "…")
	desc := ansi.Truncate("  "+tuiDimStyle.Render(it.desc), width, "…")
	fmt.Fprint(w, title+"\n"+desc)
}

// opResultMsg reports the outcome of a mutating operation so the root can
// show a status line and refresh the active vault + top screen.
type opResultMsg struct {
	okText string
	err    error
}

// submitOp pops the current (form) screen, runs do, and reports the result.
func submitOp(okText string, do func() error) tea.Cmd {
	return tea.Sequence(popScreen(), func() tea.Msg {
		if err := do(); err != nil {
			return opResultMsg{err: err}
		}
		return opResultMsg{okText: okText}
	})
}

// --- listScreen ------------------------------------------------------------

// listItem is one selectable row in a listScreen.
type listItem struct {
	name   string
	desc   string
	filter string
	act    func() tea.Cmd // invoked on enter
}

func (i listItem) Title() string       { return i.name }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string {
	if i.filter != "" {
		return i.filter
	}
	return i.name
}

// listKeyAction is an extra action key on a listScreen, run with the
// currently-selected item.
type listKeyAction struct {
	binding key.Binding
	run     func(sel listItem) tea.Cmd
}

type listScreen struct {
	title    string
	list     list.Model
	acts     []listKeyAction
	reloadFn func() []list.Item
	helpText string
	cmdText  string
}

func newListScreen(title string, items []list.Item, acts []listKeyAction, reloadFn func() []list.Item) *listScreen {
	l := list.New(items, arrowDelegate{}, 0, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return &listScreen{title: title, list: l, acts: acts, reloadFn: reloadFn}
}

func (s *listScreen) Init() tea.Cmd { return nil }
func (s *listScreen) Title() string { return s.title }

func (s *listScreen) capturesInput() bool { return s.list.FilterState() == list.Filtering }

// hasAppliedFilter reports whether a committed filter is in effect, so the
// root can route esc to clearing it rather than treating esc as "back".
func (s *listScreen) hasAppliedFilter() bool { return s.list.FilterState() == list.FilterApplied }

func (s *listScreen) selected() (listItem, bool) {
	it, ok := s.list.SelectedItem().(listItem)
	return it, ok
}

func (s *listScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.list.SetSize(msg.Width, msg.Height)
		return s, nil
	case tea.KeyMsg:
		if s.capturesInput() {
			var cmd tea.Cmd
			s.list, cmd = s.list.Update(msg)
			return s, cmd
		}
		if key.Matches(msg, keyEnter) {
			if it, ok := s.selected(); ok && it.act != nil {
				return s, it.act()
			}
			return s, nil
		}
		for _, a := range s.acts {
			if key.Matches(msg, a.binding) {
				if it, ok := s.selected(); ok {
					return s, a.run(it)
				}
				return s, nil
			}
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *listScreen) View(width, height int) string {
	s.list.SetSize(width, height)
	return s.list.View()
}

func (s *listScreen) shortHelp() []key.Binding {
	binds := []key.Binding{keyEnter}
	for _, a := range s.acts {
		binds = append(binds, a.binding)
	}
	binds = append(binds, keyFilter)
	return binds
}

func (s *listScreen) fullHelp() [][]key.Binding {
	return [][]key.Binding{s.shortHelp()}
}

func (s *listScreen) reload() tea.Cmd {
	if s.reloadFn != nil {
		s.list.SetItems(s.reloadFn())
	}
	return nil
}

func (s *listScreen) helpInfo() string              { return s.helpText }
func (s *listScreen) withHelp(t string) *listScreen { s.helpText = t; return s }
func (s *listScreen) cliCommand() string            { return s.cmdText }
func (s *listScreen) withCmd(c string) *listScreen  { s.cmdText = c; return s }

// --- tableScreen -----------------------------------------------------------

// tableAction is a per-row action key; run receives the cursor row index.
type tableAction struct {
	binding key.Binding
	run     func(idx int) tea.Cmd
}

type tableScreen struct {
	title    string
	tbl      table.Model
	rows     []table.Row // data rows incl. the leading cursor cell
	acts     []tableAction
	count    int
	emptyMsg string
	reloadFn func() ([]table.Column, []table.Row)
	helpText string
	cmdText  string
}

// arrowColumn is the narrow leading column that carries the "▸" cursor.
func arrowColumn() table.Column { return table.Column{Title: " ", Width: 2} }

// withArrowCol prepends an (initially blank) cursor cell to each data row.
func withArrowCol(rows []table.Row) []table.Row {
	out := make([]table.Row, len(rows))
	for i, r := range rows {
		out[i] = append(table.Row{" "}, r...)
	}
	return out
}

func newTableScreen(title string, cols []table.Column, rows []table.Row, acts []tableAction, emptyMsg string, reloadFn func() ([]table.Column, []table.Row)) *tableScreen {
	t := table.New(
		table.WithColumns(append([]table.Column{arrowColumn()}, cols...)),
		table.WithFocused(true),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true)
	st.Selected = tuiVaultStyle // current row in the accent color (the ▸ marks it too)
	t.SetStyles(st)
	s := &tableScreen{title: title, tbl: t, acts: acts, count: len(rows), emptyMsg: emptyMsg, reloadFn: reloadFn}
	s.rows = withArrowCol(rows)
	s.applyCursor()
	return s
}

// applyCursor sets the "▸" marker on the current row and "" elsewhere.
func (s *tableScreen) applyCursor() {
	cur := s.tbl.Cursor()
	for i := range s.rows {
		if i == cur {
			s.rows[i][0] = "▸"
		} else {
			s.rows[i][0] = " "
		}
	}
	s.tbl.SetRows(s.rows)
}

func (s *tableScreen) Init() tea.Cmd       { return nil }
func (s *tableScreen) Title() string       { return s.title }
func (s *tableScreen) capturesInput() bool { return false }

func (s *tableScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.tbl.SetWidth(msg.Width)
		h := msg.Height - 1
		if h < 1 {
			h = 1
		}
		s.tbl.SetHeight(h)
		return s, nil
	case tea.KeyMsg:
		for _, a := range s.acts {
			if key.Matches(msg, a.binding) {
				// Each action's closure guards against an empty selection, so
				// row-less actions (add/grant) still work on an empty table.
				return s, a.run(s.tbl.Cursor())
			}
		}
	}
	var cmd tea.Cmd
	s.tbl, cmd = s.tbl.Update(msg)
	s.applyCursor() // keep the ▸ marker on the (possibly moved) cursor row
	return s, cmd
}

func (s *tableScreen) View(width, height int) string {
	if s.count == 0 {
		return tuiDimStyle.Render("  " + s.emptyMsg)
	}
	return s.tbl.View()
}

func (s *tableScreen) shortHelp() []key.Binding {
	binds := []key.Binding{}
	for _, a := range s.acts {
		binds = append(binds, a.binding)
	}
	return binds
}

func (s *tableScreen) fullHelp() [][]key.Binding { return [][]key.Binding{s.shortHelp()} }

func (s *tableScreen) reload() tea.Cmd {
	if s.reloadFn != nil {
		cols, rows := s.reloadFn()
		s.tbl.SetColumns(append([]table.Column{arrowColumn()}, cols...))
		s.rows = withArrowCol(rows)
		s.count = len(rows)
		if s.tbl.Cursor() >= s.count {
			s.tbl.SetCursor(maxInt(0, s.count-1))
		}
		s.applyCursor()
	}
	return nil
}

func (s *tableScreen) helpInfo() string               { return s.helpText }
func (s *tableScreen) withHelp(t string) *tableScreen { s.helpText = t; return s }
func (s *tableScreen) cliCommand() string             { return s.cmdText }
func (s *tableScreen) withCmd(c string) *tableScreen  { s.cmdText = c; return s }

// --- pagerScreen -----------------------------------------------------------

type pagerAction struct {
	binding key.Binding
	run     func() tea.Cmd
}

type pagerScreen struct {
	title    string
	vp       viewport.Model
	acts     []pagerAction
	reloadFn func() string
	ready    bool
	helpText string
	cmdText  string
}

func newPagerScreen(title, content string, acts []pagerAction, reloadFn func() string) *pagerScreen {
	vp := viewport.New(0, 0)
	vp.SetContent(content)
	return &pagerScreen{title: title, vp: vp, acts: acts, reloadFn: reloadFn}
}

func (s *pagerScreen) Init() tea.Cmd       { return nil }
func (s *pagerScreen) Title() string       { return s.title }
func (s *pagerScreen) capturesInput() bool { return false }

func (s *pagerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.vp.Width = msg.Width
		s.vp.Height = msg.Height
		s.ready = true
		return s, nil
	case tea.KeyMsg:
		for _, a := range s.acts {
			if key.Matches(msg, a.binding) {
				return s, a.run()
			}
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s *pagerScreen) View(width, height int) string {
	if !s.ready {
		s.vp.Width = width
		s.vp.Height = height
	}
	return s.vp.View()
}

func (s *pagerScreen) shortHelp() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
	}
	for _, a := range s.acts {
		binds = append(binds, a.binding)
	}
	return binds
}

func (s *pagerScreen) fullHelp() [][]key.Binding { return [][]key.Binding{s.shortHelp()} }

func (s *pagerScreen) reload() tea.Cmd {
	if s.reloadFn != nil {
		s.vp.SetContent(s.reloadFn())
	}
	return nil
}

func (s *pagerScreen) helpInfo() string               { return s.helpText }
func (s *pagerScreen) withHelp(t string) *pagerScreen { s.helpText = t; return s }
func (s *pagerScreen) cliCommand() string             { return s.cmdText }
func (s *pagerScreen) withCmd(c string) *pagerScreen  { s.cmdText = c; return s }

// --- formScreen ------------------------------------------------------------

type formScreen struct {
	title    string
	form     *huh.Form
	onSubmit func() tea.Cmd
	done     bool
	helpText string
	cmdFn    func() string // live CLI-command preview of the entered values
}

func (s *formScreen) helpInfo() string { return s.helpText }
func (s *formScreen) cliCommand() string {
	if s.cmdFn != nil {
		return s.cmdFn()
	}
	return ""
}

func newFormScreen(title string, form *huh.Form, onSubmit func() tea.Cmd) *formScreen {
	return &formScreen{title: title, form: form.WithShowHelp(true).WithShowErrors(true), onSubmit: onSubmit}
}

func (s *formScreen) Init() tea.Cmd       { return s.form.Init() }
func (s *formScreen) Title() string       { return s.title }
func (s *formScreen) capturesInput() bool { return true }

func (s *formScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
		return s, tea.Batch(popScreen(), status("cancelled"))
	}
	// Size the form ONCE per resize, not on every render: re-applying
	// WithWidth/WithHeight each View resets huh's scroll + focus, so the final
	// Submit/Cancel field never appears highlighted and enter on it looks
	// dead. huh scrolls the focused field into view on its own.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.form = s.form.WithWidth(minInt(ws.Width, 72)).WithHeight(maxInt(ws.Height, 6))
	}
	fm, cmd := s.form.Update(msg)
	if f, ok := fm.(*huh.Form); ok {
		s.form = f
	}
	switch s.form.State {
	case huh.StateCompleted:
		if !s.done {
			s.done = true
			return s, s.onSubmit()
		}
	case huh.StateAborted:
		return s, tea.Batch(popScreen(), status("cancelled"))
	}
	return s, cmd
}

func (s *formScreen) View(width, height int) string { return s.form.View() }

func (s *formScreen) shortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (s *formScreen) fullHelp() [][]key.Binding { return [][]key.Binding{s.shortHelp()} }
func (s *formScreen) reload() tea.Cmd           { return nil }

// --- shared int helpers ----------------------------------------------------

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
