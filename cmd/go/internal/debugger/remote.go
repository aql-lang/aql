// remote.go — the remote stepping adapter (design/AQL-DEBUGGER.0.md
// §8.3, Phase 4): a THIRD front end over the same Session, publishing
// pauses over the debugserve HTTP transport so a separate process can
// drive stepping with `aql debug attach`.
//
// Concurrency model, inherited from the DAP adapter: the engine parks
// inside the session's pauseHook (holding the session lock) until an
// action arrives. HTTP handlers are the other actors; the adapter's own
// mutex serializes them, and the paused/busy flags encode who may act:
// an action or eval needs a PARKED engine (paused, not busy); pause
// polls are read-only and always allowed. Delivering an action flips
// paused BEFORE the channel send, so a second action cannot double-
// resume; an eval marks busy so an action cannot resume the engine
// while the eval runs against the parked program.
package debugger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/aql-lang/aql/lang/go/debugserve"
)

// Remote publishes one Session's pauses over HTTP and accepts actions.
type Remote struct {
	sess *Session

	mu       sync.Mutex
	seq      int
	paused   bool
	busy     bool // an eval is running against the parked engine
	info     PauseInfo
	exited   bool
	exitErr  string
	actionCh chan string
	doneCh   chan struct{}
}

// NewRemote wires a Remote as the session's pause front end. Call
// before RunProgram; then run the program on its own goroutine and
// call Finish with its error.
func NewRemote(sess *Session) *Remote {
	rm := &Remote{sess: sess, actionCh: make(chan string), doneCh: make(chan struct{})}
	sess.pauseHook = rm.onPause
	return rm
}

// onPause is the session's pauseHook: publish, park, await an action.
func (rm *Remote) onPause(info PauseInfo) string {
	rm.mu.Lock()
	rm.seq++
	rm.paused, rm.info = true, info
	rm.mu.Unlock()
	return <-rm.actionCh
}

// Finish records the program's completion (call from the run
// goroutine). After it, actions and evals refuse with "exited".
func (rm *Remote) Finish(runErr error) {
	rm.mu.Lock()
	rm.exited = true
	if runErr != nil {
		rm.exitErr = runErr.Error()
	}
	rm.mu.Unlock()
	close(rm.doneCh)
}

// Shutdown releases a parked engine with quit (detach) and waits for
// the run goroutine to drain — the serve loop calls it after the HTTP
// server stops, so the program's side effects complete before exit.
func (rm *Remote) Shutdown() {
	rm.mu.Lock()
	deliver := rm.paused && !rm.busy
	if deliver {
		rm.paused = false
	}
	rm.mu.Unlock()
	if deliver {
		rm.actionCh <- "quit"
	}
	<-rm.doneCh
}

// ExitError returns the finished program's error text ("" for a clean
// run) — the serve loop reports it after Shutdown.
func (rm *Remote) ExitError() string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.exitErr
}

// state snapshots the wire view under the adapter's lock.
func (rm *Remote) state() debugserve.StepState {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	st := debugserve.StepState{Seq: rm.seq}
	switch {
	case rm.exited:
		st.State, st.Exit = "exited", rm.exitErr
	case rm.paused:
		st.State = "paused"
		st.Kind, st.Row, st.Detail, st.InBody = rm.info.Kind, rm.info.Row, rm.info.Detail, rm.info.InBody
		st.File, st.Line, st.Stack = rm.info.File, rm.info.Line, rm.info.Stack
	default:
		st.State = "running"
	}
	return st
}

// Register mounts the stepping routes on mux, each behind the given
// auth gate (the debugserve server's own Bearer check). /debug/eval is
// mounted too, SHADOWING the introspection eval: a raw registry eval
// would fire the armed debug hook from the HTTP goroutine and block on
// the parked engine's session lock — this one goes through the
// session's eval (hook suppressed) under the busy flag instead.
func (rm *Remote) Register(mux *http.ServeMux, auth func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/debug/pause", auth(rm.handlePause))
	mux.HandleFunc("/debug/action", auth(rm.handleAction))
	mux.HandleFunc("/debug/break", auth(rm.handleBreak))
	mux.HandleFunc("/debug/eval", auth(rm.handleEval))
}

func (rm *Remote) handlePause(w http.ResponseWriter, _ *http.Request) {
	writeStepJSON(w, http.StatusOK, rm.state())
}

// takeParked claims the parked engine for an action delivery: the
// claim IS the resume (paused flips false before the send, so a
// second action cannot double-resume). Returns a refusal message when
// the program is not deliverable.
func (rm *Remote) takeParked() string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	switch {
	case rm.exited:
		return "the program has exited"
	case !rm.paused:
		return "the program is running — not paused"
	case rm.busy:
		return "another remote operation is in progress"
	}
	rm.paused = false
	return ""
}

func (rm *Remote) handleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal(body, &req)
	switch req.Action {
	case "step", "next", "out", "continue", "quit":
	default:
		writeStepErr(w, http.StatusBadRequest, fmt.Sprintf("unknown action %q", req.Action))
		return
	}
	if why := rm.takeParked(); why != "" {
		writeStepErr(w, http.StatusConflict, why)
		return
	}
	rm.actionCh <- req.Action
	writeStepJSON(w, http.StatusOK, rm.state())
}

// claimQuiet claims the session for a read-modify operation (an eval,
// a breakpoint edit) in whichever quiet state holds: PARKED (the
// engine blocked in the hook, session lock held by it — the busy flag
// is the claim) or EXITED (no engine at all — busy still serializes
// concurrent handlers). Returns a refusal message while the program
// RUNS un-paused, or while another claim is live. release with
// releaseQuiet.
func (rm *Remote) claimQuiet() string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	switch {
	case rm.busy:
		return "another remote operation is in progress"
	case rm.exited, rm.paused:
		rm.busy = true
		return ""
	}
	return "the program is running — not paused"
}

func (rm *Remote) releaseQuiet() {
	rm.mu.Lock()
	rm.busy = false
	rm.mu.Unlock()
}

func (rm *Remote) handleEval(w http.ResponseWriter, r *http.Request) {
	src, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if why := rm.claimQuiet(); why != "" {
		writeStepErr(w, http.StatusConflict, why)
		return
	}
	// Parked: the engine holds the session lock, blocked in the hook;
	// with busy held no action can resume it, so the session's
	// internals are ours — the DAP serve loop's single-actor contract.
	// Exited: no engine at all, busy serializes the handlers.
	result := rm.sess.evalText(string(src))
	rm.releaseQuiet()
	writeStepJSON(w, http.StatusOK, map[string]string{"result": result})
}

func (rm *Remote) handleBreak(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Spec   string `json:"spec"`
		Delete bool   `json:"delete"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal(body, &req)
	if req.Spec == "" {
		writeStepErr(w, http.StatusBadRequest, "missing breakpoint spec")
		return
	}
	// Quiet states (parked/exited) mutate under the busy claim; a
	// RUNNING program's maps are guarded by the session lock, which the
	// trace holds only across one fire — TryLock between fires (a plain
	// Lock could park behind a pause and hang until the next action).
	quiet := rm.claimQuiet() == ""
	if !quiet && !rm.sess.mu.TryLock() {
		writeStepErr(w, http.StatusConflict, "the session is busy — retry")
		return
	}
	var result string
	if req.Delete {
		result = rm.sess.deleteBreak(req.Spec)
	} else {
		result = rm.sess.SetBreak(req.Spec)
	}
	if quiet {
		rm.releaseQuiet()
	} else {
		rm.sess.mu.Unlock()
	}
	writeStepJSON(w, http.StatusOK, map[string]string{"result": result})
}

func writeStepJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeStepErr(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}
