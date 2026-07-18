package modules

import (
	"encoding/json"
	"fmt"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// BenchmarkTuiKeystrokeStorm floods one driver session with key events
// and reports how many frames the §2.2 drain-coalescing actually paints
// per keystroke — the render-on-change check of the implementation
// plan's P5: a storm folds per event but renders per BATCH, so
// renders/keystroke must stay well under 1 as the storm grows.
func BenchmarkTuiKeystrokeStorm(b *testing.B) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	engine := native.NewTop(reg)
	var out []native.Value
	for _, step := range []string{
		`import "aql:tui"`,
		`def u ([s:Any e:Map] => [ if (e.key eq "q") [ Tui.quit s ] [ (s add 1) ] ])`,
		`def v ([s:Any] => [ Tui.text (convert String s) ])`,
		`[u/r v/r]`,
	} {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			b.Fatal(pErr)
		}
		if out, err = engine.Run(vals); err != nil {
			b.Fatal(err)
		}
	}
	fns, _ := native.AsList(out[len(out)-1])
	app := &tuiApp{init: native.NewInteger(0), update: fns.Get(0), view: fns.Get(1)}
	app.updateInfo, _ = native.FnDefFromValue(app.update)
	app.viewInfo, _ = native.FnDefFromValue(app.view)
	if reg.Procs == nil {
		reg.Procs = eng.NewProcessRuntime()
	}
	proc := eng.NewProcess(reg.Procs, eng.DefaultMailboxBound, eng.OverflowBlock)
	if err := reg.Procs.Insert(proc); err != nil {
		b.Fatal(err)
	}

	events := make(chan tuikit.Event, b.N+1)
	for i := 0; i < b.N; i++ {
		events <- tuikit.Event{Tag: "key", Key: "x", Char: "x"}
	}
	events <- tuikit.Event{Tag: "key", Key: "q", Char: "q"}
	close(events)

	renders := 0
	d := &tuiDriver{
		reg: reg, app: app, proc: proc,
		events: events,
		paint:  func(native.Value, int, int) error { renders++; return nil },
		finish: func() {},
		cols:   80, rows: 24,
	}
	b.ResetTimer()
	final, rErr := d.run()
	b.StopTimer()
	if rErr != nil {
		b.Fatal(rErr)
	}
	// every keystroke folded (init +1, then one per storm key)
	if n, _ := final.AsConcreteInteger(); n != int64(b.N)+1 {
		b.Fatalf("final = %v, want %d", final, b.N+1)
	}
	if renders > b.N {
		b.Fatalf("renders = %d for %d keystrokes — coalescing is off", renders, b.N)
	}
	b.ReportMetric(float64(renders)/float64(b.N), "renders/keystroke")
}

// BenchmarkTuiWireBytes answers RFC §11.5's trigger question with
// numbers: how many bytes does one full-tree frame cost on the §6.2
// wire at realistic app sizes? Trees are the graduated todo app's
// shape (a box+list-view+status page) scaled by list length. The
// unchanged-frame suppression (P6) makes an IDLE session cost zero
// regardless of these numbers — this measures the CHANGING case.
func BenchmarkTuiWireBytes(b *testing.B) {
	for _, rows := range []int{50, 200, 1000} {
		b.Run(fmt.Sprintf("rows-%d", rows), func(b *testing.B) {
			items := make([]any, rows)
			for i := range items {
				items[i] = fmt.Sprintf("[ ] todo item number %d with realistic text", i)
			}
			tree := map[string]any{
				"w": "rows",
				"children": []any{
					map[string]any{"w": "box", "title": "new todo", "border": "round", "size": 3,
						"child": map[string]any{"w": "input", "value": "milk", "cursor": 4, "placeholder": "", "focus": true}},
					map[string]any{"w": "box", "title": "todos", "border": "round", "size": map[string]any{"flex": 1},
						"child": map[string]any{"w": "list-view", "items": items, "cursor": rows / 2}},
					map[string]any{"w": "text", "text": "up/down move · space toggle · q quit", "size": 1},
				},
			}
			var bytesPerFrame int
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := json.Marshal(tree)
				if err != nil {
					b.Fatal(err)
				}
				bytesPerFrame = len(data)
			}
			b.StopTimer()
			b.ReportMetric(float64(bytesPerFrame), "bytes/frame")
		})
	}
}
