// SPECULATIVE VALIDATION TEST — written against design/TUI.0.md before
// aql:tui exists. design/ is outside go.work, so this file is never
// compiled. It graduates to lang/go/test/apps_test.go (beside
// TestAppTodoAPI, whose harness and assertion shape it mirrors) when
// the module ships: runAppSteps gains backend registration, the scripted
// HTTP calls become scripted key events, and the "|"-joined final-state
// fields are asserted part-by-part exactly like the REST statuses were.
//
// Speculative surfaces used (all specified in design/TUI.0.md):
//   - lang/go/tuikit: VirtualBackend (§4.4), Frames history (§8.1),
//     Event constructors (illustrative — §2.4 fixes the value shapes,
//     not the Go constructor spellings).
//   - lang/go/modules: RegisterHostTui + TuiSpec (§4.3).
package tuiprobe

import (
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/tuikit" // SPECULATIVE (TUI.0.md §4.2)
)

// runTuiAppSteps mirrors runAppSteps (lang/go/test/apps_test.go) with one
// addition: the virtual terminal backend is registered on the registry
// before any step runs (TUI.0.md §4.3), so `TodoTui.run {}` drives an
// in-memory screen instead of a TTY. Pass vb == nil to leave NO backend
// registered (the negative path).
func runTuiAppSteps(t *testing.T, vb *tuikit.VirtualBackend, appFiles, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for _, name := range appFiles {
		src, err := os.ReadFile("../../../design/examples/tui/" + name)
		if err != nil {
			t.Fatalf("read app %s: %v", name, err)
		}
		mem.Files["/apps/"+name] = src
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	native.SetHostFileOps(reg, mem)
	modules.InstallResolver(reg)
	if vb != nil {
		err = modules.RegisterHostTui(reg, modules.TuiSpec{
			Name: "virtual",
			Open: func() (tuikit.Backend, error) { return vb, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// typeKeys queues one key event per rune (Event constructor spellings
// illustrative; the event VALUE shapes are fixed by TUI.0.md §2.4).
func typeKeys(vb *tuikit.VirtualBackend, s string) {
	for _, r := range s {
		vb.Inject(tuikit.KeyEvent(string(r)))
	}
}

// anyFrameContains reports whether any recorded frame's rendered screen
// contains want — the golden-substring check over the frame history the
// virtual backend records at every Present (TUI.0.md §8.1).
func anyFrameContains(vb *tuikit.VirtualBackend, want string) bool {
	for i := 0; i < vb.FrameCount(); i++ {
		if strings.Contains(strings.Join(vb.Screen(i), "\n"), want) {
			return true
		}
	}
	return false
}

// The todo TUI (todo-tui.aql): the todo-api CRUD contract driven by key
// events over the virtual backend — create twice, toggle one, delete the
// other, quit — asserting the returned final state AND the rendered
// frames, positives paired with negatives throughout.
func TestAppTodoTUI(t *testing.T) {
	vb := tuikit.NewVirtualBackend(80, 24)

	// Script the whole session up front; the driver drains the queue in
	// order, as if typed (events are coalesced per TUI.0.md §2.2, which
	// this test deliberately does not depend on: only final state and
	// "some frame showed X" are asserted, never frame counts).
	typeKeys(vb, "buy milk")
	vb.Inject(tuikit.KeyEvent("enter")) // create #1
	typeKeys(vb, "walk dog")
	vb.Inject(tuikit.KeyEvent("enter")) // create #2
	vb.Inject(tuikit.KeyEvent("tab"))   // focus: entry → list
	vb.Inject(tuikit.KeyEvent("down"))  // select #2
	vb.Inject(tuikit.KeyEvent("space")) // toggle walk dog → done
	vb.Inject(tuikit.KeyEvent("up"))    // select #1
	vb.Inject(tuikit.KeyEvent("d"))     // delete buy milk
	vb.Inject(tuikit.KeyEvent("q"))     // quit from list focus

	out, err := runTuiAppSteps(t, vb, []string{"todo-tui.aql"}, []string{
		`import "/apps/todo-tui.aql"`,
		`def final (TodoTui.run {})`,
		// mirror TestAppTodoAPI: join the interesting fields, assert parts
		`def t0 (final.todos get 0)`,
		`join "|" [(convert String (size final.todos)) t0.text (convert String t0.done)
		           (convert String final.next) (convert String final.sel) final.focus]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	if len(parts) != 6 {
		t.Fatalf("expected 6 fields, got %q", got)
	}
	if parts[0] != "1" {
		t.Errorf("todo count = %s, want 1 (created 2, deleted 1)", parts[0])
	}
	if parts[1] != "walk dog" {
		t.Errorf("surviving todo = %s, want walk dog", parts[1])
	}
	if parts[2] != "true" {
		t.Errorf("surviving todo done = %s, want true (space toggled it)", parts[2])
	}
	if parts[3] != "3" {
		t.Errorf("next id counter = %s, want 3 (ids are never reused, like the server)", parts[3])
	}
	if parts[4] != "0" {
		t.Errorf("sel = %s, want 0 (clamped after delete)", parts[4])
	}
	if parts[5] != "list" {
		t.Errorf("focus = %s, want list", parts[5])
	}

	// Golden frames from the recorded history (§8.1): the entry echoed
	// keystrokes, both rows rendered, the toggle rendered, and the
	// count in the box title tracked done/total.
	for _, want := range []string{
		"what needs doing?", // placeholder frame, before any typing
		"buy milk",          // echoed in the entry, then listed
		"[ ] walk dog",      // listed before the toggle
		"[x] walk dog",      // listed after the toggle
		"todos 1/2",         // box title after toggle, before delete
		"todos 1/1",         // box title after the delete
	} {
		if !anyFrameContains(vb, want) {
			t.Errorf("no recorded frame contains %q", want)
		}
	}
	if anyFrameContains(vb, "[x] buy milk") {
		t.Errorf("buy milk was never toggled; no frame should show it done")
	}
}

// The negative pairs, mirroring TestAppTodoAPI's bodyless-POST-400 row.
func TestAppTodoTUIRejections(t *testing.T) {
	// (1) no backend registered → the module's first-class error (§4.3).
	_, err := runTuiAppSteps(t, nil, []string{"todo-tui.aql"}, []string{
		`import "/apps/todo-tui.aql"`,
		`def final (TodoTui.run {})`,
	})
	if err == nil || !strings.Contains(err.Error(), "no terminal backend registered") {
		t.Errorf("run without a backend = %v, want no-backend tui_error", err)
	}

	// (2) a config map with no update word → rejected before the loop starts.
	vb := tuikit.NewVirtualBackend(80, 24)
	_, err = runTuiAppSteps(t, vb, nil, []string{
		`import "aql:tui"`,
		`Tui.run {}`,
	})
	if err == nil || !strings.Contains(err.Error(), "missing update") {
		t.Errorf("Tui.run {} = %v, want missing-update error", err)
	}

	// (3) Ctrl-\ always quits (§2.5) — even mid-typing, never delivered
	// to update, and the final state still comes back cleanly.
	vb2 := tuikit.NewVirtualBackend(80, 24)
	typeKeys(vb2, "half a tod")
	vb2.Inject(tuikit.CtrlEvent(`\`))
	out, err := runTuiAppSteps(t, vb2, []string{"todo-tui.aql"}, []string{
		`import "/apps/todo-tui.aql"`,
		`def final (TodoTui.run {})`,
		`convert String (size final.todos)`,
	})
	if err != nil {
		t.Fatalf("hard quit run: %v", err)
	}
	if got := appLastString(t, out); got != "0" {
		t.Errorf("todos after hard quit = %s, want 0 (entry text was never submitted)", got)
	}
}

// appLastString is shared with apps_test.go at graduation time; inlined
// here so the speculative file is self-contained.
func appLastString(t *testing.T, vals []native.Value) string {
	t.Helper()
	if len(vals) == 0 {
		t.Fatal("no result values")
	}
	s, err := native.AsString(vals[len(vals)-1])
	if err != nil {
		t.Fatalf("expected String result, got %v", vals)
	}
	return s
}
