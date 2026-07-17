package modules

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// Loopback coverage for the remote tier (tui_serve.go): a real
// ephemeral listener, a raw json-lines wire client, the full
// attach→frames→events→quit choreography, and the §6.3 denial rules —
// negatives paired throughout.

// serveApp quits on the "q" key carrying its state. The title rides in
// the app config (the SAME map shape Tui.run takes — §6.1): serve
// forwards it as a wire line instead of setting it locally.
const serveApp = `def app {
  init: {n: 41}
  title: "served"
  update: ([state:Map ev:Map] => [ case ev.key [
      "up" [ drop state set n (add state.n 1) ]
      "boom" [ drop raise "kaboom" ]
      "q"  [ drop Tui.quit state ]
      state ] ])
  view: ([state:Map] => [ Tui.text (convert String state.n) ])
}`

// startServe launches Tui.serve on its own registry/goroutine and
// reports the bound address plus the final-result channel.
func startServe(t *testing.T) (net.Addr, chan error, chan string) {
	t.Helper()
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }

	errs := make(chan error, 1)
	finals := make(chan string, 1)
	go func() {
		reg, rErr := native.DefaultRegistry()
		if rErr != nil {
			errs <- rErr
			return
		}
		out, sErr := runTuiStepsOn(t, reg, []string{
			`import "aql:tui"`, serveApp,
			`def final (Tui.serve {tcp: 0  token: "s3cret"} app)`,
			`convert String final.n`,
		})
		if sErr != nil {
			errs <- sErr
			return
		}
		finals <- func() string {
			s, _ := out[len(out)-1].AsConcreteString()
			return s
		}()
	}()
	select {
	case addr := <-bound:
		return addr, errs, finals
	case err := <-errs:
		t.Fatalf("serve failed before binding: %v", err)
		return nil, nil, nil
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
		return nil, nil, nil
	}
}

type wireClient struct {
	conn    net.Conn
	scanner *bufio.Scanner
}

func dialWire(t *testing.T, addr net.Addr) *wireClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	return &wireClient{conn: conn, scanner: sc}
}

func (w *wireClient) send(t *testing.T, payload map[string]any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.conn.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func (w *wireClient) recv(t *testing.T) map[string]any {
	t.Helper()
	_ = w.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !w.scanner.Scan() {
		t.Fatalf("wire closed: %v", w.scanner.Err())
	}
	var msg map[string]any
	if err := json.Unmarshal(w.scanner.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestTuiServeLoopback(t *testing.T) {
	addr, errs, finals := startServe(t)
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 12, "rows": 3, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	// a second viewer is denied busy while the session is live
	c2 := dialWire(t, addr)
	if msg := c2.recv(t); msg["tag"] != "deny" || msg["why"] != "busy" {
		t.Fatalf("second viewer = %v", msg)
	}
	c2.conn.Close()

	sawTitle, sawFrame := false, false
	// a malformed line and an unknown tag are dropped, not fatal
	if _, err := c.conn.Write([]byte("zoink\n")); err != nil {
		t.Fatal(err)
	}
	c.send(t, map[string]any{"tag": "mystery"})
	c.send(t, map[string]any{"tag": "key", "key": "up", "char": "", "mods": []string{}})
	c.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	for {
		msg := c.recv(t)
		switch msg["tag"] {
		case "title":
			sawTitle = msg["text"] == "served"
		case "frame":
			tree, ok := msg["tree"].(map[string]any)
			if !ok || tree["w"] != "text" {
				t.Fatalf("frame tree = %v", msg["tree"])
			}
			sawFrame = true
		case "quit":
			if !sawTitle || !sawFrame {
				t.Fatalf("missing traffic before quit: title=%v frame=%v", sawTitle, sawFrame)
			}
			select {
			case final := <-finals:
				if final != "42" {
					t.Fatalf("final = %s, want 42", final)
				}
			case err := <-errs:
				t.Fatalf("serve error: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("serve did not return")
			}
			return
		}
	}
}

func TestTuiServeDisconnectQuits(t *testing.T) {
	addr, errs, finals := startServe(t)
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	c.conn.Close() // the viewer vanishes → v1 disconnect-quits
	select {
	case final := <-finals:
		if final != "41" {
			t.Fatalf("final = %s, want the pre-disconnect state 41", final)
		}
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return on disconnect")
	}
}

func TestTuiServeDenials(t *testing.T) {
	addr, errs, _ := startServe(t)
	// wrong token
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "wrong", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["why"] != "bad-token" {
		t.Fatalf("bad token = %v", msg)
	}
	c.conn.Close()
	// wrong proto
	c = dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 9})
	if msg := c.recv(t); msg["why"] != "proto" {
		t.Fatalf("bad proto = %v", msg)
	}
	c.conn.Close()
	// garbage handshake
	c = dialWire(t, addr)
	if _, err := c.conn.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	if msg := c.recv(t); msg["why"] != "transport" {
		t.Fatalf("garbage handshake = %v", msg)
	}
	c.conn.Close()
	// a valid attach then quits the app so the goroutine ends
	c = dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("final handshake = %v", msg)
	}
	c.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	for {
		if msg := c.recv(t); msg["tag"] == "quit" {
			break
		}
	}
	c.conn.Close()
	select {
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	default:
	}
}

func TestTuiServeConfigAndPolicyArms(t *testing.T) {
	reg := tcReg(t)
	check := func(opts, app, want string) {
		t.Helper()
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "aql:tui"`,
			`Tui.serve ` + opts + ` ` + app})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("serve %s = %v, want %q", opts, err, want)
		}
	}
	okApp := `{update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`
	check(`{token: "x"}`, okApp, "tcp: must be an Integer port")
	check(`{tcp: -1  token: "x"}`, okApp, "tcp: must be an Integer port")
	check(`{tcp: 0}`, okApp, "token: is required")
	check(`{tcp: 0  token: 5}`, okApp, "token: must be a String")
	check(`{tcp: 0  token: "x"}`, `{}`, "missing update")
	_, dErr := tuiServeHandler([]native.Value{native.NewTypeLiteral(native.TMap), native.NewMap(native.NewOrderedMap())}, nil, nil, reg)
	if dErr == nil || !strings.Contains(dErr.Error(), "expected an options Map") {
		t.Fatalf("type-literal opts = %v", dErr)
	}

	// sandbox denies the network scope (checked first) — a direct
	// handler call, since sandbox refuses the module import itself
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	preg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	sopts := native.NewOrderedMap()
	sopts.Set("tcp", native.NewInteger(0))
	sopts.Set("token", native.NewString("x"))
	_, nErr := tuiServeHandler([]native.Value{native.NewMap(sopts), native.NewMap(native.NewOrderedMap())}, nil, nil, preg)
	var denied *policy.Denied
	if !errors.As(nErr, &denied) || denied.Scope != "network" {
		t.Fatalf("sandbox serve = %v", nErr)
	}

	// a network-allowed, terminal-denied profile reaches the terminal gate
	split, err := policy.LoadInline(`{version: 1  name: "split"  scopes: {
	    network:  { words: { default: "allow" } }
	    terminal: { install: false }
	  }}`)
	if err != nil {
		t.Fatal(err)
	}
	sreg, err := native.DefaultRegistryWithPolicy(split)
	if err != nil {
		t.Fatal(err)
	}
	InstallResolver(sreg)
	_, tErr := runTuiStepsOn(t, sreg, []string{`import "aql:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	if !errors.As(tErr, &denied) || denied.Scope != "terminal" {
		t.Fatalf("split-profile serve = %v", tErr)
	}

	// a squatted name rejects serve before it listens
	reg2 := tcReg(t)
	if reg2.Procs == nil {
		t.Fatal("no process runtime")
	}
	sq := newSquatter(t, reg2)
	defer sq.Close()
	_, aErr := runTuiStepsOn(t, reg2, []string{`import "aql:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	if aErr == nil || !strings.Contains(aErr.Error(), "already owns the terminal") {
		t.Fatalf("squatted serve = %v", aErr)
	}

	// a failing listener maps to the net error vocabulary
	oldListen := tuiListen
	t.Cleanup(func() { tuiListen = oldListen })
	tuiListen = func(string, string) (net.Listener, error) { return nil, errors.New("no sockets today") }
	_, lErr := runTuiStepsOn(t, tcReg(t), []string{`import "aql:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	if lErr == nil || !strings.Contains(lErr.Error(), "no sockets today") {
		t.Fatalf("listen failure = %v", lErr)
	}
	tuiListen = oldListen

	// a listener that closes before any viewer attaches
	closingListen := func() {
		old := tuiListen
		defer func() { tuiListen = old }()
		oldBound := tuiServeBound
		defer func() { tuiServeBound = oldBound }()
		tuiServeBound = func(a net.Addr) {}
		tuiListen = func(network, addr string) (net.Listener, error) {
			ln, err := net.Listen(network, addr)
			if err != nil {
				return nil, err
			}
			_ = ln.Close() // sabotage: accept fails immediately
			return ln, nil
		}
		_, cErr := runTuiStepsOn(t, tcReg(t), []string{`import "aql:tui"`,
			`Tui.serve {tcp: 0  token: "x"} ` + okApp})
		if cErr == nil || !strings.Contains(cErr.Error(), "listener closed before a viewer attached") {
			t.Fatalf("closed listener = %v", cErr)
		}
	}
	closingListen()
}

// newSquatter registers a placeholder process under the tui name.
func newSquatter(t *testing.T, reg *native.Registry) interface{ Close() } {
	t.Helper()
	sq := eng.NewProcess(reg.Procs, 4, eng.OverflowDrop)
	if err := reg.Procs.Insert(sq); err != nil {
		t.Fatal(err)
	}
	if err := reg.Procs.RegisterName(tuiRegisteredName, sq); err != nil {
		t.Fatal(err)
	}
	return sq
}

// tuiWirePaint frames carry an increasing seq and the projected tree;
// a failing write maps to the viewer-gone signal, not an app error.
func TestTuiWirePaint(t *testing.T) {
	var got []map[string]any
	paint := tuiWirePaint(func(payload any) error {
		m, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload = %T", payload)
		}
		got = append(got, m)
		return nil
	})
	if err := paint(native.NewString("x"), 4, 2); err != nil {
		t.Fatal(err)
	}
	if err := paint(native.NewString("y"), 4, 2); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["tag"] != "frame" || got[0]["seq"] != 1 ||
		got[1]["seq"] != 2 || got[1]["tree"] != "y" {
		t.Fatalf("payloads = %v", got)
	}
	bad := tuiWirePaint(func(any) error { return errors.New("gone") })
	if err := bad(native.NewString("x"), 4, 2); !errors.Is(err, errTuiViewerGone) {
		t.Fatalf("write failure = %v", err)
	}
}

// serve on a runtime-less registry mints one before listening (the
// listen seam then fails so the test stays listener-free).
func TestTuiServeLazyRuntime(t *testing.T) {
	reg := tcReg(t)
	if _, err := runTuiStepsOn(t, reg, []string{`import "aql:tui"`, serveApp}); err != nil {
		t.Fatal(err)
	}
	reg.Procs = nil
	oldListen := tuiListen
	t.Cleanup(func() { tuiListen = oldListen })
	tuiListen = func(string, string) (net.Listener, error) { return nil, errors.New("no sockets today") }
	_, sErr := runTuiStepsOn(t, reg, []string{`Tui.serve {tcp: 0  token: "x"} app`})
	if sErr == nil || !strings.Contains(sErr.Error(), "no sockets today") {
		t.Fatalf("lazy-runtime serve = %v", sErr)
	}
	if reg.Procs == nil {
		t.Fatal("serve did not create the process runtime")
	}
}

// a runtime shut down between the bind and the attach fails the
// process insert and releases the connection and listener.
func TestTuiServeInsertFailure(t *testing.T) {
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }
	reg := tcReg(t)
	rt := reg.Procs
	if rt == nil {
		t.Fatal("no process runtime")
	}
	errs := make(chan error, 1)
	go func() {
		_, sErr := runTuiStepsOn(t, reg, []string{`import "aql:tui"`, serveApp,
			`Tui.serve {tcp: 0  token: "s3cret"} app`})
		errs <- sErr
	}()
	var addr net.Addr
	select {
	case addr = <-bound:
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
	}
	rt.Shutdown()
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	select {
	case sErr := <-errs:
		if sErr == nil || !strings.Contains(sErr.Error(), "shut down") {
			t.Fatalf("insert failure = %v", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not fail")
	}
}

// an error raised inside the app's update propagates out of serve.
func TestTuiServeAppErrorPropagates(t *testing.T) {
	addr, errs, _ := startServe(t)
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	c.send(t, map[string]any{"tag": "key", "key": "boom", "char": "", "mods": []string{}})
	select {
	case sErr := <-errs:
		if sErr == nil || !strings.Contains(sErr.Error(), "kaboom") {
			t.Fatalf("app error = %v", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not propagate the app error")
	}
}

// tuiHandshake's transport arms and dimension defaulting, driven
// directly over a net.Pipe.
func TestTuiHandshakeDirect(t *testing.T) {
	// a hello with no dimensions defaults to 80×24
	client, server := net.Pipe()
	go func() {
		data, _ := json.Marshal(map[string]any{"tag": "attach", "token": "t", "proto": 1})
		_, _ = client.Write(append(data, '\n'))
		sc := bufio.NewScanner(client)
		_ = sc.Scan() // consume the accept reply
	}()
	cols, rows, why := tuiHandshake(server, "t")
	if why != "" || cols != 80 || rows != 24 {
		t.Fatalf("defaulted handshake = %dx%d %q", cols, rows, why)
	}
	_ = client.Close()
	_ = server.Close()

	// a viewer that hangs up before sending anything
	client, server = net.Pipe()
	_ = client.Close()
	if _, _, why := tuiHandshake(server, "t"); why != "transport" {
		t.Fatalf("silent close = %q", why)
	}
	_ = server.Close()

	// a viewer that vanishes before the accept reply lands
	client, server = net.Pipe()
	go func() {
		data, _ := json.Marshal(map[string]any{"tag": "attach", "token": "t", "proto": 1})
		_, _ = client.Write(append(data, '\n'))
		_ = client.Close()
	}()
	if _, _, why := tuiHandshake(server, "t"); why != "transport" {
		t.Fatalf("vanish before accept = %q", why)
	}
	_ = server.Close()
}
