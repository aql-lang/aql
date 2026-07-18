//go:build js && wasm

// tuiweb.go — the browser Backend for aql:tui (the RFC §9 playground
// tier, closed by the P7 follow-ups): frames render into the page as
// styled HTML rows, key events flow back from the page's keydown
// handler. The playground page provides the JS half as global hooks —
// aqlTuiOpen/aqlTuiFrame/aqlTuiCursor/aqlTuiTitle/aqlTuiBell/
// aqlTuiClose — and calls the Go-exposed aqlTuiKey/aqlTuiResize to
// feed input. Missing hooks are tolerated (the backend no-ops), so the
// engine build never depends on page structure.
package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"syscall/js"

	"github.com/aql-lang/aql/lang/go/tuikit"
)

// webBackend implements tuikit.Backend on the playground page. One
// instance per Tui.open/run; the LAST opened instance owns the
// aqlTuiKey/aqlTuiResize input hooks (a playground shows one app).
// mu serializes event sends with Close: a JS input callback must never
// send on a channel the app goroutine is concurrently closing.
type webBackend struct {
	events chan tuikit.Event
	cols   int
	rows   int
	closed atomic.Bool
	mu     sync.Mutex
}

// webCurrent is the instance whose events channel the page feeds.
var (
	webCurrent   *webBackend
	webCurrentMu sync.Mutex
)

func newWebBackend() *webBackend {
	return &webBackend{events: make(chan tuikit.Event, 64), cols: 80, rows: 24}
}

// callHook invokes a page-provided hook if it exists.
func callHook(name string, args ...any) js.Value {
	fn := js.Global().Get(name)
	if fn.IsUndefined() || fn.IsNull() {
		return js.Undefined()
	}
	return fn.Invoke(args...)
}

func (w *webBackend) Open(opts tuikit.OpenOpts) (tuikit.Info, error) {
	if w.closed.Load() {
		return tuikit.Info{}, errors.New("web terminal is closed")
	}
	webCurrentMu.Lock()
	webCurrent = w
	webCurrentMu.Unlock()
	dims := callHook("aqlTuiOpen", opts.Title)
	if dims.Type() == js.TypeObject {
		if c := dims.Get("cols"); c.Type() == js.TypeNumber && c.Int() > 0 {
			w.cols = c.Int()
		}
		if r := dims.Get("rows"); r.Type() == js.TypeNumber && r.Int() > 0 {
			w.rows = r.Int()
		}
	}
	return tuikit.Info{Cols: w.cols, Rows: w.rows}, nil
}

func (w *webBackend) Close() error {
	w.mu.Lock()
	if w.closed.Swap(true) {
		w.mu.Unlock()
		return nil
	}
	close(w.events)
	w.mu.Unlock()
	callHook("aqlTuiClose")
	return nil
}

func (w *webBackend) Size() (int, int, error) {
	return w.cols, w.rows, nil
}

func (w *webBackend) Events() <-chan tuikit.Event { return w.events }

func (w *webBackend) Present(f *tuikit.Frame) error {
	if w.closed.Load() {
		return errors.New("web terminal is closed")
	}
	callHook("aqlTuiFrame", frameHTML(f))
	return nil
}

func (w *webBackend) SetCursor(x, y int, visible bool) error {
	callHook("aqlTuiCursor", x, y, visible)
	return nil
}

func (w *webBackend) SetTitle(title string) error {
	callHook("aqlTuiTitle", title)
	return nil
}

func (w *webBackend) Bell() error {
	callHook("aqlTuiBell")
	return nil
}

// deliver feeds one event to the CURRENT backend, shedding on overflow
// (the run loop coalesces anyway). The closed re-check happens under
// the same mutex Close holds while closing the channel, so a late JS
// callback drops its event instead of panicking on a closed send.
func webDeliver(ev tuikit.Event) {
	webCurrentMu.Lock()
	w := webCurrent
	webCurrentMu.Unlock()
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed.Load() {
		return
	}
	select {
	case w.events <- ev:
	default:
	}
}

// installTuiInput exposes the page-callable input feeders.
func installTuiInput() {
	js.Global().Set("aqlTuiKey", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		var mods []string
		if len(args) > 2 && args[2].Truthy() {
			mods = append(mods, "ctrl")
		}
		if len(args) > 3 && args[3].Truthy() {
			mods = append(mods, "alt")
		}
		if len(args) > 4 && args[4].Truthy() {
			mods = append(mods, "shift")
		}
		webDeliver(tuikit.Event{Tag: "key", Key: args[0].String(), Char: args[1].String(), Mods: mods})
		return nil
	}))
	js.Global().Set("aqlTuiResize", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		webDeliver(tuikit.Event{Tag: "resize", Cols: args[0].Int(), Rows: args[1].Int()})
		return nil
	}))
}

// ---- frame → HTML ----

// ansi16 is the classic palette as CSS colors (indexes 0–15).
var ansi16 = [16]string{
	"#000000", "#cd3131", "#0dbc79", "#e5e510", "#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
	"#666666", "#f14c4c", "#23d18b", "#f5f543", "#3b8eea", "#d670d6", "#29b8db", "#ffffff",
}

// cssColor maps a style color string onto a CSS color ("" = default).
func cssColor(s string) string {
	col, ok := tuikit.ParseColor(s)
	if !ok {
		return ""
	}
	switch col.Kind {
	case tuikit.ColorNamed:
		return ansi16[col.Index&15]
	case tuikit.ColorIndex:
		return css256(col.Index)
	case tuikit.ColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", col.R, col.G, col.B)
	}
	return ""
}

// css256 expands a 256-palette index by the xterm formula.
func css256(n int) string {
	switch {
	case n < 16:
		return ansi16[n&15]
	case n < 232:
		n -= 16
		steps := [6]int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", steps[n/36], steps[n/6%6], steps[n%6])
	default:
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// frameHTML renders a frame as one HTML string: rows of styled spans,
// adjacent same-style cells merged. Wide-rune continuation cells
// (Width -1) are skipped — the wide rune spans them visually.
func frameHTML(f *tuikit.Frame) string {
	var b strings.Builder
	for y := 0; y < f.Rows; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		var runText strings.Builder
		runStyle := ""
		flush := func() {
			if runText.Len() == 0 {
				return
			}
			if runStyle == "" {
				b.WriteString(escapeHTML(runText.String()))
			} else {
				b.WriteString(`<span style="` + runStyle + `">`)
				b.WriteString(escapeHTML(runText.String()))
				b.WriteString(`</span>`)
			}
			runText.Reset()
		}
		for x := 0; x < f.Cols; x++ {
			cell, ok := f.At(x, y)
			if !ok || cell.Width < 0 {
				continue
			}
			st := cellCSS(cell.Style)
			if st != runStyle {
				flush()
				runStyle = st
			}
			if cell.Content == "" {
				runText.WriteByte(' ')
			} else {
				runText.WriteString(cell.Content)
			}
		}
		flush()
	}
	return b.String()
}

// cellCSS renders one cell style as inline CSS ("" for the default).
func cellCSS(st tuikit.Style) string {
	fg, bg := cssColor(st.FG), cssColor(st.BG)
	if st.Reverse {
		if fg == "" {
			fg = "#e6e6e6"
		}
		if bg == "" {
			bg = "#101418"
		}
		fg, bg = bg, fg
	}
	var parts []string
	if fg != "" {
		parts = append(parts, "color:"+fg)
	}
	if bg != "" {
		parts = append(parts, "background:"+bg)
	}
	if st.Bold {
		parts = append(parts, "font-weight:bold")
	}
	if st.Italic {
		parts = append(parts, "font-style:italic")
	}
	if st.Underline {
		parts = append(parts, "text-decoration:underline")
	}
	return strings.Join(parts, ";")
}
