package tuikit

import (
	"errors"
	"sync"
)

// virtualEventBuffer is how many injected events a VirtualBackend can
// hold before Inject starts dropping — far above any scripted test.
const virtualEventBuffer = 1024

// defaultMaxFrames bounds the recorded frame history (TUI.0.md §4.4):
// every Present is recorded so tests can golden-match MID-RUN screens
// (after quit the real terminal is restored, so the final grid is not
// where the interesting frames are — probe finding F2).
const defaultMaxFrames = 256

// VirtualBackend is the in-memory Backend used by tests and any host
// without a real terminal: a fixed-size grid, a scripted input queue
// (Inject), and a bounded history of every presented frame
// (FrameCount/ScreenAt). The exported *Err fields are test knobs that
// force the corresponding call to fail, so error arms are reachable on
// a terminal-less CI.
type VirtualBackend struct {
	OpenErr    error
	CloseErr   error
	SizeErr    error
	PresentErr error
	CursorErr  error
	TitleErr   error
	BellErr    error

	mu       sync.Mutex
	cols     int
	rows     int
	open     bool
	closed   bool
	events   chan Event
	grid     *Frame
	frames   []*Frame
	maxKeep  int
	title    string
	bells    int
	presents int
	cursorX  int
	cursorY  int
	cursorOn bool
}

// NewVirtualBackend returns a Cols×Rows virtual terminal. Events may be
// injected before Open — they are delivered once something reads the
// event channel.
func NewVirtualBackend(cols, rows int) *VirtualBackend {
	return &VirtualBackend{
		cols:    cols,
		rows:    rows,
		events:  make(chan Event, virtualEventBuffer),
		grid:    NewFrame(cols, rows),
		maxKeep: defaultMaxFrames,
	}
}

// Inject queues one input event, as if the user had typed it. After
// Close (or when the buffer is full) the event is silently dropped —
// a scripted test that overruns either bound is asserting nothing.
func (vb *VirtualBackend) Inject(ev Event) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.closed {
		return
	}
	select {
	case vb.events <- ev:
	default:
	}
}

// Open implements Backend. A second Open before Close errors — the
// module's single-owner rule assumes a backend surface is not shared.
func (vb *VirtualBackend) Open(opts OpenOpts) (Info, error) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.OpenErr != nil {
		return Info{}, vb.OpenErr
	}
	if vb.closed {
		return Info{}, errors.New("virtual backend is closed")
	}
	if vb.open {
		return Info{}, errors.New("virtual backend is already open")
	}
	vb.open = true
	if opts.Title != "" {
		vb.title = opts.Title
	}
	return Info{Cols: vb.cols, Rows: vb.rows}, nil
}

// Close implements Backend: idempotent, and closes the event channel so
// blocked readers observe end-of-input.
func (vb *VirtualBackend) Close() error {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.closed {
		return nil
	}
	if vb.CloseErr != nil {
		return vb.CloseErr
	}
	vb.closed = true
	vb.open = false
	close(vb.events)
	return nil
}

// Size implements Backend.
func (vb *VirtualBackend) Size() (int, int, error) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.SizeErr != nil {
		return 0, 0, vb.SizeErr
	}
	return vb.cols, vb.rows, nil
}

// Events implements Backend.
func (vb *VirtualBackend) Events() <-chan Event {
	return vb.events
}

// Present implements Backend: the frame becomes the current grid and is
// recorded (as a clone) into the bounded history.
func (vb *VirtualBackend) Present(f *Frame) error {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.PresentErr != nil {
		return vb.PresentErr
	}
	c := f.Clone()
	vb.grid = c
	vb.presents++
	if len(vb.frames) == vb.maxKeep {
		vb.frames = vb.frames[1:]
	}
	vb.frames = append(vb.frames, c)
	return nil
}

// SetCursor implements Backend, recording the placement for assertions.
func (vb *VirtualBackend) SetCursor(x, y int, visible bool) error {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.CursorErr != nil {
		return vb.CursorErr
	}
	vb.cursorX, vb.cursorY, vb.cursorOn = x, y, visible
	return nil
}

// Cursor reports the last SetCursor placement.
func (vb *VirtualBackend) Cursor() (x, y int, visible bool) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.cursorX, vb.cursorY, vb.cursorOn
}

// SetTitle implements Backend.
func (vb *VirtualBackend) SetTitle(title string) error {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.TitleErr != nil {
		return vb.TitleErr
	}
	vb.title = title
	return nil
}

// Bell implements Backend.
func (vb *VirtualBackend) Bell() error {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if vb.BellErr != nil {
		return vb.BellErr
	}
	vb.bells++
	return nil
}

// Screen renders the CURRENT grid, one string per row. For mid-run
// frames use FrameCount/ScreenAt over the recorded history.
func (vb *VirtualBackend) Screen() []string {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return renderRows(vb.grid)
}

// CellAt reads a cell of the current grid, reporting false out of bounds.
func (vb *VirtualBackend) CellAt(x, y int) (Cell, bool) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.grid.At(x, y)
}

// FrameCount reports how many presented frames the history holds.
func (vb *VirtualBackend) FrameCount() int {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return len(vb.frames)
}

// ScreenAt renders recorded frame i (0-based, oldest first), one string
// per row; nil when i is out of range.
func (vb *VirtualBackend) ScreenAt(i int) []string {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	if i < 0 || i >= len(vb.frames) {
		return nil
	}
	return renderRows(vb.frames[i])
}

// Title reports the last SetTitle value; Bells counts Bell calls;
// Presents counts Present calls — assertion getters for tests.
func (vb *VirtualBackend) Title() string {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.title
}

func (vb *VirtualBackend) Bells() int {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.bells
}

func (vb *VirtualBackend) Presents() int {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.presents
}
