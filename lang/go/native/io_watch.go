package native

import (
	"fmt"
	"sync"

	"github.com/aql-lang/aql/eng/go"
)

// io_watch.go — the aql:io filesystem watcher: `IO.watch path [body]`
// subscribes to change events on a Pathon (a file, or a directory's
// direct children — non-recursive, inotify semantics) and runs the body
// once per event, PUSH style, exactly like aql:time-util's `interval`
// runs its callback per tick. The body executes on a concurrent fork of
// the registry with the EVENT RECORD on the stack:
//
//	{op: create/q|write/q|remove/q|rename/q|chmod/q, path: <Pathon>}
//
// `IO.unwatch w` stops the watcher and closes the stream. The backend
// is whatever FileOps is effective at watch time: fsnotify on the real
// filesystem, the in-memory event source under __sys.fs.mem, and the
// union of both layers under __sys.fs.overlay — so watch behaviour is
// part of the same real/mem parity surface as every other io word.

// WatcherInfo is the payload behind a Watcher handle (carried via
// ExtensionPayload — a module-scoped resource, like Timeout/Interval
// but without a global FixedID).
type WatcherInfo struct {
	ID   string
	Path string
	stop func() error
	once sync.Once
	// done closes when the callback pump exits (the event stream
	// closed) — tests use it to join deterministically.
	done chan struct{}
}

// Stop releases the underlying watch exactly once (idempotent, like
// fsnotify's Close) and reports the release error, if any.
func (wi *WatcherInfo) Stop() error {
	var err error
	wi.once.Do(func() { err = wi.stop() })
	return err
}

// Done exposes the pump-exit join point for tests.
func (wi *WatcherInfo) Done() <-chan struct{} { return wi.done }

// watcherFormatBehavior renders a Watcher as "Watcher(id,path)".
type watcherFormatBehavior struct{}

func (watcherFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (watcherFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (watcherFormatBehavior) Format(v Value) string {
	if wi, ok := asWatcherInfo(v); ok {
		return fmt.Sprintf("Watcher(%s,%s)", wi.ID, wi.Path)
	}
	return "Watcher(nil)"
}

// MintWatcherType mints the module-scoped Watcher resource type —
// per-import, like StreamKind/FileType, never a global builtin.
func MintWatcherType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("Watcher", eng.TIdeal, watcherFormatBehavior{})
}

// NewWatcherType mints a standalone Watcher type (into its own dynamic
// type table) for test helpers that register the io words under bare
// names without a host registry to mint into — the NewStreamKind /
// NewFileType twin. Production code mints per import via MintWatcherType.
func NewWatcherType() *Type {
	return eng.NewDynamicTypeTable().MintTypeWithBehavior("Watcher", eng.TIdeal, watcherFormatBehavior{})
}

// asWatcherInfo unwraps a Watcher handle's payload.
func asWatcherInfo(v Value) (*WatcherInfo, bool) {
	ext, ok := v.Data.(ExtensionPayload)
	if !ok {
		return nil, false
	}
	wi, ok := ext.Body.(*WatcherInfo)
	return wi, ok
}

// watchEventValue builds the AQL event record for one WatchEvent.
func watchEventValue(op, path string) Value {
	om := NewOrderedMap()
	om.Set("op", NewAtom(op))
	om.Set("path", NewPathonFromString(path))
	return NewMap(om)
}

// doWatchWord implements IO.watch: subscribe via the effective FileOps,
// then pump events into per-event body runs on a concurrent fork. The
// body list is captured unevaluated (NoEvalArgs); each run gets the
// event record pushed beneath it, so `[.path print]`-style bodies read
// the event off the stack.
func doWatchWord(args []Value, r *Registry, watcherType *Type) ([]Value, error) {
	path := extractPath(args[0])
	body := args[1]
	if !IsConcrete(body) {
		return nil, r.AqlError("watch_error", "watch: the callback body must be a concrete list", "watch")
	}
	bodyList, _ := AsList(body)
	tokens := make([]Value, bodyList.Len())
	copy(tokens, bodyList.Slice())

	ch, stop, err := EffectiveFileOps(r).Watch(path)
	if err != nil {
		return nil, r.AqlError("watch_error", fmt.Sprintf("watch: %v", err), "watch")
	}
	info := &WatcherInfo{ID: GenerateID("W_"), Path: path, stop: stop, done: make(chan struct{})}
	// Fork now, on the subscribing goroutine, so every event runs the
	// body on an isolated registry and never races the main interpreter
	// — the same pattern interval's ticker uses.
	fork := r.ForkConcurrent()
	go func() {
		defer close(info.done)
		for ev := range ch {
			input := make([]Value, 0, len(tokens)+1)
			input = append(input, watchEventValue(ev.Op, ev.Path))
			input = append(input, tokens...)
			sub := New(fork)
			// Execute and discard results — watch callbacks run for
			// side effects, like timer callbacks.
			_, _ = sub.Run(input)
		}
	}()
	return []Value{eng.NewValueRaw(watcherType, ExtensionPayload{Body: info})}, nil
}

// doUnwatchWord implements IO.unwatch: stop the watcher (idempotent).
func doUnwatchWord(args []Value, r *Registry) ([]Value, error) {
	wi, ok := asWatcherInfo(args[0])
	if !ok {
		return nil, r.AqlError("unwatch_error", fmt.Sprintf("unwatch: not a Watcher handle (got %s)", args[0].Parent), "unwatch")
	}
	if err := wi.Stop(); err != nil {
		return nil, r.AqlError("unwatch_error", fmt.Sprintf("unwatch: %v", err), "unwatch")
	}
	return nil, nil
}
