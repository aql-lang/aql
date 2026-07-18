package capabilities

import (
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// WatchEvent is one filesystem change notification. Op is a small closed
// vocabulary shared by every backend (the fsnotify op names, lowered):
//
//	create  a path came into existence (write of a new file, mkdir,
//	        symlink/hard-link creation, rename destination)
//	write   a file's content changed (rewrite, truncate)
//	remove  a path was deleted
//	rename  a path was moved away (its old name)
//	chmod   metadata changed (chmod, chtimes)
type WatchEvent struct {
	Op   string
	Path string
}

// watchBufferSize is each subscriber channel's buffer. A slow consumer
// drops events past this depth (never blocks the mutator) — the same
// overflow posture fsnotify takes with the kernel queue.
const watchBufferSize = 64

// newFsnotifyWatcher seams the watcher constructor: it fails only on
// fd / inotify-instance exhaustion, which a test cannot force directly
// (design/TEST-SEAMS.10.md).
var newFsnotifyWatcher = fsnotify.NewWatcher

// --- OSFileOps ---------------------------------------------------------

// Watch subscribes to change events for path — a file, or a directory's
// direct children (non-recursive, inotify semantics) — via fsnotify.
// The returned stop function releases the watch and closes the stream.
// Watching an absent path errors.
func (o *OSFileOps) Watch(path string) (<-chan WatchEvent, func() error, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, nil, err
	}
	w, err := newFsnotifyWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := w.Add(resolved); err != nil {
		_ = w.Close()
		return nil, nil, err
	}
	out := make(chan WatchEvent, watchBufferSize)
	// Watch errors (overflow, races) are not events — drain them so the
	// stream stays open; fsnotify closes both channels on Close.
	go drainWatchErrors(w.Errors)
	go pumpFsnotify(w.Events, out)
	return out, w.Close, nil
}

// drainWatchErrors consumes a watcher's error stream until it closes.
// Errors (kernel queue overflow, races) are advisory — the event stream
// stays open — so they are observed and dropped.
func drainWatchErrors(errs <-chan error) {
	for range errs {
		continue // advisory only; the event stream is unaffected
	}
}

// pumpFsnotify adapts fsnotify's event stream onto a WatchEvent channel,
// closing out when the source closes. Extracted so the adaptation —
// op-name lowering, empty-mask skip, drop-on-full — is deterministic to
// test with a synthetic source.
func pumpFsnotify(events <-chan fsnotify.Event, out chan<- WatchEvent) {
	defer close(out)
	for ev := range events {
		op := fsnotifyOpName(ev.Op)
		if op == "" {
			continue
		}
		select {
		case out <- WatchEvent{Op: op, Path: ev.Name}:
		default: // drop past the buffer, never block
		}
	}
}

// fsnotifyOpName lowers an fsnotify op bitmask to the shared vocabulary,
// picking the most specific set bit ("" for an empty mask).
func fsnotifyOpName(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Remove):
		return "remove"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Chmod):
		return "chmod"
	}
	return ""
}

// --- MemFileOps --------------------------------------------------------

// memWatcher is one live mem subscription: a root (file path, or a
// directory whose direct children are observed) and its event channel.
type memWatcher struct {
	root string
	ch   chan WatchEvent
}

// Watch subscribes to change events on the in-memory filesystem, with
// the same shape as the OS side: non-recursive, absent paths error, and
// slow consumers drop events past the buffer.
func (m *MemFileOps) Watch(path string) (<-chan WatchEvent, func() error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := m.infoFor(final); !ok {
		return nil, nil, &os.PathError{Op: "watch", Path: path, Err: os.ErrNotExist}
	}
	w := &memWatcher{root: final, ch: make(chan WatchEvent, watchBufferSize)}
	m.watchers = append(m.watchers, w)
	stop := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, cur := range m.watchers {
			if cur == w {
				m.watchers = append(m.watchers[:i], m.watchers[i+1:]...)
				close(w.ch)
				return nil
			}
		}
		return nil // already stopped — idempotent, like fsnotify Close
	}
	return w.ch, stop, nil
}

// emit fans a change notification out to the subscriptions covering
// resolved: an exact match (file watch) or its parent directory (dir
// watch, non-recursive — inotify semantics).
func (m *MemFileOps) emit(op, resolved string) {
	for _, w := range m.watchers {
		if resolved == w.root || filepath.Dir(resolved) == w.root {
			select {
			case w.ch <- WatchEvent{Op: op, Path: resolved}:
			default: // drop past the buffer, never block the mutator
			}
		}
	}
}

// --- OverlayFileOps ----------------------------------------------------

// Watch subscribes at the UNION level: the upper layer is always watched
// (materialising the path there first when it only exists below — a
// semantically invisible copy-up), and the lower layer is additionally
// watched when it can be (so external changes to real fixtures surface).
// Events from both layers merge into one stream.
func (o *OverlayFileOps) Watch(path string) (<-chan WatchEvent, func() error, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, nil, err
	}
	fi, err := o.Stat(path, true)
	if err != nil {
		return nil, nil, &os.PathError{Op: "watch", Path: path, Err: os.ErrNotExist}
	}
	// Materialise into the upper so mutations (which always land there)
	// are observed.
	if !o.inUpper(path) {
		if fi.IsDir {
			if err := o.Upper.MkdirAll(path, fi.Mode.Perm()); err != nil {
				return nil, nil, err
			}
		} else if err := o.copyUp(path); err != nil {
			return nil, nil, err
		}
	}
	upCh, upStop, err := o.Upper.Watch(path)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan WatchEvent, watchBufferSize)
	upDone := make(chan struct{}, 1)
	go forwardWatchEvents(upCh, out, upDone)
	stops := []func() error{upStop}
	lowCh, lowStop, lowErr := o.Lower.Watch(path)
	lowDone := make(chan struct{}, 1)
	if lowErr == nil {
		stops = append(stops, lowStop)
		go forwardWatchEvents(lowCh, out, lowDone)
	} else {
		lowDone <- struct{}{} // no lower stream to drain
	}
	go func() {
		<-upDone
		<-lowDone
		close(out)
	}()
	stop := func() error {
		var first error
		for _, s := range stops {
			if err := s(); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
	return out, stop, nil
}

// forwardWatchEvents copies one layer's stream onto the merged union
// stream (drop-on-full), signalling done when the layer closes.
// Extracted for deterministic testing of the drop arm.
func forwardWatchEvents(ch <-chan WatchEvent, out chan<- WatchEvent, done chan<- struct{}) {
	for ev := range ch {
		select {
		case out <- ev:
		default:
		}
	}
	done <- struct{}{}
}
