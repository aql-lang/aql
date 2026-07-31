package modules

import (
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// End-to-end coverage for the Tier-2 app runtime (tui_run.go) and the
// widget constructor words (tui_widgets.go), driven entirely over the
// VirtualBackend — the §1 counter and friends, with the negative
// contracts paired per house rule.

func key(k, char string, mods ...string) tuikit.Event {
	return tuikit.Event{Tag: "key", Key: k, Char: char, Mods: mods}
}

const counterApp = `def app {
  init: {n: 0}
  update: ([state:Map ev:Map] => [ case ev.key [
      "up"   [ drop state set n (add state.n 1) ]
      "down" [ drop state set n (state.n sub 1) ]
      "q"    [ drop Tui.quit state ]
      state ] ])
  view: ([state:Map] => [
      Tui.box (Tui.text (join "" ["count: " (convert String state.n)]) {style: {bold: true}})
              {title: "counter"  border: "round"} ])
}`

// The RFC §1 counter, verbatim shape: keys fold state, quit returns it,
// frames land on the virtual screen.
func TestTuiRunCounter(t *testing.T) {
	vb := tuikit.NewVirtualBackend(20, 5)
	for _, ev := range []tuikit.Event{
		key("up", ""), key("up", ""), key("down", ""), key("q", "q"),
	} {
		vb.Inject(ev)
	}
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		counterApp,
		`def final (Tui.run app)`,
		`convert String final.n`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := tuiLastString(t, out); got != "1" {
		t.Fatalf("final.n = %s, want 1", got)
	}
	// the whole scripted session drains in one coalesced batch and quit
	// skips the final paint (§2.2) — the observable frame is the init
	// render, boxed and titled
	found := false
	for i := 0; i < vb.FrameCount(); i++ {
		joined := strings.Join(vb.ScreenAt(i), "\n")
		if strings.Contains(joined, "count: 0") && strings.Contains(joined, "counter") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no frame showed the boxed counter; last = %q", vb.ScreenAt(vb.FrameCount()-1))
	}
}

// Ctrl-C quits by default with the current state; {ctrl-c: "deliver"}
// hands it to update instead; Ctrl-\ quits even in deliver mode.
func TestTuiRunCtrlChords(t *testing.T) {
	vb := tuikit.NewVirtualBackend(10, 3)
	vb.Inject(key("up", ""))
	vb.Inject(key("c", "c", "ctrl"))
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`, counterApp,
		`def final (Tui.run app)`,
		`convert String final.n`,
	})
	if err != nil {
		t.Fatalf("ctrl-c run: %v", err)
	}
	if got := tuiLastString(t, out); got != "1" {
		t.Fatalf("ctrl-c final.n = %s, want 1", got)
	}

	vb2 := tuikit.NewVirtualBackend(10, 3)
	vb2.Inject(key("c", "c", "ctrl")) // delivered: update ignores it
	vb2.Inject(key("up", ""))
	vb2.Inject(key(`\`, "", "ctrl")) // the hard chord still quits
	out, err = runTuiSteps(t, vb2, []string{
		`import "boru:tui"`, counterApp,
		`def cfg (app set ctrl-c "deliver")`,
		`def final (Tui.run cfg)`,
		`convert String final.n`,
	})
	if err != nil {
		t.Fatalf("deliver run: %v", err)
	}
	if got := tuiLastString(t, out); got != "1" {
		t.Fatalf("deliver final.n = %s, want 1", got)
	}
}

// A resize event re-dimensions the render and still reaches update; a
// worker process sends an ordinary message into the UI mailbox via the
// registered "tui" name; the focused input places the hardware cursor.
func TestTuiRunResizeWorkerAndCursor(t *testing.T) {
	vb := tuikit.NewVirtualBackend(8, 2)
	vb.Inject(tuikit.Event{Tag: "resize", Cols: 12, Rows: 3})
	vb.Inject(key("q", "q"))
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def app {
		   init: {last: ""  entry: (Tui.input "hi" {focus: true})}
		   update: ([state:Map ev:Map] => [ case ev.tag [
		       "resize" [ drop state set last (convert String ev.cols) ]
		       "key"    [ drop case ev.key [ "q" [ drop Tui.quit state ] state ] ]
		       state ] ])
		   view: ([state:Map] => [ Tui.rows [ state.entry ] ])
		 }`,
		`def final (Tui.run app)`,
		`final.last`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := tuiLastString(t, out); got != "12" {
		t.Fatalf("resize reached update as %q, want 12", got)
	}
	if x, y, on := vb.Cursor(); !on || x != 2 || y != 0 {
		t.Fatalf("cursor = %d,%d,%v want 2,0 visible", x, y, on)
	}

	// worker: a spawned process addresses the UI by registered name
	vb2 := tuikit.NewVirtualBackend(8, 2)
	out, err = runTuiSteps(t, vb2, []string{
		`import "boru:tui"`,
		`def app {
		   init: {got: ""}
		   update: ([state:Map ev:Map] => [ case ev.tag [
		       "init"   [ drop spawn [ send {tag: "loaded"  items: "three"} (whereis "tui") ] drop state ]
		       "loaded" [ drop Tui.quit (state set got ev.items) ]
		       state ] ])
		   view: ([state:Map] => [ Tui.text state.got ])
		 }`,
		`def final (Tui.run app)`,
		`final.got`,
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}
	if got := tuiLastString(t, out); got != "three" {
		t.Fatalf("worker message = %q, want three", got)
	}
}

// The negative contracts of the run word.
func TestTuiRunRejections(t *testing.T) {
	steps := func(vb *tuikit.VirtualBackend, body string) error {
		_, err := runTuiSteps(t, vb, []string{`import "boru:tui"`, body})
		return err
	}
	vb := tuikit.NewVirtualBackend(4, 2)
	if err := steps(vb, `Tui.run {}`); err == nil || !strings.Contains(err.Error(), "missing update") {
		t.Fatalf("missing update = %v", err)
	}
	vb = tuikit.NewVirtualBackend(4, 2)
	if err := steps(vb, `Tui.run {update: ([s:Map e:Map] => [s])}`); err == nil ||
		!strings.Contains(err.Error(), "missing view") {
		t.Fatalf("missing view = %v", err)
	}
	vb = tuikit.NewVirtualBackend(4, 2)
	if err := steps(vb, `Tui.run {update: 5  view: 6}`); err == nil ||
		!strings.Contains(err.Error(), "update must be a function") {
		t.Fatalf("non-fn update = %v", err)
	}
	vb = tuikit.NewVirtualBackend(4, 2)
	if err := steps(vb, `Tui.run {update: ([s:Map e:Map] => [s])  view: 6}`); err == nil ||
		!strings.Contains(err.Error(), "view must be a function") {
		t.Fatalf("non-fn view = %v", err)
	}
	vb = tuikit.NewVirtualBackend(4, 2)
	if err := steps(vb, `Tui.run {ctrl-c: "maybe"  update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`); err == nil ||
		!strings.Contains(err.Error(), `ctrl-c: must be`) {
		t.Fatalf("bad ctrl-c = %v", err)
	}
	// a view returning a non-widget is a bad_widget error, terminal restored
	vb = tuikit.NewVirtualBackend(4, 2)
	err := steps(vb, `Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [5])}`)
	if err == nil || !strings.Contains(err.Error(), "bad widget") {
		t.Fatalf("bad view tree = %v", err)
	}
	// an update raising mid-loop propagates after restore
	vb = tuikit.NewVirtualBackend(4, 2)
	vb.Inject(key("x", "x"))
	err = steps(vb, `Tui.run {update: ([s:Map e:Map] => [ case e.tag [
	    "init" [ drop s ] [ raise "boom" ] ] ])
	  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("raising update = %v", err)
	}
}

// A second concurrent run is rejected: the registered name is the lock.
func TestTuiRunAlreadyRunning(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterHostTui(reg, TuiSpec{Name: "v", Open: func() (tuikit.Backend, error) { return vb, nil }}); err != nil {
		t.Fatal(err)
	}
	if reg.Procs == nil {
		reg.Procs = eng.NewProcessRuntime()
	}
	squatter := eng.NewProcess(reg.Procs, 4, eng.OverflowDrop)
	if err := reg.Procs.Insert(squatter); err != nil {
		t.Fatal(err)
	}
	if err := reg.Procs.RegisterName(tuiRegisteredName, squatter); err != nil {
		t.Fatal(err)
	}
	cfg := native.NewOrderedMap()
	_, rErr := tuiRunHandler([]native.Value{native.NewMap(cfg)}, nil, nil, reg)
	if rErr == nil || !strings.Contains(rErr.Error(), "missing update") {
		t.Fatalf("precheck = %v", rErr)
	}
	// with a well-formed app, the name squat rejects the run
	_, err2 := runTuiStepsOn(t, reg, []string{
		`import "boru:tui"`,
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`,
	})
	if err2 == nil || !strings.Contains(err2.Error(), "already owns the terminal") {
		t.Fatalf("already running = %v", err2)
	}
	squatter.Close()
}

// runTuiStepsOn runs steps on a caller-prepared registry.
func runTuiStepsOn(t *testing.T, reg *native.Registry, steps []string) ([]native.Value, error) {
	t.Helper()
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	engine := native.NewTop(reg)
	var result []native.Value
	var err error
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

// The pure Tier-2 words: constructors by shape, the edit fold, the
// focusable walk, the style merge — engine-level so dispatch is proven.
func TestTuiWidgetWordsEndToEnd(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def tw (Tui.text "hi" {size: 1})`,
		`def ed (Tui.edit (Tui.input "ab" {focus: true}) {tag: "key"  key: "backspace"  char: ""  mods: []})`,
		`def ids (Tui.focusable (Tui.rows [ (Tui.input "x" {id: "a"}) (Tui.box (Tui.input "y" {id: "b"})) ]))`,
		`def st (Tui.style {fg: "red"  bold: true} {fg: "blue"})`,
		`def qm (Tui.quit 7)`,
		`def sp Tui.spacer`,
		`def vp (Tui.viewport (Tui.text "v") {offset: [1 0]})`,
		`def tb (Tui.table [{title: "c"}] [[1]] {cursor: 0})`,
		`def lv (Tui.list-view ["a" "b"] {cursor: 1})`,
		`def co (Tui.cols [ sp ] {gap: 2})`,
		`join "|" [tw.w tw.text (convert String tw.size) ed.value (convert String ed.cursor)
		           (convert String (size ids)) (ids get 0) (ids get 1) st.fg (convert String st.bold)
		           (convert String (qm get "@quit")) sp.w vp.w tb.w lv.w co.w (convert String co.gap)]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := tuiLastString(t, out)
	want := "text|hi|1|a|1|2|a|b|blue|true|7|spacer|viewport|table|list-view|cols|2"
	if got != want {
		t.Fatalf("widget words = %q\nwant          %q", got, want)
	}
}

// Constructor rejections, one per guard.
func TestTuiWidgetWordRejections(t *testing.T) {
	reg := tcReg(t)
	i := native.NewInteger
	m := func() native.Value { return native.NewMap(native.NewOrderedMap()) }
	cases := []struct {
		name string
		err  error
	}{
		{"text", func() error { _, e := tuiTextHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"rows", func() error { _, e := tuiRowsHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"cols", func() error { _, e := tuiColsHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"box", func() error { _, e := tuiBoxHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"list-view", func() error { _, e := tuiListViewHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"table-cols", func() error { _, e := tuiTableHandler([]native.Value{i(1), i(2)}, nil, nil, reg); return e }()},
		{"table-rows", func() error {
			_, e := tuiTableHandler([]native.Value{native.NewList([]native.Value{}), i(2)}, nil, nil, reg)
			return e
		}()},
		{"input", func() error { _, e := tuiInputHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"viewport", func() error { _, e := tuiViewportHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"style-base", func() error { _, e := tuiStyleHandler([]native.Value{i(1), m()}, nil, nil, reg); return e }()},
		{"style-over", func() error { _, e := tuiStyleHandler([]native.Value{m(), i(1)}, nil, nil, reg); return e }()},
		{"edit-input", func() error { _, e := tuiEditHandler([]native.Value{i(1), m()}, nil, nil, reg); return e }()},
		{"edit-event", func() error { _, e := tuiEditHandler([]native.Value{m(), i(1)}, nil, nil, reg); return e }()},
		{"focusable", func() error { _, e := tuiFocusableHandler([]native.Value{i(1)}, nil, nil, reg); return e }()},
		{"opts", func() error {
			_, e := tuiTextHandler([]native.Value{native.NewString("x"), i(1)}, nil, nil, reg)
			return e
		}()},
	}
	for _, c := range cases {
		if c.err == nil || tcCode(t, c.err) != "tui_error" {
			t.Errorf("%s: err = %v", c.name, c.err)
		}
	}
}

// The edit fold's remaining moves.
func TestTuiEditFoldMoves(t *testing.T) {
	mkInput := func(value string, cursor int) native.Value {
		m := native.NewOrderedMap()
		m.Set("w", native.NewString("input"))
		m.Set("value", native.NewString(value))
		m.Set("cursor", native.NewInteger(int64(cursor)))
		return native.NewMap(m)
	}
	mkKey := func(k, char string, mods ...string) native.Value {
		m := native.NewOrderedMap()
		m.Set("tag", native.NewString("key"))
		m.Set("key", native.NewString(k))
		m.Set("char", native.NewString(char))
		var mv []native.Value
		for _, mod := range mods {
			mv = append(mv, native.NewString(mod))
		}
		m.Set("mods", native.NewList(mv))
		return native.NewMap(m)
	}
	reg := tcReg(t)
	fold := func(in native.Value, ev native.Value) (string, int) {
		out, err := tuiEditHandler([]native.Value{in, ev}, nil, nil, reg)
		if err != nil {
			t.Fatal(err)
		}
		mp, _ := native.AsMap(out[0])
		v, _ := mp.Get("value")
		c, _ := mp.Get("cursor")
		s, _ := v.AsConcreteString()
		n, _ := c.AsConcreteInteger()
		return s, int(n)
	}
	if v, c := fold(mkInput("a漢b", 1), mkKey("x", "x")); v != "ax漢b" || c != 2 {
		t.Errorf("insert = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("delete", "")); v != "a" || c != 1 {
		t.Errorf("delete = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("left", "")); v != "ab" || c != 0 {
		t.Errorf("left = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("right", "")); v != "ab" || c != 2 {
		t.Errorf("right = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("home", "")); v != "ab" || c != 0 {
		t.Errorf("home = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("end", "")); v != "ab" || c != 2 {
		t.Errorf("end = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 99), mkKey("f5", "")); v != "ab" || c != 2 {
		t.Errorf("clamp+ignore = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", -3), mkKey("backspace", "")); v != "ab" || c != 0 {
		t.Errorf("negative-cursor backspace = %q,%d", v, c)
	}
	if v, c := fold(mkInput("ab", 1), mkKey("c", "c", "ctrl")); v != "ab" || c != 1 {
		t.Errorf("chord ignored = %q,%d", v, c)
	}
}
