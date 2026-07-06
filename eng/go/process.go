package eng

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// BEAM-style lightweight processes — the concurrency substrate specified
// in design/PROCESSES.0.md. A process is a goroutine running an AQL body
// on its own ForkConcurrent() registry, plus a bounded, mutex-guarded
// mailbox. The kernel owns the runtime (this file); the language layer
// (lang/go/native/native_process.go) owns the words (`spawn`, `self`,
// `send`, `receive`, `register`, `whereis`, `unregister`) and the patrun
// clause dispatch.
//
// The runtime is a POINTER field on Registry (Registry.Procs), created by
// NewRegistry, so ForkConcurrent's shallow copy shares one runtime across
// every fork — the process table and name registry are engine-wide with
// no changes to fork.go (the PROCESSES.0.md §2 enabling fact).

// DefaultMailboxBound is the default mailbox capacity (messages) for a
// spawned process — bounded by design, diverging from BEAM's unbounded
// mailbox (see PROCESSES.0.md §3 "Bounded mailbox").
const DefaultMailboxBound = 1024

// Mailbox overflow policies (SERVICES.0.md §8.1).
const (
	OverflowBlock = "block" // sender blocks until space frees (backpressure)
	OverflowFail  = "fail"  // delivery fails fast (raise `overload`)
	OverflowDrop  = "drop"  // the new message is silently discarded
)

// ErrMailboxOverload is returned by Process.Send under the "fail"
// overflow policy (or a timed-out "block") when the target mailbox is
// full. The language layer surfaces it as the `overload` error code.
var ErrMailboxOverload = fmt.Errorf("mailbox full")

// ErrRuntimeDown is returned by mailbox operations after the runtime has
// been shut down (host exit / cancellation).
var ErrRuntimeDown = fmt.Errorf("process runtime is shut down")

// ProcessRuntime owns the engine-wide process state: the pid table, the
// named-process registry, and the root cancellation scope. One per
// top-level engine; every ForkConcurrent fork shares the same pointer.
type ProcessRuntime struct {
	mu    sync.Mutex
	table map[string]*Process
	names map[string]*Process

	ctx    context.Context
	cancel context.CancelFunc
	down   bool

	// syncOuts caches one SyncWriter per underlying writer so every
	// process sharing a host writer serializes through the SAME wrapper
	// (two independent wrappers around one writer would not exclude
	// each other).
	outMu    sync.Mutex
	syncOuts map[io.Writer]*SyncWriter
}

// NewProcessRuntime builds an empty runtime. Called by NewRegistry; no
// goroutines start until the first spawn.
func NewProcessRuntime() *ProcessRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessRuntime{
		table:    map[string]*Process{},
		names:    map[string]*Process{},
		ctx:      ctx,
		cancel:   cancel,
		syncOuts: map[io.Writer]*SyncWriter{},
	}
}

// SyncOutput returns the runtime's shared SyncWriter for w, creating it
// on first use. If w is already a *SyncWriter it is returned as-is (a
// process spawning a child must not double-wrap).
func (rt *ProcessRuntime) SyncOutput(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	if sw, ok := w.(*SyncWriter); ok {
		return sw
	}
	rt.outMu.Lock()
	defer rt.outMu.Unlock()
	sw, ok := rt.syncOuts[w]
	if !ok {
		sw = NewSyncWriter(w)
		rt.syncOuts[w] = sw
	}
	return sw
}

// Shutdown cancels the runtime: every blocked receive/send wakes with
// ErrRuntimeDown and no new process can be inserted. Idempotent.
func (rt *ProcessRuntime) Shutdown() {
	rt.mu.Lock()
	if rt.down {
		rt.mu.Unlock()
		return
	}
	rt.down = true
	procs := make([]*Process, 0, len(rt.table))
	for _, p := range rt.table {
		procs = append(procs, p)
	}
	rt.mu.Unlock()
	rt.cancel()
	for _, p := range procs {
		p.mu.Lock()
		p.cond.Broadcast()
		p.mu.Unlock()
	}
}

// Down reports whether the runtime has been shut down.
func (rt *ProcessRuntime) Down() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.down
}

// Insert adds p to the pid table. Returns ErrRuntimeDown after Shutdown.
func (rt *ProcessRuntime) Insert(p *Process) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.down {
		return ErrRuntimeDown
	}
	rt.table[p.ID] = p
	return nil
}

// Remove deletes p from the pid table and drops its registered name (if
// any). Called by the process goroutine's exit cleanup.
func (rt *ProcessRuntime) Remove(p *Process) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.table, p.ID)
	if p.name != "" {
		if cur, ok := rt.names[p.name]; ok && cur == p {
			delete(rt.names, p.name)
		}
		p.name = ""
	}
}

// LookupPid returns the live process with the given pid id, if any.
func (rt *ProcessRuntime) LookupPid(id string) (*Process, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	p, ok := rt.table[id]
	return p, ok
}

// RegisterName binds name to p. Re-registering a name rebinds it (the
// previous holder keeps running but loses the name). Returns
// ErrRuntimeDown after Shutdown.
func (rt *ProcessRuntime) RegisterName(name string, p *Process) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.down {
		return ErrRuntimeDown
	}
	if prev, ok := rt.names[name]; ok && prev != p {
		prev.name = ""
	}
	rt.names[name] = p
	p.name = name
	return nil
}

// Whereis resolves a registered name to its process.
func (rt *ProcessRuntime) Whereis(name string) (*Process, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	p, ok := rt.names[name]
	return p, ok
}

// UnregisterName removes a name binding. Missing names are a no-op
// (matching `send` to an unknown name: fire-and-forget semantics).
func (rt *ProcessRuntime) UnregisterName(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if p, ok := rt.names[name]; ok {
		p.name = ""
		delete(rt.names, name)
	}
}

// Process is one lightweight process: an identity, a bounded FIFO
// mailbox, and lifecycle state. The goroutine + forked registry that run
// the body live in the language layer; the kernel only owns the mailbox
// discipline so `send`/`receive` have one implementation.
type Process struct {
	ID string

	rt       *ProcessRuntime
	mu       sync.Mutex
	cond     *sync.Cond
	mailbox  []Value
	bound    int    // capacity; <= 0 means unbounded (explicit opt-in)
	overflow string // OverflowBlock / OverflowFail / OverflowDrop
	closed   bool
	name     string // registered name ("" if none), guarded by rt.mu

	done chan struct{}
}

// NewProcess allocates a process handle (no goroutine). bound <= 0 means
// an explicitly unbounded mailbox; overflow defaults to OverflowBlock.
func NewProcess(rt *ProcessRuntime, bound int, overflow string) *Process {
	if overflow == "" {
		overflow = OverflowBlock
	}
	p := &Process{
		ID:       GenerateID("P_"),
		rt:       rt,
		bound:    bound,
		overflow: overflow,
		done:     make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Runtime returns the owning runtime.
func (p *Process) Runtime() *ProcessRuntime { return p.rt }

// Name returns the process's registered name ("" if none).
func (p *Process) Name() string {
	p.rt.mu.Lock()
	defer p.rt.mu.Unlock()
	return p.name
}

// Done returns a channel closed when the process exits.
func (p *Process) Done() <-chan struct{} { return p.done }

// Close marks the process exited: the mailbox stops accepting sends,
// blocked senders wake, and Done() closes. Idempotent.
func (p *Process) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
	close(p.done)
}

// Closed reports whether the process has exited.
func (p *Process) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Send enqueues msg per the mailbox's overflow policy. Sends to a closed
// process are silently dropped (BEAM semantics: fire-and-forget).
// Returns ErrMailboxOverload under "fail" (or when a bounded "block"
// cannot make progress after runtime shutdown → ErrRuntimeDown).
func (p *Process) Send(msg Value) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.closed {
			return nil // dropped: dead target
		}
		if p.bound <= 0 || len(p.mailbox) < p.bound {
			p.mailbox = append(p.mailbox, msg)
			p.cond.Broadcast()
			return nil
		}
		switch p.overflow {
		case OverflowFail:
			return ErrMailboxOverload
		case OverflowDrop:
			return nil
		default: // OverflowBlock — wait for space
			if p.rt.Down() {
				return ErrRuntimeDown
			}
			p.waitWithWake()
		}
	}
}

// PopFront blocks until a message is available and returns the FRONT
// (oldest) message — the consume-front discipline of PROCESSES.0.md §3.
// With hasDeadline, it gives up after timeout and returns ok=false (the
// `after` clause path); timeout 0 polls once. Returns ErrRuntimeDown if
// the runtime shuts down while waiting.
func (p *Process) PopFront(timeout time.Duration, hasDeadline bool) (Value, bool, error) {
	var deadline time.Time
	if hasDeadline {
		deadline = time.Now().Add(timeout)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if len(p.mailbox) > 0 {
			msg := p.mailbox[0]
			p.mailbox = p.mailbox[1:]
			return msg, true, nil
		}
		if p.rt.Down() {
			return Value{}, false, ErrRuntimeDown
		}
		if hasDeadline {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return Value{}, false, nil
			}
			timer := time.AfterFunc(remaining, func() {
				p.mu.Lock()
				p.cond.Broadcast()
				p.mu.Unlock()
			})
			p.cond.Wait()
			timer.Stop()
		} else {
			p.waitWithWake()
		}
	}
}

// MailboxDepth reports the current queue length (observability).
func (p *Process) MailboxDepth() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.mailbox)
}

// waitWithWake waits on the condition with a periodic wake so a runtime
// shutdown (which may race the wait) is always observed. The 100ms poll
// only runs while blocked; the common signalled path is immediate.
func (p *Process) waitWithWake() {
	timer := time.AfterFunc(100*time.Millisecond, func() {
		p.mu.Lock()
		p.cond.Broadcast()
		p.mu.Unlock()
	})
	p.cond.Wait()
	timer.Stop()
}
