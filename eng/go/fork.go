package eng

import (
	"io"
	"sync"
)

// Concurrency model for forked registries.
//
// The interpreter's Registry is single-threaded shared state: the step
// loop constantly pushes and pops the def table, the context stack, and
// the args stack. Running two engines that share one Registry on
// separate goroutines is therefore a data race (concurrent map / slice
// mutation = undefined behaviour in Go).
//
// The concurrent words (`await`, `timeout`, `interval`) need real
// parallelism, so each concurrent execution gets its OWN registry via
// ForkConcurrent: an isolated copy that shares the read-mostly
// infrastructure (the builtin lattice, ideals, capabilities, modules,
// native registrations, parser, host hooks) but gives the goroutine its
// own value-binding, context, args, and flow-control scopes. Concurrent
// branches then never touch each other's mutable stacks.
//
// Caller contract: ForkConcurrent reads the parent's def table, context
// stack, and dynamic type table to seed the clone, so it must be called
// while the parent is NOT concurrently mutating those — i.e. on the
// goroutine that owns the parent registry. `await` forks up-front on the
// dispatching goroutine before spawning branches; the timer words fork
// at schedule time (also on the owning goroutine), not when the timer
// fires. Output/ErrOutput remain shared, so callers running several
// forks at once should wrap the writer in a SyncWriter first.

// ForkConcurrent returns an isolated registry safe to run on its own
// goroutine concurrently with the parent and with sibling forks. It
// shares the parent's read-only infrastructure but isolates every
// mutable execution scope. The fork observes the parent's bindings and
// context as of the fork call; its later writes stay private.
func (r *Registry) ForkConcurrent() *Registry {
	fork := *r // shallow copy inherits the shared read-only pointers
	fork.Defs = r.Defs.Clone()
	fork.Types = r.Types.CloneDynamic()
	fork.Contexts = NewContextStack()
	// A private child layer over the inherited context chain: reads walk
	// down into the (parked) parent's stores, writes land in the child.
	fork.Contexts.Push(r.Contexts.Top())
	fork.Args = NewArgsStack()
	fork.FnBaselines = nil
	fork.FlowCtrl = FlowNone
	fork.pendingGen = nil
	fork.macroCache = nil
	fork.errs = nil
	fork.SDKCache = make(map[string]any)
	fork.vmRunning = 0 // the fork starts idle, independent of the parent's run
	return &fork
}

// SyncWriter serializes concurrent Write calls so that several forked
// registries sharing one underlying writer (e.g. os.Stdout, or a test
// buffer) do not race or interleave a single write. Wrap the parent's
// Output once and assign the wrapper to each fork before spawning.
type SyncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewSyncWriter wraps w so concurrent writes are mutually excluded.
func NewSyncWriter(w io.Writer) *SyncWriter {
	return &SyncWriter{w: w}
}

// Write satisfies io.Writer under a mutex.
func (s *SyncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
