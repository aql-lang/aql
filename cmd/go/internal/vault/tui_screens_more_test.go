package vault

// tui_screens_more_test.go — direct tests for the four generic screen
// types (list / table / pager / form) plus the command-execution helpers
// used to drive screen actions synchronously in tests.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// blockingCmdPtrs are the code pointers of the closures returned by
// tea.Tick and cursor.(*Model).BlinkCmd. Both BLOCK until a deadline
// (~500ms for a blink); executing them synchronously would stall every
// pumped keystroke, so collectMsgs drops them by pointer identity.
var blockingCmdPtrs = func() map[uintptr]bool {
	ptrs := map[uintptr]bool{
		reflect.ValueOf(tea.Tick(time.Hour, func(time.Time) tea.Msg { return nil })).Pointer(): true,
	}
	c := cursor.New() // default mode is CursorBlink
	if cmd := c.BlinkCmd(); cmd != nil {
		ptrs[reflect.ValueOf(cmd).Pointer()] = true
	}
	return ptrs
}()

func isTickCmd(cmd tea.Cmd) bool {
	return cmd != nil && blockingCmdPtrs[reflect.ValueOf(cmd).Pointer()]
}

// collectMsgs executes a tea.Cmd, recursively flattening tea.Sequence and
// tea.Batch wrappers (both are slices of tea.Cmd under the hood), and
// returns every terminal message produced. Nil commands yield nothing;
// blocking tick (cursor blink) commands are skipped.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil || isTickCmd(cmd) {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	rv := reflect.ValueOf(msg)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Func {
		var out []tea.Msg
		for i := 0; i < rv.Len(); i++ {
			if c, ok := rv.Index(i).Interface().(tea.Cmd); ok {
				out = append(out, collectMsgs(c)...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

// msgOfType reports whether any collected message has the same dynamic
// type as want.
func msgOfType(msgs []tea.Msg, want tea.Msg) bool {
	for _, m := range msgs {
		if reflect.TypeOf(m) == reflect.TypeOf(want) {
			return true
		}
	}
	return false
}

// firstOpResult returns the first opResultMsg among msgs, if any.
func firstOpResult(msgs []tea.Msg) (opResultMsg, bool) {
	for _, m := range msgs {
		if op, ok := m.(opResultMsg); ok {
			return op, true
		}
	}
	return opResultMsg{}, false
}

// noisyMsg filters out self-perpetuating UI ticks (cursor blink etc.)
// so pumpScreen never loops forever.
func noisyMsg(m tea.Msg) bool {
	name := reflect.TypeOf(m).String()
	return strings.Contains(name, "Blink") || strings.Contains(name, "Tick")
}

// pumpScreen initializes a screen and feeds it the given messages in
// order, executing every returned command and re-feeding the cascade
// (bounded). It returns the final screen and every message observed.
func pumpScreen(t *testing.T, s screen, msgs ...tea.Msg) (screen, []tea.Msg) {
	t.Helper()
	var seen []tea.Msg
	queue := collectMsgs(s.Init())
	feedOne := func(m tea.Msg) {
		seen = append(seen, m)
		ns, cmd := s.Update(m)
		s = ns
		for _, out := range collectMsgs(cmd) {
			if !noisyMsg(out) {
				queue = append(queue, out)
			}
		}
	}
	steps := 0
	for _, m := range msgs {
		// Drain the cascade before the next scripted message.
		for len(queue) > 0 && steps < 500 {
			steps++
			next := queue[0]
			queue = queue[1:]
			feedOne(next)
		}
		feedOne(m)
	}
	for len(queue) > 0 && steps < 500 {
		steps++
		next := queue[0]
		queue = queue[1:]
		feedOne(next)
	}
	return s, seen
}

// --- listScreen --------------------------------------------------------

func TestListScreenActionsAndReload(t *testing.T) {
	ran := ""
	items := []list.Item{
		listItem{name: "one", desc: "first", act: func() tea.Cmd { return status("opened one") }},
		listItem{name: "two", desc: "second", filter: "custom-filter"},
	}
	acts := []listKeyAction{
		{binding: key.NewBinding(key.WithKeys("x")), run: func(sel listItem) tea.Cmd {
			ran = sel.name
			return nil
		}},
	}
	reloaded := false
	s := newListScreen("things", items, acts, func() []list.Item {
		reloaded = true
		return items[:1]
	}).withHelp("help text").withCmd("aql vault things")

	if s.Title() != "things" || s.helpInfo() != "help text" || s.cliCommand() != "aql vault things" {
		t.Errorf("metadata wrong: %q %q %q", s.Title(), s.helpInfo(), s.cliCommand())
	}
	if s.Init() != nil {
		t.Error("listScreen Init should be nil")
	}
	// listItem accessors, incl. the filter override.
	it := items[1].(listItem)
	if it.Title() != "two" || it.Description() != "second" || it.FilterValue() != "custom-filter" {
		t.Errorf("listItem accessors: %q %q %q", it.Title(), it.Description(), it.FilterValue())
	}
	if (items[0].(listItem)).FilterValue() != "one" {
		t.Error("FilterValue should fall back to the name")
	}

	// Size, then enter runs the selected item's act.
	ns, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	s = ns.(*listScreen)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if msgs := collectMsgs(cmd); !msgOfType(msgs, setStatusMsg{}) {
		t.Errorf("enter should run the item's act, got %v", msgs)
	}
	// The action key runs with the current selection.
	if _, _ = s.Update(keyMsg("x")); ran != "one" {
		t.Errorf("action key ran with %q, want one", ran)
	}
	// View renders both rows via the arrow delegate.
	v := s.View(80, 20)
	if !strings.Contains(v, "one") || !strings.Contains(v, "▸") {
		t.Errorf("view missing rows/cursor: %q", v)
	}
	// reload swaps in the reloadFn items.
	if s.reload(); !reloaded || len(s.list.Items()) != 1 {
		t.Errorf("reload did not refresh items (%d left)", len(s.list.Items()))
	}
	// shortHelp / fullHelp include enter + actions + filter.
	if len(s.shortHelp()) < 3 || len(s.fullHelp()) != 1 {
		t.Errorf("help bindings wrong: %d / %d", len(s.shortHelp()), len(s.fullHelp()))
	}
	// arrowDelegate glue.
	var d arrowDelegate
	if d.Height() != 2 || d.Spacing() != 0 || d.Update(nil, nil) != nil {
		t.Error("arrowDelegate metrics changed")
	}
}

func TestListScreenFilteringCapturesInput(t *testing.T) {
	items := []list.Item{
		listItem{name: "alpha", desc: "a"},
		listItem{name: "beta", desc: "b"},
	}
	s := newListScreen("l", items, nil, nil)
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if s.capturesInput() {
		t.Error("should not capture input before filtering")
	}
	// "/" starts filtering; keys are then routed to the list.
	s.Update(keyMsg("/"))
	if !s.capturesInput() {
		t.Fatal("filtering should capture input")
	}
	s.Update(keyMsg("a"))
	s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit the filter
	if !s.hasAppliedFilter() {
		t.Error("filter should be applied after enter")
	}
}

// --- tableScreen -------------------------------------------------------

func newTestTable(rows []table.Row, acts []tableAction, reload func() ([]table.Column, []table.Row)) *tableScreen {
	cols := []table.Column{{Title: "A", Width: 8}, {Title: "B", Width: 8}}
	return newTableScreen("tbl", cols, rows, acts, "empty!", reload)
}

func TestTableScreenEmptyAndActions(t *testing.T) {
	gotIdx := -1
	acts := []tableAction{
		{binding: key.NewBinding(key.WithKeys("a")), run: func(idx int) tea.Cmd {
			gotIdx = idx
			return status("acted")
		}},
	}
	s := newTestTable(nil, acts, nil)
	if s.Init() != nil || s.Title() != "tbl" || s.capturesInput() {
		t.Error("table metadata wrong")
	}
	if v := s.View(40, 10); !strings.Contains(v, "empty!") {
		t.Errorf("empty table should show the empty message: %q", v)
	}
	// Actions still fire on an empty table (add/grant work without rows).
	if _, cmd := s.Update(keyMsg("a")); cmd == nil || gotIdx != 0 {
		t.Errorf("action on empty table: idx=%d", gotIdx)
	}
	if s.withHelp("h").helpInfo() != "h" || s.withCmd("c").cliCommand() != "c" {
		t.Error("withHelp/withCmd on tableScreen broken")
	}
	if len(s.shortHelp()) != 1 || len(s.fullHelp()) != 1 {
		t.Error("table help bindings wrong")
	}
}

func TestTableScreenCursorAndReload(t *testing.T) {
	rows := []table.Row{{"r1", "x"}, {"r2", "y"}, {"r3", "z"}}
	current := rows
	s := newTestTable(rows, nil, func() ([]table.Column, []table.Row) {
		return []table.Column{{Title: "A", Width: 8}, {Title: "B", Width: 8}}, current
	})
	// Tiny window exercises the height clamp.
	s.Update(tea.WindowSizeMsg{Width: 30, Height: 0})
	s.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	// Move the cursor down; the ▸ marker follows.
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if s.tbl.Cursor() != 2 || s.rows[2][0] != "▸" || s.rows[0][0] != " " {
		t.Errorf("cursor marker wrong: cursor=%d rows=%v", s.tbl.Cursor(), s.rows)
	}
	if v := s.View(60, 20); !strings.Contains(v, "r1") {
		t.Errorf("view missing data: %q", v)
	}
	// Shrinking the data clamps the cursor.
	current = rows[:1]
	s.reload()
	if s.count != 1 || s.tbl.Cursor() != 0 {
		t.Errorf("reload clamp: count=%d cursor=%d", s.count, s.tbl.Cursor())
	}
}

// --- pagerScreen -------------------------------------------------------

func TestPagerScreenBasics(t *testing.T) {
	acted := false
	acts := []pagerAction{
		{binding: key.NewBinding(key.WithKeys("p")), run: func() tea.Cmd {
			acted = true
			return nil
		}},
	}
	content := "line one\nline two"
	s := newPagerScreen("pg", content, acts, func() string { return "reloaded" }).
		withHelp("pager help").withCmd("aql vault audit")
	if s.Init() != nil || s.Title() != "pg" || s.capturesInput() {
		t.Error("pager metadata wrong")
	}
	if s.helpInfo() != "pager help" || s.cliCommand() != "aql vault audit" {
		t.Error("pager help/cmd wrong")
	}
	// View before any resize sizes the viewport lazily.
	if v := s.View(40, 10); !strings.Contains(v, "line one") {
		t.Errorf("pager view: %q", v)
	}
	// Resize marks it ready; scroll keys route to the viewport.
	s.Update(tea.WindowSizeMsg{Width: 40, Height: 5})
	if !s.ready {
		t.Error("resize should mark the pager ready")
	}
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if _, _ = s.Update(keyMsg("p")); !acted {
		t.Error("pager action key did not run")
	}
	if len(s.shortHelp()) != 2 || len(s.fullHelp()) != 1 {
		t.Error("pager help bindings wrong")
	}
	s.reload()
	if v := s.View(40, 5); !strings.Contains(v, "reloaded") {
		t.Errorf("reload should replace content: %q", v)
	}
}

// --- formScreen --------------------------------------------------------

func TestFormScreenLifecycle(t *testing.T) {
	submitted := false
	var val string
	mk := func() *formScreen {
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("V").Value(&val),
		))
		return newFormScreen("f", form, func() tea.Cmd {
			submitted = true
			return status("done")
		})
	}
	s := mk()
	if s.Title() != "f" || !s.capturesInput() || s.reload() != nil {
		t.Error("form metadata wrong")
	}
	if s.cliCommand() != "" {
		t.Error("cliCommand without cmdFn should be empty")
	}
	s.cmdFn = func() string { return "aql vault add " + val }
	if !strings.HasPrefix(s.cliCommand(), "aql vault add") {
		t.Errorf("cliCommand with cmdFn: %q", s.cliCommand())
	}
	if len(s.shortHelp()) != 3 || len(s.fullHelp()) != 1 {
		t.Error("form help bindings wrong")
	}
	// Esc pops with a "cancelled" status.
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	msgs := collectMsgs(cmd)
	if !msgOfType(msgs, popMsg{}) || !msgOfType(msgs, setStatusMsg{}) {
		t.Errorf("esc should pop + status, got %v", msgs)
	}
	// Resizing applies the width bound without re-triggering submit.
	s.Update(tea.WindowSizeMsg{Width: 200, Height: 3})
	// A completed form fires onSubmit exactly once.
	s.form.State = huh.StateCompleted
	_, cmd = s.Update(keyMsg("x"))
	if !msgOfType(collectMsgs(cmd), setStatusMsg{}) || !submitted {
		t.Error("completed form should run onSubmit")
	}
	if _, cmd = s.Update(keyMsg("x")); cmd != nil {
		t.Error("onSubmit must fire only once")
	}
	// An aborted form pops with "cancelled".
	s2 := mk()
	s2.form.State = huh.StateAborted
	_, cmd = s2.Update(keyMsg("x"))
	if msgs := collectMsgs(cmd); !msgOfType(msgs, popMsg{}) {
		t.Errorf("aborted form should pop, got %v", msgs)
	}
}

func TestFormScreenDrivenToCompletion(t *testing.T) {
	// A single confirm field driven with the real key events: "y" accepts
	// and completes the group, which fires onSubmit.
	var ok bool
	done := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("sure?").Affirmative("Yes").Negative("No").Value(&ok),
	))
	fs := newFormScreen("confirm", form, func() tea.Cmd {
		done = true
		return nil
	})
	pumpScreen(t, fs, tea.WindowSizeMsg{Width: 60, Height: 10}, keyMsg("y"))
	if !done || !ok {
		t.Errorf("driving 'y' should confirm and submit (done=%v ok=%v)", done, ok)
	}
}

func TestSubmitOpHelpers(t *testing.T) {
	// submitOp pops, runs, and reports ok / error.
	msgs := collectMsgs(submitOp("did it", func() error { return nil }))
	if !msgOfType(msgs, popMsg{}) {
		t.Error("submitOp should pop the form")
	}
	if op, ok := firstOpResult(msgs); !ok || op.okText != "did it" || op.err != nil {
		t.Errorf("submitOp ok result wrong: %+v", msgs)
	}
	msgs = collectMsgs(submitOp("nope", func() error { return errAborted }))
	if op, ok := firstOpResult(msgs); !ok || op.err == nil {
		t.Errorf("submitOp error result wrong: %+v", msgs)
	}
	// submitOpNoPop never pops.
	msgs = collectMsgs(submitOpNoPop("toggled", func() error { return nil }))
	if msgOfType(msgs, popMsg{}) {
		t.Error("submitOpNoPop must not pop")
	}
	if op, ok := firstOpResult(msgs); !ok || op.okText != "toggled" {
		t.Errorf("submitOpNoPop result wrong: %+v", msgs)
	}
	if op, _ := firstOpResult(collectMsgs(submitOpNoPop("x", func() error { return errAborted }))); op.err == nil {
		t.Error("submitOpNoPop should surface errors")
	}
}

func TestMinMaxInt(t *testing.T) {
	if minInt(1, 2) != 1 || minInt(3, 2) != 2 {
		t.Error("minInt broken")
	}
	if maxInt(1, 2) != 2 || maxInt(3, 2) != 3 {
		t.Error("maxInt broken")
	}
}
