package modules

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// The remote tier of design/TUI.0.md §6 (implementation plan P4):
// `Tui.serve {tcp: <port> token: "…"} app` runs the SAME driver loop as
// Tui.run against the remote half of the renderer seam — widget trees
// go down the wire as json-lines (§6.2), decoded events come up, the
// attach client (cmd/go/internal/attach) lays out locally. One viewer
// at a time; the viewer disconnecting quits the app (§6.3); token auth
// is a constant-time compare with `token: "none"` as the explicit
// localhost opt-out.

// Test seams (design/TEST-SEAMS.10.md).
var (
	tuiListen     = net.Listen
	tuiServeBound = func(net.Addr) {}
)

// tuiHandshakeTimeout bounds how long an attaching connection may take
// to present its handshake line.
const tuiHandshakeTimeout = 10 * time.Second

// tuiWireLimit bounds one json-line (a paste event or a large tree).
const tuiWireLimit = 4 << 20

type tuiSession struct {
	conn net.Conn
	cols int
	rows int
}

func tuiServeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	opts, err := native.RequireConcreteMap(args[0], "serve")
	if err != nil {
		return nil, r.AqlError("tui_error", "serve: expected an options Map", "serve")
	}
	port, hasPort, pErr := nodePortOf(opts)
	if pErr != nil || !hasPort {
		return nil, r.AqlError("tui_error", "serve: tcp: must be an Integer port", "serve")
	}
	token, tErr := tuiOptString(opts, "token")
	if tErr != nil {
		return nil, r.AqlError("tui_error", "serve: "+tErr.Error(), "serve")
	}
	if token == "" {
		return nil, r.AqlError("tui_error", `serve: token: is required (use token: "none" to opt out on a trusted network)`, "serve")
	}
	if err := checkNetPolicy(r, "listen", "", port); err != nil {
		return nil, err
	}
	if err := checkTuiPolicy(r, "serve"); err != nil {
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
		return nil, r.AqlError("already_running", "serve: another TUI app already owns the terminal", "serve")
	}

	ln, lErr := tuiListen("tcp", fmt.Sprintf(":%d", port))
	if lErr != nil {
		return nil, mapNetErr(r, "serve", lErr)
	}
	tuiServeBound(ln.Addr())

	// the acceptor owns the listener: the first authenticated handshake
	// becomes THE session; every later connection is denied busy.
	sessions := make(chan tuiSession, 1)
	go tuiAcceptor(ln, token, sessions)

	session, ok := <-sessions
	if !ok {
		return nil, r.AqlError("closed", "serve: listener closed before a viewer attached", "serve")
	}
	defer func() { _ = session.conn.Close() }()

	proc := eng.NewProcess(rt, eng.DefaultMailboxBound, eng.OverflowBlock)
	if err := rt.Insert(proc); err != nil {
		_ = session.conn.Close()
		_ = ln.Close()
		return nil, mapTuiErr(r, "serve", err)
	}
	if err := rt.RegisterName(tuiRegisteredName, proc); err != nil { //covergate:allow RegisterName fails only when the runtime is down, which the Insert call above already rejected; the guard covers the down-race between the two calls (§modules)
		proc.Close()
		_ = session.conn.Close()
		_ = ln.Close()
		return nil, mapTuiErr(r, "serve", err)
	}

	events := make(chan tuikit.Event, 64)
	go tuiWireReader(session.conn, events)

	// finish releases the LISTENER (stopping the acceptor); the session
	// conn outlives the driver so the quit goodbye below can still be
	// written, and the deferred close above releases it on every path.
	var closeOnce sync.Once
	finish := func() {
		closeOnce.Do(func() { _ = ln.Close() })
	}
	writeLine := func(payload any) error {
		data, mErr := json.Marshal(payload)
		if mErr != nil { //covergate:allow json.Marshal cannot fail on the wire payload shapes this handler builds (tag/seq/title scalars and the jsonReady-projected tree); kept as the io seam's defensive twin (§modules)
			return mErr
		}
		_, wErr := session.conn.Write(append(data, '\n'))
		return wErr
	}
	if app.opts.Title != "" {
		_ = writeLine(map[string]any{"tag": "title", "text": app.opts.Title})
	}

	d := &tuiDriver{
		reg: r, app: app, proc: proc,
		events: events,
		paint:  tuiWirePaint(writeLine),
		finish: finish,
		cols:   session.cols, rows: session.rows,
	}
	final, runErr := d.run()
	// the goodbye: the driver released the listener, the conn is still
	// ours until the deferred close — a vanished viewer just makes this
	// write fail, which is fine
	_ = writeLine(map[string]any{"tag": "quit"})
	rt.UnregisterName(tuiRegisteredName)
	proc.Close()
	if runErr != nil {
		return nil, runErr
	}
	return []native.Value{final}, nil
}

// tuiWirePaint is the remote half of the renderer seam: the view tree
// is NOT laid out here — it goes down the wire whole (§6.2) and the
// attach client renders it locally. A write failure means the viewer
// vanished, which is the graceful §6.3 disconnect-quit.
func tuiWirePaint(writeLine func(any) error) func(native.Value, int, int) error {
	seq := 0
	return func(tree native.Value, _, _ int) error {
		seq++
		if wErr := writeLine(map[string]any{"tag": "frame", "seq": seq, "tree": jsonReady(native.ValueToAny(tree))}); wErr != nil {
			return errTuiViewerGone
		}
		return nil
	}
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

// tuiAcceptor handshakes connections: the first authenticated attach
// becomes the session (channel then closed to later joiners via the
// busy denial). It exits when the listener closes.
func tuiAcceptor(ln net.Listener, token string, sessions chan<- tuiSession) {
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow acceptor recover body: the loop calls Accept on a listener this handler minted plus the panic-free handshake helper (§modules)
			_ = rec
		}
	}()
	claimed := false
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !claimed {
				close(sessions)
			}
			return
		}
		if claimed {
			_ = tuiWriteDeny(conn, "busy")
			_ = conn.Close()
			continue
		}
		cols, rows, why := tuiHandshake(conn, token)
		if why != "" {
			_ = tuiWriteDeny(conn, why)
			_ = conn.Close()
			continue
		}
		claimed = true
		sessions <- tuiSession{conn: conn, cols: cols, rows: rows}
	}
}

func tuiWriteDeny(conn net.Conn, why string) error {
	data, _ := json.Marshal(map[string]any{"tag": "deny", "why": why})
	_, err := conn.Write(append(data, '\n'))
	return err
}

// tuiHandshake validates one attach line; the returned why is "" on
// success ("bad-token" / "proto" / "transport" otherwise).
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
	data, _ := json.Marshal(map[string]any{"tag": "accept", "proto": 1})
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return 0, 0, "transport"
	}
	if hello.Cols <= 0 {
		hello.Cols = 80
	}
	if hello.Rows <= 0 {
		hello.Rows = 24
	}
	return hello.Cols, hello.Rows, ""
}

// tuiWireReader decodes client json-lines into driver events; on EOF or
// error it emits the disconnect marker and closes the channel.
func tuiWireReader(conn net.Conn, events chan<- tuikit.Event) {
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow wire-reader recover body: the loop only scans a connection and sends on a buffered channel, both panic-free (§modules)
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
			case events <- tuikit.Event{
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
	select {
	case events <- tuikit.Event{Tag: "__disconnect"}:
	default:
	}
	close(events)
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
