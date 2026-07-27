// dap.go — the Debug Adapter Protocol front end (design/AQL-DEBUGGER.0.md
// §8.2): a SECOND front end over the same Session, so editors (VS Code
// and friends) drive the identical stepping, breakpoint, and inspection
// semantics the TTY prompt does. The transport is the LSP base protocol
// (Content-Length framed JSON over stdio) with DAP's request / response /
// event message shapes.
//
// Concurrency model: the adapter's serve loop owns everything. A reader
// goroutine feeds requests to a channel; the engine runs on its own
// goroutine and, at each pause, the session's pauseHook parks it on a
// channel handshake (holding the session lock) until the serve loop
// answers with an action. While parked, the serve loop is the only
// actor, so it may call the session's unlocked internals — exactly the
// contract the interactive prompt has. An `evaluate` runs on the serve
// goroutine; if the expression itself pauses (a Debug.break), OnStep's
// TryLock fails against the parked engine's hold and continues — the
// same guard that protects the prompt.
package debugger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aql-lang/aql/lang/go/native"
)

// dapMessage is the DAP wire shape — one struct for requests,
// responses, and events (the type field discriminates).
type dapMessage struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	// Success is explicit on every response — a refusal MUST serialize
	// "success":false, so no omitempty.
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Event   string `json:"event,omitempty"`
	Body    any    `json:"body,omitempty"`
}

type runResult struct {
	err error
}

type dapAdapter struct {
	wmu  sync.Mutex // guards the writer and seq
	w    *bufio.Writer
	seq  int
	msgs chan dapMessage

	pauseCh  chan PauseInfo
	actionCh chan string
	doneCh   chan runResult
}

// RunDAP drives the parsed program under a DAP session on in/out.
// Returns the process exit code: the program's failure (1) or 0.
func RunDAP(reg *native.Registry, cfg Config, tokens []native.Value, source string, in io.Reader, out io.Writer) int {
	a := &dapAdapter{
		w:        bufio.NewWriter(out),
		msgs:     make(chan dapMessage, 16),
		pauseCh:  make(chan PauseInfo),
		actionCh: make(chan string),
		doneCh:   make(chan runResult, 1),
	}
	sess := New(reg, Config{
		In: strings.NewReader(""), Out: io.Discard,
		File: cfg.File, Source: cfg.Source, BreakOnError: cfg.BreakOnError,
	})
	sess.pauseHook = func(info PauseInfo) string {
		a.pauseCh <- info
		return <-a.actionCh
	}
	// Program output becomes DAP "output" events.
	reg.Output = &dapOutputWriter{a: a}
	go a.readLoop(bufio.NewReader(in))
	return a.serve(sess, tokens, source)
}

// readLoop parses Content-Length framed messages until EOF or a framing
// error, then closes the channel — the serve loop treats that as a
// disconnect.
func (a *dapAdapter) readLoop(r *bufio.Reader) {
	defer close(a.msgs)
	for {
		m, err := readDAPMessage(r)
		if err != nil {
			return
		}
		a.msgs <- m
	}
}

// readDAPMessage reads one framed message: header lines to a blank
// line (Content-Length is required), then the JSON body.
func readDAPMessage(r *bufio.Reader) (dapMessage, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return dapMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			n, cerr := strconv.Atoi(strings.TrimSpace(v))
			if cerr != nil {
				return dapMessage{}, fmt.Errorf("dap: bad Content-Length %q", v)
			}
			length = n
		}
	}
	if length < 0 {
		return dapMessage{}, fmt.Errorf("dap: missing Content-Length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return dapMessage{}, err
	}
	var m dapMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return dapMessage{}, err
	}
	return m, nil
}

// write sends one framed message, stamping the adapter's seq.
func (a *dapAdapter) write(m dapMessage) {
	a.wmu.Lock()
	defer a.wmu.Unlock()
	a.seq++
	m.Seq = a.seq
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	fmt.Fprintf(a.w, "Content-Length: %d\r\n\r\n%s", len(data), data)
	_ = a.w.Flush()
}

func (a *dapAdapter) respond(req dapMessage, body any) {
	a.write(dapMessage{Type: "response", Command: req.Command, RequestSeq: req.Seq, Success: true, Body: body})
}

func (a *dapAdapter) refuse(req dapMessage, why string) {
	a.write(dapMessage{Type: "response", Command: req.Command, RequestSeq: req.Seq, Success: false, Message: why})
}

func (a *dapAdapter) event(name string, body any) {
	a.write(dapMessage{Type: "event", Event: name, Body: body})
}

// dapOutputWriter turns the program's Output writes into "output" events.
type dapOutputWriter struct{ a *dapAdapter }

func (w *dapOutputWriter) Write(p []byte) (int, error) {
	w.a.event("output", map[string]any{"category": "stdout", "output": string(p)})
	return len(p), nil
}

// serve is the adapter's state machine.
func (a *dapAdapter) serve(sess *Session, tokens []native.Value, source string) int {
	launched, configured, started := false, false, false
	paused := false
	exitCode := 0
	var curInfo PauseInfo

	maybeStart := func() {
		if launched && configured && !started {
			started = true
			go func() {
				_, err := sess.RunProgram(tokens, source)
				a.doneCh <- runResult{err: err}
			}()
		}
	}
	// resume releases the parked engine with an action (shared session
	// semantics via applyAction on the engine side).
	resume := func(action string) {
		paused = false
		a.actionCh <- action
	}
	// shutdown releases a parked engine with quit (detach) and DRAINS
	// the engine goroutine before returning, so no output event races
	// the caller. Requests are only consumed while paused or idle, so a
	// running-unparked engine can never reach here.
	shutdown := func() int {
		if paused {
			resume("quit")
		}
		if started {
			if res := <-a.doneCh; res.err != nil {
				exitCode = 1
			}
		}
		return exitCode
	}

	onPause := func(info PauseInfo) {
		paused, curInfo = true, info
		a.event("stopped", map[string]any{
			"reason":            stopReason(info.Kind),
			"threadId":          1,
			"allThreadsStopped": true,
			"description":       info.Detail,
		})
	}
	onDone := func(res runResult) {
		started = false
		if res.err != nil {
			exitCode = 1
			a.event("output", map[string]any{"category": "stderr", "output": res.err.Error() + "\n"})
		}
		a.event("exited", map[string]any{"exitCode": exitCode})
		a.event("terminated", map[string]any{})
	}

	for {
		// While the program runs un-paused, only the engine can speak:
		// requests queue until the next stop or termination. This keeps
		// the engine's (lock-holding) pause handshake deadlock-free and
		// makes scripted clients deterministic.
		if started && !paused {
			select {
			case info := <-a.pauseCh:
				onPause(info)
			case res := <-a.doneCh:
				onDone(res)
			}
			continue
		}
		// Paused or idle, the engine is parked (or absent): only the
		// client can speak.
		m, ok := <-a.msgs
		{
			if !ok {
				// Client hung up: treat as a disconnect.
				return shutdown()
			}
			switch m.Command {
			case "initialize":
				a.respond(m, map[string]any{"supportsConfigurationDoneRequest": true})
				a.event("initialized", map[string]any{})
			case "launch":
				// The program comes from the command line (`aql debug
				// --dap file.aql`); launch just gates the start.
				a.respond(m, nil)
				launched = true
				maybeStart()
			case "configurationDone":
				a.respond(m, nil)
				configured = true
				maybeStart()
			case "setBreakpoints":
				a.respond(m, a.setBreakpoints(sess, m, !paused))
			case "threads":
				a.respond(m, map[string]any{"threads": []map[string]any{{"id": 1, "name": "main"}}})
			case "stackTrace":
				if !paused {
					a.refuse(m, "not paused")
					break
				}
				a.respond(m, a.stackTrace(sess, curInfo))
			case "scopes":
				a.respond(m, map[string]any{"scopes": []map[string]any{
					{"name": "Defs", "variablesReference": 1, "expensive": false},
					{"name": "Stack", "variablesReference": 2, "expensive": false},
				}})
			case "variables":
				if !paused {
					a.refuse(m, "not paused")
					break
				}
				a.respond(m, a.variables(sess, m))
			case "evaluate":
				if !paused {
					a.refuse(m, "not paused")
					break
				}
				var args struct {
					Expression string `json:"expression"`
				}
				_ = json.Unmarshal(m.Arguments, &args)
				a.respond(m, map[string]any{
					"result": sess.evalText(args.Expression), "variablesReference": 0,
				})
			case "continue":
				a.respond(m, map[string]any{"allThreadsContinued": true})
				if paused {
					resume("continue")
				}
			case "next":
				a.respond(m, nil)
				if paused {
					resume("next")
				}
			case "stepIn":
				a.respond(m, nil)
				if paused {
					resume("step")
				}
			case "stepOut":
				a.respond(m, nil)
				if paused {
					resume("out")
				}
			case "pause":
				// No mid-run preemption seam (ADR-005): honest refusal.
				a.refuse(m, "pause is not supported: the engine has no preemption seam")
			case "disconnect", "terminate":
				a.respond(m, nil)
				return shutdown()
			default:
				a.refuse(m, "unsupported request: "+m.Command)
			}
		}
	}
}

// stopReason maps a PauseInfo kind onto DAP's stopped-event reasons.
func stopReason(kind string) string {
	switch kind {
	case "breakpoint", "marker":
		return "breakpoint"
	case "fault":
		return "exception"
	}
	return "step"
}

// setBreakpoints REPLACES the session's line breakpoints with the
// request's set (the DAP per-source contract). Un-paused (configuring,
// running, or finished) the session lock guards the mutation; while
// parked at a pause the engine holds the lock and the serve loop
// mutates directly.
func (a *dapAdapter) setBreakpoints(sess *Session, m dapMessage, needLock bool) map[string]any {
	var args struct {
		Breakpoints []struct {
			Line int `json:"line"`
		} `json:"breakpoints"`
	}
	_ = json.Unmarshal(m.Arguments, &args)
	if needLock {
		sess.mu.Lock()
		defer sess.mu.Unlock()
	}
	sess.lineBPs = make(map[int]bool)
	verified := make([]map[string]any, 0, len(args.Breakpoints))
	for _, bp := range args.Breakpoints {
		sess.SetBreak(strconv.Itoa(bp.Line))
		verified = append(verified, map[string]any{"verified": true, "line": bp.Line})
	}
	return map[string]any{"breakpoints": verified}
}

// stackTrace renders the pause location plus the live frame chain. Only
// the pausing token's row is known (frames carry no positions yet — the
// design's file-identity gap), so deeper frames report line 0.
func (a *dapAdapter) stackTrace(sess *Session, info PauseInfo) map[string]any {
	entries := sess.frameEntries() // outermost first
	top := "main"
	if len(entries) > 0 {
		top = entries[len(entries)-1].name
	}
	frames := []map[string]any{{
		"id": 0, "name": top, "line": info.Row, "column": 1,
		"source": map[string]any{"path": sess.file},
	}}
	for k := len(entries) - 2; k >= 0; k-- {
		frames = append(frames, map[string]any{
			"id": len(frames), "name": entries[k].name, "line": 0, "column": 0,
			"presentationHint": "subtle",
		})
	}
	return map[string]any{"stackFrames": frames, "totalFrames": len(frames)}
}

// variables serves the two fixed scopes: 1 = the program's defs,
// 2 = the data stack (top first).
func (a *dapAdapter) variables(sess *Session, m dapMessage) map[string]any {
	var args struct {
		VariablesReference int `json:"variablesReference"`
	}
	_ = json.Unmarshal(m.Arguments, &args)
	vars := []map[string]any{}
	if args.VariablesReference == 1 {
		defs := sess.programDefs(false)
		sort.Slice(defs, func(i, j int) bool { return defs[i][0] < defs[j][0] })
		for _, d := range defs {
			vars = append(vars, map[string]any{"name": d[0], "value": d[1], "variablesReference": 0})
		}
	} else {
		vals, _ := sess.dataStack()
		for i := len(vals) - 1; i >= 0; i-- {
			vars = append(vars, map[string]any{
				"name":  fmt.Sprintf("#%d", len(vals)-1-i),
				"value": native.FormatForPrint(vals[i]), "variablesReference": 0,
			})
		}
	}
	return map[string]any{"variables": vars}
}
