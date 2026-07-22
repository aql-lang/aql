// Package termback is the real-TTY tuikit.Backend (TUI.0.md §4.4,
// implementation plan P3): raw mode + alt-screen via x/term, escape
// emission and input decoding hand-rolled over the tuikit primitives
// (DiffFrames + SGR are the shared damage/style sources, so the TTY
// and the virtual backend can never disagree). Every OS touchpoint sits
// behind a package seam (design/TEST-SEAMS.10.md) so the whole backend
// is exercised on a terminal-less CI.
package termback

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"

	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// Test seams — each defaults to the real implementation.
var (
	isTerminal = term.IsTerminal
	makeRaw    = term.MakeRaw
	restore    = term.Restore
	getSize    = term.GetSize
	ttyIn      = func() (io.Reader, int) { return os.Stdin, int(os.Stdin.Fd()) }
	ttyOut     = func() (io.Writer, int) { return os.Stdout, int(os.Stdout.Fd()) }
	// resizeSignal yields a channel that ticks when the terminal size
	// changes (SIGWINCH on unix; nil where unsupported). stop ends the
	// underlying notification.
	resizeSignal = platformResizeSignal
	// interruptRead unblocks a decoder blocked in Read during Close so
	// teardown never eats the NEXT keystroke typed after the app exits.
	// Best-effort: a deadline poke on real files, a no-op elsewhere.
	interruptRead = func(in io.Reader) {
		if f, ok := in.(*os.File); ok {
			_ = f.SetReadDeadline(time.Now())
		}
	}
)

// ttyBusy is the process-wide single-owner latch: one raw-mode
// takeover of the process TTY at a time. A second concurrent Open —
// two `Tui.open {}` calls, or an open racing a running app — fails
// loudly instead of stacking raw modes and competing input decoders.
var ttyBusy atomic.Bool

// Spec is what the CLI registers on every registry it builds
// (buildrt / the REPL): the words resolve it at dispatch time.
func Spec() modules.TuiSpec {
	return modules.TuiSpec{Name: "tty", Open: func() (tuikit.Backend, error) { return New(), nil }}
}

// Backend drives one real terminal.
type Backend struct {
	in    io.Reader
	inFD  int
	out   io.Writer
	outFD int

	mu        sync.Mutex
	rawState  *term.State
	prev      *tuikit.Frame
	events    chan tuikit.Event
	open      bool
	closed    atomic.Bool
	mouse     bool
	stopWinch chan struct{}

	// evMu serializes the resize watcher's sends with the decoder-exit
	// close of the events channel; evClosed is the under-lock latch.
	evMu     sync.Mutex
	evClosed bool
	// sizeFn is the getSize seam captured at Open: the watcher runs in
	// the background, so it must never read the swappable package var.
	sizeFn func(int) (int, int, error)
}

// New builds a backend over the process TTY (seam-resolved).
func New() *Backend {
	in, inFD := ttyIn()
	out, outFD := ttyOut()
	return &Backend{in: in, inFD: inFD, out: out, outFD: outFD}
}

// streamFD returns the OS file descriptor behind a caller-supplied
// stream, or -1 when it is not a real file — Open then rejects it with
// not_a_tty rather than silently driving the process TTY instead.
func streamFD(v any) int {
	if f, ok := v.(*os.File); ok {
		return int(f.Fd())
	}
	return -1
}

// NewFor builds a backend over the caller's OWN streams and their file
// descriptors, not the process TTY — so a vault TUI launched with
// stdin/stdout that differ from os.Stdin/os.Stdout (a pty, a test
// harness, a programmatic embed) drives the right terminal. New() stays
// the process-TTY constructor behind the ttyIn/ttyOut seams.
func NewFor(in io.Reader, out io.Writer) *Backend {
	return &Backend{in: in, inFD: streamFD(in), out: out, outFD: streamFD(out)}
}

// SpecFor is Spec bound to caller-supplied streams — the launch path for
// callers whose terminal is not the process stdin/stdout.
func SpecFor(in io.Reader, out io.Writer) modules.TuiSpec {
	return modules.TuiSpec{Name: "tty", Open: func() (tuikit.Backend, error) { return NewFor(in, out), nil }}
}

const (
	seqAltScreenOn  = "\x1b[?1049h"
	seqAltScreenOff = "\x1b[?1049l"
	seqCursorHide   = "\x1b[?25l"
	seqCursorShow   = "\x1b[?25h"
	seqMouseOn      = "\x1b[?1002h\x1b[?1006h"
	seqMouseOff     = "\x1b[?1006l\x1b[?1002l"
	seqPasteOn      = "\x1b[?2004h"
	seqPasteOff     = "\x1b[?2004l"
	seqClear        = "\x1b[2J"
)

// Open implements tuikit.Backend.
func (b *Backend) Open(opts tuikit.OpenOpts) (tuikit.Info, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.open || b.closed.Load() {
		return tuikit.Info{}, errors.New("terminal backend is not reusable")
	}
	if !isTerminal(b.outFD) {
		return tuikit.Info{}, errors.New("not_a_tty: stdout is not a terminal")
	}
	if !ttyBusy.CompareAndSwap(false, true) {
		return tuikit.Info{}, errors.New("terminal is already owned by another open (close it first)")
	}
	state, err := makeRaw(b.inFD)
	if err != nil {
		ttyBusy.Store(false)
		return tuikit.Info{}, fmt.Errorf("raw mode: %w", err)
	}
	b.rawState = state
	cols, rows, err := getSize(b.outFD)
	if err != nil {
		_ = restore(b.inFD, b.rawState)
		ttyBusy.Store(false)
		return tuikit.Info{}, fmt.Errorf("terminal size: %w", err)
	}
	var setup strings.Builder
	setup.WriteString(seqAltScreenOn + seqCursorHide + seqPasteOn + seqClear)
	if opts.Mouse {
		b.mouse = true
		setup.WriteString(seqMouseOn)
	}
	if opts.Title != "" {
		setup.WriteString("\x1b]0;" + opts.Title + "\x07")
	}
	if _, err := io.WriteString(b.out, setup.String()); err != nil {
		_ = restore(b.inFD, b.rawState)
		ttyBusy.Store(false)
		return tuikit.Info{}, err
	}
	b.events = make(chan tuikit.Event, 64)
	b.open = true
	b.stopWinch = make(chan struct{})
	go func() {
		decodeInput(bufio.NewReader(b.in), b.events, &b.closed)
		b.evMu.Lock()
		b.evClosed = true
		close(b.events)
		b.evMu.Unlock()
	}()
	b.sizeFn = getSize
	if winch := resizeSignal(b.stopWinch); winch != nil {
		go b.watchResize(winch)
	}
	return tuikit.Info{Cols: cols, Rows: rows}, nil
}

// watchResize turns size-change ticks into resize events so real-TTY
// apps hear window resizes exactly like virtual/remote ones do. Sends
// go through the event latch: once the decoder has exited and closed
// the channel, a late tick is dropped instead of panicking.
func (b *Backend) watchResize(winch <-chan struct{}) {
	for range winch {
		if b.closed.Load() {
			return
		}
		cols, rows, err := b.sizeFn(b.outFD)
		if err != nil {
			continue
		}
		b.evMu.Lock()
		if !b.evClosed {
			select {
			case b.events <- tuikit.Event{Tag: "resize", Cols: cols, Rows: rows}:
			default:
			}
		}
		b.evMu.Unlock()
	}
}

// Close implements tuikit.Backend: restore everything; idempotent. The
// decoder goroutine ends on its next read (EOF, one more byte, or the
// interruptRead deadline poke); its wrapper then closes the events
// channel under the event latch, so neither a blocked reader nor a
// concurrent resize tick ever races the close.
func (b *Backend) Close() error {
	if b.closed.Swap(true) {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.open {
		return nil
	}
	var teardown strings.Builder
	if b.mouse {
		teardown.WriteString(seqMouseOff)
	}
	teardown.WriteString(seqPasteOff + seqCursorShow + seqAltScreenOff)
	_, wErr := io.WriteString(b.out, teardown.String())
	rErr := restore(b.inFD, b.rawState)
	b.open = false
	close(b.stopWinch)
	interruptRead(b.in)
	ttyBusy.Store(false)
	if wErr != nil {
		return wErr
	}
	return rErr
}

// Size implements tuikit.Backend.
func (b *Backend) Size() (int, int, error) {
	return getSize(b.outFD)
}

// Events implements tuikit.Backend.
func (b *Backend) Events() <-chan tuikit.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events
}

// Present implements tuikit.Backend: damage spans → cursor moves + SGR
// runs. tuikit owns the diff and the SGR tables.
func (b *Backend) Present(f *tuikit.Frame) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	spans := tuikit.DiffFrames(b.prev, f)
	if len(spans) == 0 {
		return nil
	}
	var out strings.Builder
	for _, span := range spans {
		fmt.Fprintf(&out, "\x1b[%d;%dH", span.Y+1, span.X+1)
		current := ""
		for _, cell := range span.Cells {
			if cell.Width < 0 {
				continue // continuation of the wide rune already written
			}
			sgr := tuikit.SGR(cell.Style, tuikit.ProfileTrueColor)
			if sgr != current {
				out.WriteString(tuikit.SGRReset + sgr)
				current = sgr
			}
			if cell.Content == "" {
				out.WriteString(" ")
			} else {
				// cell content is PRINTABLE text only: C0/DEL controls
				// smuggled through widget data must not reach the TTY
				// (cursor moves, OSCs) and desynchronize the diff
				out.WriteString(sanitizeCell(cell.Content))
			}
		}
		out.WriteString(tuikit.SGRReset)
	}
	if _, err := io.WriteString(b.out, out.String()); err != nil {
		return err
	}
	b.prev = f.Clone()
	return nil
}

// sanitizeCell replaces C0 control runes and DEL with spaces; styling
// stays on the Style/SGR path, never inside cell content.
func sanitizeCell(s string) string {
	clean := true
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// SetCursor implements tuikit.Backend.
func (b *Backend) SetCursor(x, y int, visible bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !visible {
		_, err := io.WriteString(b.out, seqCursorHide)
		return err
	}
	_, err := fmt.Fprintf(b.out, "\x1b[%d;%dH%s", y+1, x+1, seqCursorShow)
	return err
}

// SetTitle implements tuikit.Backend.
func (b *Backend) SetTitle(title string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := io.WriteString(b.out, "\x1b]0;"+title+"\x07")
	return err
}

// Bell implements tuikit.Backend.
func (b *Backend) Bell() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := io.WriteString(b.out, "\x07")
	return err
}
