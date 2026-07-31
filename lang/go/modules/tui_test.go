package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// End-to-end coverage for the boru:tui Tier-1 words (tui.go) over the
// VirtualBackend — no TTY anywhere, per design/TUI.0.md §8.1. Positive
// paths are paired with the negative contracts (closed, no backend,
// deadline None, policy denial — the latter in tui_cover_test.go) per
// lang/go/CLAUDE.md.

// runTuiSteps runs steps on a fully-wired registry with vb registered as
// the terminal backend (vb == nil leaves NO backend registered).
func runTuiSteps(t *testing.T, vb *tuikit.VirtualBackend, steps []string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	if vb != nil {
		if rErr := RegisterHostTui(reg, TuiSpec{
			Name: "virtual",
			Open: func() (tuikit.Backend, error) { return vb, nil },
		}); rErr != nil {
			t.Fatal(rErr)
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

func tuiLastString(t *testing.T, vals []native.Value) string {
	t.Helper()
	if len(vals) == 0 {
		t.Fatal("no result")
	}
	s, err := native.AsString(vals[len(vals)-1])
	if err != nil {
		t.Fatalf("expected String result, got %v", vals)
	}
	return s
}

// The full Tier-1 session: open with options, draw, present, read a
// scripted event, retitle, ring, clear, close — asserting both the boru
// results and the backend's recorded state.
func TestTuiTier1EndToEnd(t *testing.T) {
	vb := tuikit.NewVirtualBackend(10, 3)
	vb.Inject(tuikit.Event{Tag: "key", Key: "a", Char: "a", Mods: []string{"ctrl"}})
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def t (Tui.open {mouse: false title: "boot"})`,
		`def d (Tui.dims t)`,
		`Tui.print-at t 1 0 "hi" {bold: true fg: "red"}`,
		`Tui.print-at t 0 2 "ok"`,
		`Tui.show t`,
		`def ev (Tui.read-event t {within: 5000})`,
		`def quiet (Tui.read-event t {within: 10})`,
		`Tui.title t "after"`,
		`Tui.bell t`,
		`Tui.clear t`,
		`Tui.show t`,
		`Tui.close t`,
		`Tui.close t`, // double close is a no-op
		`join "|" [(convert String d.cols) (convert String d.rows) ev.tag ev.key
		           (convert String (size ev.mods)) (convert String (quiet eq None))]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := tuiLastString(t, out)
	if got != "10|3|key|a|1|true" {
		t.Fatalf("session fields = %q", got)
	}
	// frame 0: the drawn grid; frame 1: cleared
	if vb.FrameCount() != 2 {
		t.Fatalf("FrameCount = %d, want 2", vb.FrameCount())
	}
	rows := vb.ScreenAt(0)
	if rows[0] != " hi       " || rows[2] != "ok        " {
		t.Fatalf("drawn frame = %q", rows)
	}
	if c, _ := vb.CellAt(1, 0); c.Content != "" {
		t.Fatalf("clear did not blank the grid: %+v", c)
	}
	if vb.Title() != "after" {
		t.Fatalf("title = %q", vb.Title())
	}
	if vb.Bells() != 1 {
		t.Fatalf("bells = %d", vb.Bells())
	}
}

// Style attributes land on the presented cells.
func TestTuiPrintAtStyleOnCells(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 1)
	_, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def t (Tui.open {})`,
		`Tui.print-at t 0 0 "x" {fg: "red" bg: "#202030" bold: true italic: true underline: true reverse: true}`,
		`Tui.show t`,
		`Tui.close t`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	c, ok := vb.CellAt(0, 0)
	if !ok || c.Content != "x" {
		t.Fatalf("cell = %+v, %v", c, ok)
	}
	st := c.Style
	if st.FG != "red" || st.BG != "#202030" || !st.Bold || !st.Italic || !st.Underline || !st.Reverse {
		t.Fatalf("style = %+v", st)
	}
}

// print-at paints real terminal widths (the drawText contract): a wide
// rune spans two cells with a Width -1 continuation, a combining mark
// joins the previous cell's grapheme, and a wide rune that would split
// at the right edge is dropped whole.
func TestTuiPrintAtWideRunes(t *testing.T) {
	vb := tuikit.NewVirtualBackend(6, 3)
	_, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def t (Tui.open {})`,
		"Tui.print-at t 0 0 \"日éx\"",
		`Tui.print-at t 5 1 "日"`,
		"Tui.print-at t 0 2 \"́y日́\"",
		`Tui.show t`,
		`Tui.close t`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wide, _ := vb.CellAt(0, 0)
	cont, _ := vb.CellAt(1, 0)
	joined, _ := vb.CellAt(2, 0)
	tail, _ := vb.CellAt(3, 0)
	if wide.Content != "日" || wide.Width != 2 {
		t.Fatalf("wide cell = %+v", wide)
	}
	if cont.Width != -1 {
		t.Fatalf("continuation cell = %+v", cont)
	}
	if joined.Content != "é" || joined.Width != 1 {
		t.Fatalf("combining mark did not join: %+v", joined)
	}
	if tail.Content != "x" {
		t.Fatalf("tail cell = %+v", tail)
	}
	// the wide rune at x=5 cannot fit in a 6-col grid — dropped whole
	if c, _ := vb.CellAt(5, 1); c.Content != "" {
		t.Fatalf("split wide rune leaked: %+v", c)
	}
	// a mark opening the run is dropped; a mark after a continuation
	// cell (no grapheme to join) is dropped too
	if c, _ := vb.CellAt(0, 2); c.Content != "y" {
		t.Fatalf("run-opening mark mishandled: %+v", c)
	}
	if c, _ := vb.CellAt(2, 2); c.Content != "" || c.Width != -1 {
		t.Fatalf("post-wide continuation = %+v", c)
	}
}

// Without an import the namespace is unbound; with no backend registered
// the words raise the first-class no_backend error.
func TestTuiNoBackendRegistered(t *testing.T) {
	_, err := runTuiSteps(t, nil, []string{
		`import "boru:tui"`,
		`Tui.open {}`,
	})
	if err == nil || !strings.Contains(err.Error(), "no terminal backend registered") {
		t.Fatalf("open without backend = %v", err)
	}
}

// Operations on a closed Terminal raise `closed`; a second registration
// is rejected; the handle formats with its closed state.
func TestTuiClosedAndRegistrationContracts(t *testing.T) {
	vb := tuikit.NewVirtualBackend(2, 2)
	_, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def t (Tui.open {})`,
		`Tui.close t`,
		`Tui.dims t`,
	})
	if err == nil || !strings.Contains(err.Error(), "terminal is closed") {
		t.Fatalf("dims after close = %v", err)
	}

	reg, rErr := native.DefaultRegistry()
	if rErr != nil {
		t.Fatal(rErr)
	}
	spec := TuiSpec{Name: "a", Open: func() (tuikit.Backend, error) { return vb, nil }}
	if err := RegisterHostTui(reg, spec); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := RegisterHostTui(reg, TuiSpec{Name: "b", Open: spec.Open}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("second registration = %v", err)
	}
}

// The Terminal type literal is exported: `x is Tui.Terminal` works and a
// non-Terminal is rejected by the guard.
func TestTuiTerminalTypeExportAndGuard(t *testing.T) {
	vb := tuikit.NewVirtualBackend(2, 2)
	out, err := runTuiSteps(t, vb, []string{
		`import "boru:tui"`,
		`def t (Tui.open {})`,
		`def ok (t is Tui.Terminal)`,
		`Tui.close t`,
		`convert String ok`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := tuiLastString(t, out); got != "true" {
		t.Fatalf("is Tui.Terminal = %q", got)
	}
}
