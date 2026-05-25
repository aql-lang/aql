// Package service defines the lifecycle contract for long-running
// AQL services (repl, registry, lsp) and the State enum they report.
//
// The split between Command (one-shot CLI verb) and Service (named,
// supervisable long-runner) was introduced to support `aql serve
// <svc> [flags] + <svc> [flags] ...`, which composes multiple
// services into one process under a single supervisor.
//
// Implementations must:
//
//   - Return promptly from Start when ctx is canceled.
//   - Make Stop idempotent.
//   - Keep Status accurate from any goroutine.
//
// Pause/Resume are optional via the Pausable interface; services that
// cannot meaningfully pause (e.g. LSP, whose protocol assumes a live
// client) simply do not implement it.
package service

import "context"

// State is the lifecycle state of a Service, reported by Status().
type State int

const (
	// StateStopped is the initial state and the state after a clean
	// Stop. Start() may be called from this state.
	StateStopped State = iota
	// StateStarting is set transiently between Start being called
	// and the service being ready to accept work.
	StateStarting
	// StateRunning is the normal operating state.
	StateRunning
	// StatePaused indicates the service is alive but rejecting or
	// queueing work. Only services implementing Pausable enter this
	// state.
	StatePaused
	// StateStopping is set transiently between Stop being called
	// and Start returning.
	StateStopping
)

// String returns the lowercase state name (matches the strings
// returned by the ctl status endpoint).
func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateStopping:
		return "stopping"
	}
	return "unknown"
}

// Service is the contract every long-running AQL service implements.
// Start blocks until the service exits cleanly or ctx is canceled.
// Stop requests a graceful shutdown and returns when shutdown is
// complete (or stopCtx is canceled, whichever first).
type Service interface {
	// Name returns the short identifier used on the CLI and in ctl
	// commands ("repl", "registry", "lsp").
	Name() string
	// Start runs the service until ctx is canceled or the service
	// exits on its own. Returns nil on a clean exit, an error on
	// failure. A canceled context counts as a clean exit (returns nil
	// or context.Canceled — callers should treat both as success).
	Start(ctx context.Context) error
	// Stop requests a graceful shutdown. Safe to call repeatedly.
	// Returns when shutdown is complete or stopCtx is canceled.
	Stop(stopCtx context.Context) error
	// Status returns the current lifecycle state.
	Status() State
}

// Pausable is implemented by services that can suspend and resume
// processing without losing state. Pause should leave the service
// alive (sockets bound, buffers retained) but quiesce work; Resume
// returns it to StateRunning.
type Pausable interface {
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
}

// StdioUser is implemented by services that bind to process stdio
// for I/O (currently repl always, lsp when no TCP port is given).
// The supervisor uses this to reject combinations where multiple
// stdio services would compete for stdin/stdout.
type StdioUser interface {
	UsesStdio() bool
}

// WithMetadata is implemented by services that have observable
// runtime metadata (listen address, registry directory, etc.). The
// api service surfaces these key/value pairs in service responses.
// Implementations must return a stable, JSON-friendly map; values
// that change at runtime (state) belong elsewhere.
type WithMetadata interface {
	Metadata() map[string]string
}

// Inspector is the read/control surface the supervisor exposes to
// services that need to introspect or act on their siblings (api,
// tui). Implementations are safe for concurrent use.
type Inspector interface {
	// Services returns all services in supervisor-registration order.
	Services() []Service
	// ByName looks up a service by its Name(); ok is false if absent.
	ByName(name string) (svc Service, ok bool)
	// StopService cancels the service's run context and waits for it
	// to unwind (or stopCtx to cancel). Equivalent to the ctl "stop"
	// op. Returns an error if the name is unknown.
	StopService(stopCtx context.Context, name string) error
}

// SupervisorBound is implemented by services that need to read or
// drive sibling services (api, tui). The supervisor calls Bind on
// each such service after construction and before Start.
type SupervisorBound interface {
	Bind(insp Inspector)
}
