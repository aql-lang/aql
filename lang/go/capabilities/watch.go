package capabilities

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// WatchEvent is one filesystem change notification. Op is a small closed
// vocabulary shared by every backend (the fsnotify op names, lowered, plus
// the synthetic overflow marker):
//
//	create   a path came into existence (write of a new file, mkdir,
//	         symlink/hard-link creation, rename destination)
//	write    a file's content changed (rewrite, truncate)
//	remove   a path was deleted
//	rename   a path was moved away (its old name)
//	chmod    metadata changed (chmod, chtimes)
//	overflow the buffer filled and events were dropped; Path is the watch
//	         root. Coalesced (one marker per burst) and never glob-filtered,
//	         so a slow consumer always learns it missed something.
type WatchEvent struct {
	Op   string
	Path string
}

// WatchOpts tunes a subscription. Recursive extends a directory watch to the
// whole subtree (the OS side auto-adds newly created subdirectories; the mem
// side matches by path prefix). Glob {match} filtering is a word-layer
// concern — the FileOps surface never sees patterns.
type WatchOpts struct {
	Recursive bool
}

// watchOpOverflow is the op name of the coalesced buffer-overflow marker.
const watchOpOverflow = "overflow"

// watchBufferSize is each subscriber channel's buffer. A slow consumer
// drops events past this depth (never blocks the mutator) — the same
// overflow posture fsnotify takes with the kernel queue.
const watchBufferSize = 64

// newFsnotifyWatcher seams the watcher constructor: it fails only on
// fd / inotify-instance exhaustion, which a test cannot force directly
// (design/TEST-SEAMS.10.md).
var newFsnotifyWatcher = fsnotify.NewWatcher

// watchWalkDir and watchStatFn seam the recursive-add filesystem probes so
// the WalkDir failure arm and the auto-add dir check are testable without
// racing a real inotify queue.
var (
	watchWalkDir = filepath.WalkDir
	watchStatFn  = os.Stat
)

// watchAdder is the slice of fsnotify a recursive pump needs: registering a
// newly created subdirectory. *fsnotify.Watcher satisfies it; tests supply a
// recording fake.
type watchAdder interface {
	Add(name string) error
}

// sendWatch delivers ev to out without ever blocking. On a full buffer it
// sets *overflow; while *overflow holds it first tries to flush a single
// coalesced {overflow, root} marker (clearing the flag on success) so a slow
// consumer is told exactly once, in order, that events were dropped.
func sendWatch(out chan<- WatchEvent, ev WatchEvent, root string, overflow *bool) {
	if *overflow {
		select {
		case out <- WatchEvent{Op: watchOpOverflow, Path: root}:
			*overflow = false
		default:
			return // still full: drop ev, stay overflowed
		}
	}
	select {
	case out <- ev:
	default:
		*overflow = true
	}
}

// flushOverflow best-effort delivers a still-pending overflow marker before a
// stream closes (non-blocking — a consumer that has stopped reading simply
// misses it, and the stream is closing anyway).
func flushOverflow(out chan<- WatchEvent, root string, overflow bool) {
	if overflow {
		select {
		case out <- WatchEvent{Op: watchOpOverflow, Path: root}:
		default:
		}
	}
}

// --- OSFileOps ---------------------------------------------------------

// Watch subscribes to change events for path — a file, or a directory's
// direct children (inotify semantics), or the whole subtree when
// opts.Recursive is set — via fsnotify. Recursive watches add every existing
// subdirectory up front and auto-add newly created ones (chokidar-style; the
// window between a subdir's creation and its Add is an unavoidable gap). The
// returned stop function releases the watch and closes the stream. Watching
// an absent path errors.
func (o *OSFileOps) Watch(path string, opts WatchOpts) (<-chan WatchEvent, func() error, error) {
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
	if opts.Recursive {
		walkErr := watchWalkDir(resolved, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return w.Add(p)
			}
			return nil
		})
		if walkErr != nil {
			_ = w.Close()
			return nil, nil, walkErr
		}
	}
	out := make(chan WatchEvent, watchBufferSize)
	// Watch errors (overflow, races) are not events — drain them so the
	// stream stays open; fsnotify closes both channels on Close.
	go drainWatchErrors(w.Errors)
	go pumpFsnotify(w.Events, w, out, resolved, opts.Recursive)
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
// closing out when the source closes. Extracted so the adaptation — op-name
// lowering, empty-mask skip, recursive auto-add, and overflow coalescing —
// is deterministic to test with a synthetic source and a recording adder.
func pumpFsnotify(events <-chan fsnotify.Event, adder watchAdder, out chan<- WatchEvent, root string, recursive bool) {
	defer close(out)
	overflow := false
	for ev := range events {
		op := fsnotifyOpName(ev.Op)
		if op == "" {
			continue
		}
		if recursive && op == "create" {
			// Auto-add a newly created subdirectory so its own children
			// surface too (fsnotify itself is non-recursive).
			if fi, err := watchStatFn(ev.Name); err == nil && fi.IsDir() {
				_ = adder.Add(ev.Name)
			}
		}
		sendWatch(out, WatchEvent{Op: op, Path: ev.Name}, root, &overflow)
	}
	flushOverflow(out, root, overflow)
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

// memWatcher is one live mem subscription: a root (file path, or a directory
// whose children are observed), its event channel, whether the watch is
// recursive, and a sticky overflow flag (guarded by MemFileOps.mu, like the
// watchers slice itself).
type memWatcher struct {
	root      string
	ch        chan WatchEvent
	recursive bool
	overflow  bool
}

// covers reports whether a change at resolved belongs to this subscription:
// the exact target, a direct child (non-recursive dir watch, inotify
// semantics), or any descendant (recursive).
func (w *memWatcher) covers(resolved string) bool {
	if resolved == w.root {
		return true
	}
	if w.recursive {
		return isUnder(resolved, w.root)
	}
	return filepath.Dir(resolved) == w.root
}

// isUnder reports whether path lies within the root subtree. The trailing
// separator guard stops "d" from matching a sibling "dd/x".
func isUnder(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// Watch subscribes to change events on the in-memory filesystem, with the
// same shape as the OS side: absent paths error, slow consumers coalesce
// dropped events into one overflow marker, and {recursive} observes the whole
// subtree instead of only direct children.
func (m *MemFileOps) Watch(path string, opts WatchOpts) (<-chan WatchEvent, func() error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := m.infoFor(final); !ok {
		return nil, nil, &os.PathError{Op: "watch", Path: path, Err: os.ErrNotExist}
	}
	w := &memWatcher{root: final, ch: make(chan WatchEvent, watchBufferSize), recursive: opts.Recursive}
	m.watchers = append(m.watchers, w)
	stop := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, cur := range m.watchers {
			if cur == w {
				m.watchers = append(m.watchers[:i], m.watchers[i+1:]...)
				flushOverflow(w.ch, w.root, w.overflow)
				close(w.ch)
				return nil
			}
		}
		return nil // already stopped — idempotent, like fsnotify Close
	}
	return w.ch, stop, nil
}

// emit fans a change notification out to the subscriptions covering resolved,
// coalescing drops on a full subscriber into one overflow marker. Called
// under m.mu (every mutator holds it), so each watcher's overflow flag is
// safe to touch here.
func (m *MemFileOps) emit(op, resolved string) {
	for _, w := range m.watchers {
		if w.covers(resolved) {
			sendWatch(w.ch, WatchEvent{Op: op, Path: resolved}, w.root, &w.overflow)
		}
	}
}

// --- OverlayFileOps ----------------------------------------------------

// Watch subscribes at the UNION level: the upper layer is always watched
// (materialising the path there first when it only exists below — a
// semantically invisible copy-up), and the lower layer is additionally
// watched when it can be (so external changes to real fixtures surface).
// Events from both layers merge into one stream; opts (including {recursive})
// pass to both.
func (o *OverlayFileOps) Watch(path string, opts WatchOpts) (<-chan WatchEvent, func() error, error) {
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
	upCh, upStop, err := o.Upper.Watch(path, opts)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan WatchEvent, watchBufferSize)
	upDone := make(chan struct{}, 1)
	go forwardWatchEvents(upCh, out, path, upDone)
	stops := []func() error{upStop}
	lowCh, lowStop, lowErr := o.Lower.Watch(path, opts)
	lowDone := make(chan struct{}, 1)
	if lowErr == nil {
		stops = append(stops, lowStop)
		go forwardWatchEvents(lowCh, out, path, lowDone)
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

// forwardWatchEvents copies one layer's stream onto the merged union stream,
// coalescing drops into one overflow marker (root = the watched path) and
// signalling done when the layer closes. Extracted for deterministic testing
// of the drop arm.
func forwardWatchEvents(ch <-chan WatchEvent, out chan<- WatchEvent, root string, done chan<- struct{}) {
	overflow := false
	for ev := range ch {
		sendWatch(out, ev, root, &overflow)
	}
	flushOverflow(out, root, overflow)
	done <- struct{}{}
}
