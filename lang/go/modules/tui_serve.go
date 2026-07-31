package modules

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// The remote tier of design/TUI.0.md §6 (implementation plan P4, v2
// session rules P6): `Tui.serve {tcp token viewers? reattach?} app`
// runs the SAME driver loop as Tui.run against the remote half of the
// renderer seam — widget trees go down the wire as json-lines (§6.2),
// decoded events come up, the attach client (cmd/go/internal/attach)
// lays out locally at ITS own geometry. Sessions admit up to `viewers`
// concurrent viewers (default 1 — the v1 rule); frames broadcast to
// all, input merges from all, and a frame identical to the previous
// one is not re-sent. Losing the LAST viewer quits the app unless
// `reattach: true`, in which case the app keeps running headless and a
// later viewer resumes with the title and current frame. Token auth is
// a constant-time compare with `token: "none"` as the explicit opt-out.

// Test seams (design/TEST-SEAMS.10.md).
var (
	tuiListen       = net.Listen
	tuiServeBound   = func(net.Addr) {}
	tuiWriteTimeout = 10 * time.Second
)

// tuiHandshakeTimeout bounds how long an attaching connection may take
// to present its handshake line.
const tuiHandshakeTimeout = 10 * time.Second

// tuiWireLimit bounds one json-line (a paste event or a large tree).
const tuiWireLimit = 4 << 20

// tuiMaxViewers caps the viewers: option.
const tuiMaxViewers = 64

type tuiSession struct {
	cols int
	rows int
}

// tuiServeOpts is the parsed transport configuration.
type tuiServeOpts struct {
	port     int
	token    string
	viewers  int
	reattach bool
}

// tuiViewerHub owns a session's viewer set: admission up to max,
// frame/title broadcast with per-write deadlines, unchanged-frame
// suppression, late-joiner replay, and the last-viewer-gone verdict.
type tuiViewerHub struct {
	mu        sync.Mutex
	viewers   map[int]net.Conn
	nextID    int
	max       int
	reattach  bool
	closed    bool // goodbye has run; no more admissions or writes
	lastTree  []byte
	lastLine  []byte
	titleLine []byte
	seq       int
	events    chan tuikit.Event
	readers   sync.WaitGroup
}

func newTuiViewerHub(max int, reattach bool) *tuiViewerHub {
	return &tuiViewerHub{
		viewers: map[int]net.Conn{}, max: max, reattach: reattach,
		events: make(chan tuikit.Event, 64),
	}
}

// admit registers a handshaken connection. The false return means the
// session is full (or over) — the caller denies busy. The reader's
// WaitGroup slot is taken here, under the same lock that guards
// closed, so teardown's Wait cannot race a late Add.
func (h *tuiViewerHub) admit(conn net.Conn) (id int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.viewers) >= h.max {
		return 0, false
	}
	h.nextID++
	id = h.nextID
	h.viewers[id] = conn
	h.readers.Add(1)
	return id, true
}

// replay sends a just-accepted viewer the chrome it missed — the title
// and the current frame — so late joiners resume mid-session. It runs
// AFTER the accept reply so the wire stays hello → accept → chrome.
func (h *tuiViewerHub) replay(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.viewers[id]
	if !ok {
		return
	}
	if h.titleLine != nil {
		_ = h.writeTo(conn, h.titleLine)
	}
	if h.lastLine != nil {
		_ = h.writeTo(conn, h.lastLine)
	}
}

// evict removes a viewer whose accept reply never landed — no reader
// ever started and no disconnect verdict fires (the viewer was never
// part of the session from the app's point of view).
func (h *tuiViewerHub) evict(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.viewers[id]; !ok {
		return
	}
	delete(h.viewers, id)
	h.readers.Done()
}

// writeTo writes one line with the broadcast deadline. Callers hold mu.
func (h *tuiViewerHub) writeTo(conn net.Conn, line []byte) error {
	_ = conn.SetWriteDeadline(time.Now().Add(tuiWriteTimeout))
	_, err := conn.Write(line)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

// drop removes a viewer (read-side EOF). When the LAST viewer leaves a
// non-reattach session, the driver hears the §6.3 disconnect.
func (h *tuiViewerHub) drop(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.viewers[id]; !ok {
		return
	}
	delete(h.viewers, id)
	if len(h.viewers) == 0 && !h.reattach && !h.closed {
		select {
		case h.events <- tuikit.Event{Tag: "__disconnect"}:
		default:
		}
	}
}

// broadcastFrame sends the marshaled tree to every live viewer (a
// failed or timed-out write drops that viewer). A tree identical to
// the previous frame is suppressed — stored but not sent. The returned
// alive is false when a non-reattach session has no viewers left.
func (h *tuiViewerHub) broadcastFrame(treeJSON []byte) (alive bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return true
	}
	if h.lastTree != nil && bytes.Equal(h.lastTree, treeJSON) {
		return h.reattach || len(h.viewers) > 0
	}
	h.seq++
	line := append([]byte(fmt.Sprintf(`{"tag":"frame","seq":%d,"tree":`, h.seq)), treeJSON...)
	line = append(line, '}', '\n')
	h.lastTree = treeJSON
	h.lastLine = line
	for id, conn := range h.viewers {
		if err := h.writeTo(conn, line); err != nil {
			_ = conn.Close()
			delete(h.viewers, id)
		}
	}
	return h.reattach || len(h.viewers) > 0
}

// setTitle records the session title and sends it to current viewers.
func (h *tuiViewerHub) setTitle(text string) {
	data, _ := json.Marshal(map[string]any{"tag": "title", "text": text})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.titleLine = append(data, '\n')
	for id, conn := range h.viewers {
		if err := h.writeTo(conn, h.titleLine); err != nil {
			_ = conn.Close()
			delete(h.viewers, id)
		}
	}
}

// goodbye ends the session: the quit line goes to every live viewer,
// every connection closes, and no further admissions or writes happen.
func (h *tuiViewerHub) goodbye() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	line := []byte(`{"tag":"quit"}` + "\n")
	for _, conn := range h.viewers {
		_ = h.writeTo(conn, line)
		_ = conn.Close()
	}
	h.viewers = map[int]net.Conn{}
}

func tuiServeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	opts, err := tuiServeOptsOf(args[0], r)
	if err != nil {
		return nil, err
	}
	if err := checkNetPolicy(r, "serve", "listen", "", opts.port); err != nil {
		return nil, err
	}
	if err := checkTuiPolicy(r, "serve", "serve"); err != nil {
		return nil, err
	}
	app, err := parseTuiApp(args[1], "serve", r)
	if err != nil {
		return nil, err
	}

	rt := r.Procs
	if rt == nil {
		rt = eng.NewProcessRuntime()
		r.Procs = rt
	}
	if _, taken := rt.Whereis(tuiRegisteredName); taken {
		return nil, r.BoruError("already_running", "serve: another TUI app already owns the terminal", "serve")
	}

	ln, lErr := tuiListen("tcp", fmt.Sprintf(":%d", opts.port))
	if lErr != nil {
		return nil, mapNetErr(r, "serve", lErr)
	}
	tuiServeBound(ln.Addr())

	hub := newTuiViewerHub(opts.viewers, opts.reattach)
	// the acceptor owns the listener: every authenticated handshake is
	// admitted up to the viewer cap; the rest are denied busy.
	firstCh := make(chan tuiSession, 1)
	go tuiAcceptor(hub, ln, opts.token, firstCh)

	session, ok := <-firstCh
	if !ok {
		return nil, r.BoruError("closed", "serve: listener closed before a viewer attached", "serve")
	}

	teardown := func() {
		_ = ln.Close()
		hub.goodbye()
		hub.readers.Wait()
	}

	proc := eng.NewProcess(rt, eng.DefaultMailboxBound, eng.OverflowBlock)
	if err := rt.Insert(proc); err != nil {
		teardown()
		return nil, mapTuiErr(r, "serve", err)
	}
	if err := rt.RegisterName(tuiRegisteredName, proc); err != nil { //covergate:allow RegisterName fails only when the runtime is down, which the Insert call above already rejected; the guard covers the down-race between the two calls (§modules)
		proc.Close()
		teardown()
		return nil, mapTuiErr(r, "serve", err)
	}

	if app.opts.Title != "" {
		hub.setTitle(app.opts.Title)
	}

	d := &tuiDriver{
		reg: r, app: app, proc: proc,
		events: hub.events,
		paint:  tuiWirePaint(r, hub),
		// finish releases only the LISTENER: the goodbye below still
		// writes the quit line to live viewers on every exit path
		finish: func() { _ = ln.Close() },
		cols:   session.cols, rows: session.rows,
	}
	final, runErr := d.run()
	teardown()
	close(hub.events)
	rt.UnregisterName(tuiRegisteredName)
	proc.Close()
	if runErr != nil {
		return nil, runErr
	}
	return []native.Value{final}, nil
}

// tuiWirePaint is the remote half of the renderer seam: the view tree
// is NOT laid out here — it goes down the wire whole (§6.2) and each
// attach client renders it locally at its own size. The tree is still
// VALIDATED here (a throwaway render at the session's dims): a bad
// view raises bad_widget at the serving app, exactly like Tui.run,
// instead of a layout failure disconnecting every client. Losing the
// last viewer of a non-reattach session is the graceful §6.3
// disconnect-quit.
func tuiWirePaint(r *native.Registry, hub *tuiViewerHub) func(native.Value, int, int) error {
	return func(tree native.Value, cols, rows int) error {
		if _, rErr := tuikit.Render(native.ValueToAny(tree), cols, rows); rErr != nil {
			return r.BoruError("bad_widget", "serve: "+rErr.Error(), "serve")
		}
		data, mErr := json.Marshal(jsonReady(native.ValueToAny(tree)))
		if mErr != nil { //covergate:allow json.Marshal cannot fail on the jsonReady-projected tree shapes the renderer accepts; kept as the io seam's defensive twin (§modules)
			return mErr
		}
		if !hub.broadcastFrame(data) {
			return errTuiViewerGone
		}
		return nil
	}
}

// tuiServeOptsOf validates the transport options map.
func tuiServeOptsOf(v native.Value, r *native.Registry) (tuiServeOpts, error) {
	out := tuiServeOpts{viewers: 1}
	opts, err := native.RequireConcreteMap(v, "serve")
	if err != nil {
		return out, r.BoruError("tui_error", "serve: expected an options Map", "serve")
	}
	port, hasPort, pErr := nodePortOf(opts)
	if pErr != nil || !hasPort {
		return out, r.BoruError("tui_error", "serve: tcp: must be an Integer port", "serve")
	}
	out.port = port
	if out.token, err = tuiOptString(opts, "token"); err != nil {
		return out, r.BoruError("tui_error", "serve: "+err.Error(), "serve")
	}
	if out.token == "" {
		return out, r.BoruError("tui_error", `serve: token: is required (use token: "none" to opt out on a trusted network)`, "serve")
	}
	if vv, ok := opts.Get("viewers"); ok {
		n, nErr := vv.AsConcreteInteger()
		if nErr != nil || n < 1 || n > tuiMaxViewers {
			return out, r.BoruError("tui_error",
				fmt.Sprintf("serve: viewers: must be an Integer between 1 and %d", tuiMaxViewers), "serve")
		}
		out.viewers = int(n)
	}
	if out.reattach, err = tuiOptBoolean(opts, "reattach"); err != nil {
		return out, r.BoruError("tui_error", "serve: "+err.Error(), "serve")
	}
	return out, nil
}

// nodePortOf reads the tcp: port from the serve options.
func nodePortOf(mp native.ReadMap) (int, bool, error) {
	v, ok := mp.Get("tcp")
	if !ok {
		return 0, false, nil
	}
	n, err := v.AsConcreteInteger()
	if err != nil || n < 0 || n > 65535 {
		return 0, false, fmt.Errorf("tcp: must be a port number")
	}
	return int(n), true, nil
}

// tuiAcceptor handshakes connections forever: each authenticated
// attach is admitted up to the hub's cap and only THEN accepted on the
// wire (an unauthenticated probe learns nothing, a full session denies
// busy). It exits when the listener closes; the channel closes if no
// viewer ever completed an attach.
func tuiAcceptor(hub *tuiViewerHub, ln net.Listener, token string, firstCh chan<- tuiSession) {
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow acceptor recover body: the loop calls Accept on a listener this handler minted plus the panic-free handshake/hub helpers (§modules)
			_ = rec
		}
	}()
	sentFirst := false
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !sentFirst {
				close(firstCh)
			}
			return
		}
		cols, rows, why := tuiHandshake(conn, token)
		if why != "" {
			_ = tuiWriteDeny(conn, why)
			_ = conn.Close()
			continue
		}
		id, ok := hub.admit(conn)
		if !ok {
			_ = tuiWriteDeny(conn, "busy")
			_ = conn.Close()
			continue
		}
		if wErr := tuiWriteAccept(conn); wErr != nil {
			hub.evict(id)
			_ = conn.Close()
			continue
		}
		hub.replay(id)
		go tuiWireReader(hub, id, conn)
		if !sentFirst {
			sentFirst = true
			firstCh <- tuiSession{cols: cols, rows: rows}
		}
	}
}

func tuiWriteAccept(conn net.Conn) error {
	data, _ := json.Marshal(map[string]any{"tag": "accept", "proto": 1})
	_, err := conn.Write(append(data, '\n'))
	return err
}

func tuiWriteDeny(conn net.Conn, why string) error {
	data, _ := json.Marshal(map[string]any{"tag": "deny", "why": why})
	_, err := conn.Write(append(data, '\n'))
	return err
}

// tuiHandshake validates one attach line WITHOUT replying — the accept
// goes out only after admission; the returned why is "" on success
// ("bad-token" / "proto" / "transport" otherwise).
func tuiHandshake(conn net.Conn, token string) (int, int, string) {
	_ = conn.SetReadDeadline(time.Now().Add(tuiHandshakeTimeout))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	if !scanner.Scan() {
		return 0, 0, "transport"
	}
	_ = conn.SetReadDeadline(time.Time{})
	var hello struct {
		Tag   string `json:"tag"`
		Token string `json:"token"`
		Cols  int    `json:"cols"`
		Rows  int    `json:"rows"`
		Proto int    `json:"proto"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &hello); err != nil || hello.Tag != "attach" {
		return 0, 0, "transport"
	}
	if hello.Proto != 1 {
		return 0, 0, "proto"
	}
	if token != "none" && subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) != 1 {
		return 0, 0, "bad-token"
	}
	if hello.Cols <= 0 {
		hello.Cols = 80
	}
	if hello.Rows <= 0 {
		hello.Rows = 24
	}
	return hello.Cols, hello.Rows, ""
}

// tuiWireReader decodes one viewer's json-lines into the shared event
// stream; on EOF or error the viewer is dropped (which becomes the
// driver's disconnect when it was the last of a non-reattach session).
func tuiWireReader(hub *tuiViewerHub, id int, conn net.Conn) {
	defer hub.readers.Done()
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow wire-reader recover body: the loop only scans a connection and takes the hub mutex, both panic-free (§modules)
			_ = rec
		}
	}()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	for scanner.Scan() {
		var wire struct {
			Tag    string   `json:"tag"`
			Key    string   `json:"key"`
			Char   string   `json:"char"`
			Mods   []string `json:"mods"`
			Cols   int      `json:"cols"`
			Rows   int      `json:"rows"`
			Kind   string   `json:"kind"`
			X      int      `json:"x"`
			Y      int      `json:"y"`
			Text   string   `json:"text"`
			Gained bool     `json:"gained"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &wire); err != nil {
			continue // a malformed line is dropped, not fatal
		}
		switch wire.Tag {
		case "key", "resize", "mouse", "paste", "focus":
			// non-blocking: a storm sheds stale input rather than
			// stranding this goroutine after the driver exits
			select {
			case hub.events <- tuikit.Event{
				Tag: wire.Tag, Key: wire.Key, Char: wire.Char, Mods: wire.Mods,
				Cols: wire.Cols, Rows: wire.Rows, Kind: wire.Kind,
				X: wire.X, Y: wire.Y, Text: wire.Text, Gained: wire.Gained,
			}:
			default:
			}
		default:
			// unknown client messages are dropped
		}
	}
	hub.drop(id)
}

// tuiServeNatives lists the remote-tier words.
func tuiServeNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return []native.NativeFunc{
		{Name: "serve", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TMap), Impl: native.Go(tuiServeHandler), Returns: T(native.TAny),
				BarrierPos: -1, CompileEffect: native.CompileStoresFn},
		}},
	}
}
