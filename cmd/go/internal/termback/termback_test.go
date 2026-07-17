package termback

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"io"

	"golang.org/x/term"

	"github.com/aql-lang/aql/lang/go/tuikit"
)

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("write refused")
	}
	f.after--
	return len(p), nil
}

// swapSeams installs happy-path fakes and returns restore state via
// t.Cleanup (design/TEST-SEAMS.10.md discipline).
func swapSeams(t *testing.T, in *bytes.Buffer, out *bytes.Buffer) (*int, *int) {
	t.Helper()
	oldIsTerm, oldRaw, oldRestore, oldSize := isTerminal, makeRaw, restore, getSize
	oldIn, oldOut := ttyIn, ttyOut
	t.Cleanup(func() {
		isTerminal, makeRaw, restore, getSize = oldIsTerm, oldRaw, oldRestore, oldSize
		ttyIn, ttyOut = oldIn, oldOut
	})
	raws, restores := 0, 0
	isTerminal = func(int) bool { return true }
	makeRaw = func(int) (*term.State, error) { raws++; return &term.State{}, nil }
	restore = func(int, *term.State) error { restores++; return nil }
	getSize = func(int) (int, int, error) { return 10, 4, nil }
	ttyIn = func() (io.Reader, int) { return in, 0 }
	ttyOut = func() (io.Writer, int) { return out, 1 }
	return &raws, &restores
}

func TestSpecShape(t *testing.T) {
	spec := Spec()
	if spec.Name != "tty" || spec.Open == nil {
		t.Fatalf("spec = %+v", spec)
	}
	backend, err := spec.Open()
	if err != nil || backend == nil {
		t.Fatalf("spec.Open = %v, %v", backend, err)
	}
}

func TestOpenLifecycleAndRejections(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	raws, restores := swapSeams(t, in, out)

	b := New()
	info, err := b.Open(tuikit.OpenOpts{Mouse: true, Title: "boot"})
	if err != nil || info.Cols != 10 || info.Rows != 4 {
		t.Fatalf("Open = %+v, %v", info, err)
	}
	setup := out.String()
	for _, want := range []string{seqAltScreenOn, seqCursorHide, seqPasteOn, seqClear, seqMouseOn, "\x1b]0;boot\x07"} {
		if !strings.Contains(setup, want) {
			t.Errorf("setup missing %q", want)
		}
	}
	if *raws != 1 {
		t.Errorf("raw calls = %d", *raws)
	}
	if _, err := b.Open(tuikit.OpenOpts{}); err == nil {
		t.Error("second Open accepted")
	}
	if c, r, err := b.Size(); err != nil || c != 10 || r != 4 {
		t.Errorf("Size = %d,%d,%v", c, r, err)
	}
	if b.Events() == nil {
		t.Error("Events nil after open")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	teardown := out.String()
	for _, want := range []string{seqMouseOff, seqPasteOff, seqCursorShow, seqAltScreenOff} {
		if !strings.Contains(teardown, want) {
			t.Errorf("teardown missing %q", want)
		}
	}
	if *restores != 1 {
		t.Errorf("restore calls = %d", *restores)
	}
	if err := b.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
	if _, err := b.Open(tuikit.OpenOpts{}); err == nil {
		t.Error("Open after Close accepted")
	}

	// a never-opened backend closes as a no-op
	b2 := New()
	if err := b2.Close(); err != nil {
		t.Errorf("unopened Close: %v", err)
	}
}

func TestOpenFailureArms(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	_, restores := swapSeams(t, in, out)

	isTerminal = func(int) bool { return false }
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "not_a_tty") {
		t.Fatalf("non-tty = %v", err)
	}
	isTerminal = func(int) bool { return true }

	makeRaw = func(int) (*term.State, error) { return nil, errors.New("no raw") }
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "no raw") {
		t.Fatalf("raw failure = %v", err)
	}
	makeRaw = func(int) (*term.State, error) { return &term.State{}, nil }

	getSize = func(int) (int, int, error) { return 0, 0, errors.New("no size") }
	before := *restores
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "no size") {
		t.Fatalf("size failure = %v", err)
	}
	if *restores != before+1 {
		t.Error("size failure did not restore raw mode")
	}
	getSize = func(int) (int, int, error) { return 10, 4, nil }

	fw := &failWriter{}
	ttyOut = func() (io.Writer, int) { return fw, 1 }
	before = *restores
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil {
		t.Fatal("setup write failure accepted")
	}
	if *restores != before+1 {
		t.Error("setup write failure did not restore raw mode")
	}
}

func TestPresentDamageAndFailure(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	f := tuikit.NewFrame(4, 1)
	f.Set(0, 0, tuikit.Cell{Content: "h", Width: 1})
	f.Set(1, 0, tuikit.Cell{Content: "i", Width: 1, Style: tuikit.Style{Bold: true}})
	f.Set(2, 0, tuikit.Cell{Content: "漢", Width: 2})
	f.Set(3, 0, tuikit.Cell{Width: -1})
	if err := b.Present(f); err != nil {
		t.Fatal(err)
	}
	first := out.String()
	if !strings.Contains(first, "\x1b[1;1H") {
		t.Errorf("no cursor move: %q", first)
	}
	if !strings.Contains(first, "h") || !strings.Contains(first, "漢") {
		t.Errorf("content missing: %q", first)
	}
	if !strings.Contains(first, "\x1b[1m") {
		t.Errorf("style run missing: %q", first)
	}
	out.Reset()
	if err := b.Present(f); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("unchanged frame emitted %q", out.String())
	}
	// a blank cell paints a space
	f2 := f.Clone()
	f2.Set(0, 0, tuikit.Cell{})
	out.Reset()
	if err := b.Present(f2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), " ") {
		t.Errorf("blank cell not spaced: %q", out.String())
	}

	fw := &failWriter{}
	b.out = fw
	f3 := f2.Clone()
	f3.Set(1, 0, tuikit.Cell{Content: "z", Width: 1})
	if err := b.Present(f3); err == nil {
		t.Error("present write failure accepted")
	}
}

func TestCursorTitleBell(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := b.SetCursor(2, 1, true); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "\x1b[2;3H") || !strings.Contains(got, seqCursorShow) {
		t.Errorf("cursor show = %q", got)
	}
	out.Reset()
	if err := b.SetCursor(0, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != seqCursorHide {
		t.Errorf("cursor hide = %q", got)
	}
	if err := b.SetTitle("t2"); err != nil {
		t.Fatal(err)
	}
	if err := b.Bell(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "\x1b]0;t2\x07") || !strings.Contains(got, "\x07") {
		t.Errorf("title/bell = %q", got)
	}
	b.out = &failWriter{}
	if err := b.SetCursor(0, 0, true); err == nil {
		t.Error("cursor write failure accepted")
	}
	if err := b.SetCursor(0, 0, false); err == nil {
		t.Error("cursor hide write failure accepted")
	}
	if err := b.SetTitle("x"); err == nil {
		t.Error("title write failure accepted")
	}
	if err := b.Bell(); err == nil {
		t.Error("bell write failure accepted")
	}
	// Close's teardown write failure surfaces
	if err := b.Close(); err == nil {
		t.Error("teardown write failure accepted")
	}
}

// --- the decoder ---

func decodeAll(t *testing.T, input string) []tuikit.Event {
	t.Helper()
	ch := make(chan tuikit.Event, 64)
	var closed atomic.Bool
	decodeInput(bufio.NewReader(strings.NewReader(input)), ch, &closed)
	var out []tuikit.Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestDecodeKeys(t *testing.T) {
	evs := decodeAll(t, "a \r\n\t\x7f\x08\x03\x1c\x00é漢")
	want := []tuikit.Event{
		{Tag: "key", Key: "a", Char: "a"},
		{Tag: "key", Key: "space", Char: " "},
		{Tag: "key", Key: "enter"},
		{Tag: "key", Key: "enter"},
		{Tag: "key", Key: "tab"},
		{Tag: "key", Key: "backspace"},
		{Tag: "key", Key: "backspace"},
		{Tag: "key", Key: "c", Mods: []string{"ctrl"}},
		{Tag: "key", Key: `\`, Mods: []string{"ctrl"}},
		{Tag: "key", Key: "é", Char: "é"},
		{Tag: "key", Key: "漢", Char: "漢"},
	}
	if len(evs) != len(want) {
		t.Fatalf("events = %+v", evs)
	}
	for i := range want {
		if evs[i].Tag != want[i].Tag || evs[i].Key != want[i].Key || evs[i].Char != want[i].Char {
			t.Errorf("ev %d = %+v, want %+v", i, evs[i], want[i])
		}
		if len(want[i].Mods) > 0 && (len(evs[i].Mods) == 0 || evs[i].Mods[0] != want[i].Mods[0]) {
			t.Errorf("ev %d mods = %v", i, evs[i].Mods)
		}
	}
}

func TestDecodeSequences(t *testing.T) {
	cases := []struct {
		in   string
		key  string
		tag  string
		text string
		kind string
	}{
		{"\x1b[A", "up", "key", "", ""},
		{"\x1b[H", "home", "key", "", ""},
		{"\x1b[Z", "backtab", "key", "", ""},
		{"\x1b[1;5C", "right", "key", "", ""},
		{"\x1b[3~", "delete", "key", "", ""},
		{"\x1b[15~", "f5", "key", "", ""},
		{"\x1bOA", "up", "key", "", ""},
		{"\x1bOP", "f1", "key", "", ""},
		{"\x1bOS", "f4", "key", "", ""},
		{"\x1bx", "x", "key", "", ""},
		{"\x1b", "esc", "key", "", ""},
		{"\x1b[200~hi there\x1b[201~", "", "paste", "hi there", ""},
		{"\x1b[<0;3;2M", "", "mouse", "", "press"},
		{"\x1b[<0;3;2m", "", "mouse", "", "release"},
		{"\x1b[<64;1;1M", "", "mouse", "", "wheel-up"},
		{"\x1b[<65;1;1M", "", "mouse", "", "wheel-down"},
		{"\x1b[<32;5;5M", "", "mouse", "", "move"},
	}
	for _, c := range cases {
		evs := decodeAll(t, c.in)
		if len(evs) != 1 {
			t.Fatalf("%q events = %+v", c.in, evs)
		}
		ev := evs[0]
		if ev.Tag != c.tag || ev.Key != c.key || ev.Text != c.text || ev.Kind != c.kind {
			t.Errorf("%q = %+v", c.in, ev)
		}
	}
	if evs := decodeAll(t, "\x1b[<0;3;2M"); evs[0].X != 2 || evs[0].Y != 1 {
		t.Errorf("mouse coords = %+v", evs[0])
	}
}

func TestDecodeDropsAndErrors(t *testing.T) {
	// dropped shapes decode to nothing
	for _, in := range []string{
		"\x1b[99~",    // unknown tilde
		"\x1b[5X",     // unhandled CSI final
		"\x1bOz",      // unknown SS3
		"\x1b[<;1;1M", // malformed mouse params (empty button field)
		"\x1b[<0;1M",  // short mouse params
	} {
		if evs := decodeAll(t, in); len(evs) != 0 {
			t.Errorf("%q delivered %+v", in, evs)
		}
	}
	// truncated sequences end the decoder without panic
	for _, in := range []string{"\x1b[", "\x1bO", "\x1b[200~ab", "\x1b[200~a\x1b"} {
		if evs := decodeAll(t, in); len(evs) != 0 {
			t.Errorf("truncated %q delivered %+v", in, evs)
		}
	}
	// a closed backend drops the event and stops the decoder
	ch := make(chan tuikit.Event, 4)
	var closed atomic.Bool
	closed.Store(true)
	decodeInput(bufio.NewReader(strings.NewReader("abc")), ch, &closed)
	if _, ok := <-ch; ok {
		t.Error("closed decoder delivered an event")
	}
}
