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
)

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

	mu       sync.Mutex
	rawState *term.State
	prev     *tuikit.Frame
	events   chan tuikit.Event
	open     bool
	closed   atomic.Bool
	mouse    bool
}

// New builds a backend over the process TTY (seam-resolved).
func New() *Backend {
	in, inFD := ttyIn()
	out, outFD := ttyOut()
	return &Backend{in: in, inFD: inFD, out: out, outFD: outFD}
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
	state, err := makeRaw(b.inFD)
	if err != nil {
		return tuikit.Info{}, fmt.Errorf("raw mode: %w", err)
	}
	b.rawState = state
	cols, rows, err := getSize(b.outFD)
	if err != nil {
		_ = restore(b.inFD, b.rawState)
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
		return tuikit.Info{}, err
	}
	b.events = make(chan tuikit.Event, 64)
	b.open = true
	go decodeInput(bufio.NewReader(b.in), b.events, &b.closed)
	return tuikit.Info{Cols: cols, Rows: rows}, nil
}

// Close implements tuikit.Backend: restore everything; idempotent. The
// decoder goroutine ends on its next read (EOF or one more byte); the
// events channel is closed by the decoder's defer, not here, so a
// blocked reader never races a close.
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
				out.WriteString(cell.Content)
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
