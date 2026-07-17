// Package tuikit is the terminal-backend seam for the aql:tui module —
// the executable slice of design/TUI.0.md §4.2. It is a LEAF package:
// pure types and pure helpers, zero terminal dependencies, so lang/go
// stays terminal-free (the layering rule) while both the module words
// (lang/go/modules) and host backends (cmd/go, tests) import it.
//
// A Backend owns one terminal-like surface. The real TTY implementation
// lives with the host (cmd/go/internal/termback, Phase C); tests use
// VirtualBackend (virtual.go); the wasm playground registers nothing
// until an xterm.js backend exists.
package tuikit

// OpenOpts carries the options Tui.open / Tui.run accept (TUI.0.md §7.1).
type OpenOpts struct {
	// Mouse opts into mouse reporting (default off so native text
	// selection keeps working — TUI.0.md §2.4).
	Mouse bool
	// Title, when non-empty, is set as the window title at open.
	Title string
}

// Info reports the opened surface's dimensions.
type Info struct {
	Cols, Rows int
}

// Event is one decoded input event. Tag selects which of the remaining
// fields are meaningful; the value shapes delivered to AQL programs are
// fixed by TUI.0.md §2.4 (this struct is their Go carrier).
type Event struct {
	// Tag is "key", "resize", "mouse", "paste" or "focus".
	Tag string
	// Key ("enter", "esc", "up", "a", …), Char (the printable rune or
	// "") and Mods (subset of "ctrl"/"alt"/"shift") apply to Tag "key".
	Key  string
	Char string
	Mods []string
	// Cols/Rows apply to Tag "resize".
	Cols, Rows int
	// Kind ("press", "release", "move", "wheel-up", "wheel-down") and
	// X/Y apply to Tag "mouse".
	Kind string
	X, Y int
	// Text applies to Tag "paste".
	Text string
	// Gained applies to Tag "focus".
	Gained bool
}

// Backend is the host-injected terminal seam (TUI.0.md §4.2). Open puts
// the surface into raw mode + alt-screen; Close restores everything and
// is idempotent; Present receives the FULL desired frame and owns any
// diffing against what is on screen; Events delivers decoded input and
// is closed by Close.
type Backend interface {
	Open(opts OpenOpts) (Info, error)
	Close() error
	Size() (cols, rows int, err error)
	Events() <-chan Event
	Present(f *Frame) error
	SetTitle(title string) error
	Bell() error
}
