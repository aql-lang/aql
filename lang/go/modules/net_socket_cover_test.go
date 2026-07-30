package modules

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// Unit coverage for the net_socket.go handler arms the end-to-end suite
// (net_test.go) cannot reach: type-mismatch guards, option-parsing
// rejections, the error-vocabulary mapping, deadline plumbing, and the
// policy-denial arms. Per the wave convention, package-level handlers
// are called directly with crafted native.Value args; real loopback /
// unix / in-memory conns are used only where a live net.Conn matters.

// ---- helpers ----

func nscReg(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// nscMap builds a concrete Map value from alternating key/value pairs.
func nscMap(pairs ...any) native.Value {
	om := native.NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		om.Set(pairs[i].(string), pairs[i+1].(native.Value))
	}
	return native.NewMap(om)
}

// nscTCPPair returns a connected loopback TCP pair (client, server),
// both closed at test cleanup.
func nscTCPPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, aErr := ln.Accept()
		ch <- res{c, aErr}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	srv := <-ch
	if srv.err != nil {
		t.Fatal(srv.err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = srv.c.Close()
	})
	return client, srv.c
}

// nscCode unwraps the AqlError code from a handler error.
func nscCode(t *testing.T, err error) string {
	t.Helper()
	var ae *eng.AqlError
	if !errors.As(err, &ae) {
		t.Fatalf("expected an AqlError, got %v", err)
	}
	return ae.Code
}

// nscErrContains asserts a non-nil error mentioning sub.
func nscErrContains(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("want error containing %q, got %v", sub, err)
	}
}

// nscZeroConn is a net.Conn whose Read returns (0, nil) — the io.Reader
// corner bufio forwards on the large-direct-read path — and whose
// addresses carry no host:port shape. It is not a *net.TCPConn, so it
// also drives the shutdown non-TCP arm and the peer SplitHostPort
// fallback.
type nscZeroConn struct{}

type nscAddr struct{}

func (nscAddr) Network() string { return "nsc" }
func (nscAddr) String() string  { return "nsc-addr-no-port" }

func (nscZeroConn) Read(p []byte) (int, error)       { return 0, nil }
func (nscZeroConn) Write(p []byte) (int, error)      { return len(p), nil }
func (nscZeroConn) Close() error                     { return nil }
func (nscZeroConn) LocalAddr() net.Addr              { return nscAddr{} }
func (nscZeroConn) RemoteAddr() net.Addr             { return nscAddr{} }
func (nscZeroConn) SetDeadline(time.Time) error      { return nil }
func (nscZeroConn) SetReadDeadline(time.Time) error  { return nil }
func (nscZeroConn) SetWriteDeadline(time.Time) error { return nil }

// nscNoDeadlineListener hides the concrete listener's SetDeadline (the
// embedded interface's method set has only Accept/Close/Addr), so
// accept {within: ms} must be refused.
type nscNoDeadlineListener struct{ net.Listener }

// nscBadDeadlineListener exposes SetDeadline but fails it.
type nscBadDeadlineListener struct{ net.Listener }

func (nscBadDeadlineListener) SetDeadline(time.Time) error {
	return errors.New("deadline rejected by transport")
}

// nscDeadlineIsErr matches os.ErrDeadlineExceeded via errors.Is without
// implementing net.Error — the only shape that reaches mapNetErr's
// default-arm deadline check.
type nscDeadlineIsErr struct{}

func (nscDeadlineIsErr) Error() string        { return "poll: deadline reached" }
func (nscDeadlineIsErr) Is(target error) bool { return target == os.ErrDeadlineExceeded }

// ---- type registration ----

func TestNSCRegisterNetTypeDuplicateRecorded(t *testing.T) {
	saved := native.SwapTypeInitErrs(nil)
	t.Cleanup(func() { native.SwapTypeInitErrs(saved) })

	// Re-registering the already-installed Ideal/Socket path must record
	// an init error (ADR-005: never panic) and return nil.
	if tt := registerNetType("Ideal/Socket", 5009, socketBehavior{}); tt != nil {
		t.Fatalf("duplicate registration must return nil, got %v", tt)
	}
	err := native.TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "net: register Ideal/Socket") {
		t.Fatalf("expected recorded net init error, got %v", err)
	}
}

// ---- handle unwrapping + close latches ----

func TestNSCAsSocketAsListenerMisses(t *testing.T) {
	if _, ok := asSocket(native.NewInteger(1)); ok {
		t.Error("asSocket must reject a non-extension value")
	}
	if _, ok := asListener(native.NewInteger(1)); ok {
		t.Error("asListener must reject a non-extension value")
	}
	// Cross-wired extension payloads must miss too.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	lnVal := newListenerValue(ln)
	sockVal := newSocketValue(nscZeroConn{})
	if _, ok := asSocket(lnVal); ok {
		t.Error("asSocket must reject a Listener payload")
	}
	if _, ok := asListener(sockVal); ok {
		t.Error("asListener must reject a Socket payload")
	}
}

func TestNSCListenerDoubleCloseIsNoOp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	nl := &netListener{id: "LS_test", ln: ln}
	if cErr := nl.close(); cErr != nil {
		t.Fatalf("first close: %v", cErr)
	}
	if cErr := nl.close(); cErr != nil {
		t.Fatalf("second close must be a latched no-op, got %v", cErr)
	}
}

// ---- behaviors: Equal / Format ----

func TestNSCSocketBehaviorEqualFormat(t *testing.T) {
	a := newSocketValue(nscZeroConn{})
	b := newSocketValue(nscZeroConn{})
	beh := socketBehavior{}
	if !beh.Equal(a, a) {
		t.Error("a socket must equal itself")
	}
	if beh.Equal(a, b) {
		t.Error("distinct sockets must not be equal")
	}
	if beh.Equal(a, native.NewInteger(1)) {
		t.Error("socket vs integer must fall back to DefaultBehavior (false)")
	}
	open := beh.Format(a)
	if !strings.HasPrefix(open, "Socket<") || strings.Contains(open, "closed") {
		t.Errorf("open socket format = %q", open)
	}
	sc, _ := asSocket(a)
	_ = sc.close()
	if closed := beh.Format(a); !strings.Contains(closed, " closed") {
		t.Errorf("closed socket format = %q, want ' closed' marker", closed)
	}
	if got := beh.Format(native.NewInteger(1)); got != "Socket" {
		t.Errorf("non-socket format = %q, want %q", got, "Socket")
	}
}

func TestNSCListenerBehaviorEqualFormat(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	lv := newListenerValue(ln)
	beh := listenerBehavior{}
	if !beh.Equal(lv, lv) {
		t.Error("a listener must equal itself")
	}
	if beh.Equal(lv, native.NewInteger(1)) {
		t.Error("listener vs integer must fall back to DefaultBehavior (false)")
	}
	if got := beh.Format(lv); !strings.HasPrefix(got, "Listener<") {
		t.Errorf("listener format = %q", got)
	}
	if got := beh.Format(native.NewInteger(1)); got != "Listener" {
		t.Errorf("non-listener format = %q, want %q", got, "Listener")
	}
}

// ---- policy ----

func TestNSCCheckNetPolicyNilRegistry(t *testing.T) {
	if err := checkNetPolicy(nil, "listen", "listen", "", 1); err != nil {
		t.Fatalf("nil registry must skip the policy check, got %v", err)
	}
}

func TestNSCListenConnectPolicyDeniedArms(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	_, lErr := listenHandler([]native.Value{nscMap("tcp", native.NewInteger(0))}, nil, nil, reg)
	nscErrContains(t, lErr, "network")
	_, cErr := connectRawHandler([]native.Value{nscMap("tcp", native.NewString("127.0.0.1:9"))}, nil, nil, reg)
	nscErrContains(t, cErr, "network")
}

// ---- parseNetAddr ----

func TestNSCParseNetAddrArms(t *testing.T) {
	reg := nscReg(t)

	// Non-map options are refused by the concrete-map guard.
	if _, err := parseNetAddr(reg, native.NewInteger(1), "listen", true); err == nil {
		t.Error("non-map options must be rejected")
	}

	// unix: must be a non-empty String path.
	for _, bad := range []native.Value{native.NewInteger(7), native.NewString("")} {
		_, err := parseNetAddr(reg, nscMap("unix", bad), "listen", true)
		nscErrContains(t, err, "unix: must be a socket path")
	}

	// unix: happy path — network/addr/host all derive from the path.
	opts, err := parseNetAddr(reg, nscMap("unix", native.NewString("/tmp/nsc.sock")), "listen", true)
	if err != nil {
		t.Fatalf("unix parse: %v", err)
	}
	if opts.network != "unix" || opts.addr != "/tmp/nsc.sock" || opts.host != "unix:/tmp/nsc.sock" {
		t.Errorf("unix opts = %+v", opts)
	}

	// listen host: option rides into the bind address and policy host.
	opts, err = parseNetAddr(reg,
		nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1")),
		"listen", true)
	if err != nil {
		t.Fatalf("host option parse: %v", err)
	}
	if opts.host != "127.0.0.1" || opts.addr != "127.0.0.1:0" {
		t.Errorf("host option opts = %+v", opts)
	}

	// A dial string without :port keeps the whole string as policy host.
	opts, err = parseNetAddr(reg, nscMap("tcp", native.NewString("just-a-host")), "connect-raw", false)
	if err != nil {
		t.Fatalf("portless dial parse: %v", err)
	}
	if opts.host != "just-a-host" || opts.port != 0 {
		t.Errorf("portless dial opts = %+v", opts)
	}
}

// ---- recvDeadline ----

func TestNSCRecvDeadlineArms(t *testing.T) {
	// A non-map trailing arg means "no deadline".
	if _, has, err := recvDeadline([]native.Value{native.NewInteger(5)}, 0); has || err != nil {
		t.Errorf("non-map options arg: has=%v err=%v", has, err)
	}
	// within: must be a non-negative Integer.
	for _, bad := range []native.Value{native.NewString("x"), native.NewInteger(-1)} {
		if _, _, err := recvDeadline([]native.Value{nscMap("within", bad)}, 0); err == nil {
			t.Errorf("within %v must be rejected", bad)
		}
	}
}

// ---- mapNetErr vocabulary ----

func TestNSCMapNetErrVocabulary(t *testing.T) {
	reg := nscReg(t)
	cases := []struct {
		err  error
		code string
	}{
		{nscDeadlineIsErr{}, "timeout"},
		{errors.New("write tcp: broken pipe"), "closed"},
		{errors.New("read tcp: connection reset by peer"), "closed"},
		{errors.New("mystery transport failure"), "transport"},
	}
	for _, c := range cases {
		if got := nscCode(t, mapNetErr(reg, "recv", c.err)); got != c.code {
			t.Errorf("mapNetErr(%v) code = %q, want %q", c.err, got, c.code)
		}
	}
}

// ---- listen ----

func TestNSCListenTransportError(t *testing.T) {
	reg := nscReg(t)
	// Binding a unix socket inside a directory that does not exist fails
	// in net.Listen — the transport arm.
	path := filepath.Join(t.TempDir(), "missing", "x.sock")
	_, err := listenHandler([]native.Value{nscMap("unix", native.NewString(path))}, nil, nil, reg)
	if got := nscCode(t, err); got != "transport" {
		t.Errorf("listen bind failure code = %q, want transport (%v)", got, err)
	}
}

// ---- accept ----

func TestNSCAcceptArms(t *testing.T) {
	reg := nscReg(t)

	// Not a Listener.
	_, err := acceptHandler([]native.Value{newSocketValue(nscZeroConn{})}, nil, nil, reg)
	nscErrContains(t, err, "expected a Listener")

	ln, lErr := net.Listen("tcp", "127.0.0.1:0")
	if lErr != nil {
		t.Fatal(lErr)
	}
	t.Cleanup(func() { _ = ln.Close() })
	within := nscMap("within", native.NewInteger(50))

	// Malformed within:.
	_, err = acceptHandler([]native.Value{newListenerValue(ln), nscMap("within", native.NewInteger(-1))}, nil, nil, reg)
	nscErrContains(t, err, "within: must be a non-negative Integer")

	// A listener without SetDeadline cannot honour {within}.
	_, err = acceptHandler([]native.Value{newListenerValue(nscNoDeadlineListener{ln}), within}, nil, nil, reg)
	nscErrContains(t, err, "does not support {within: ms}")

	// A listener whose SetDeadline fails surfaces via mapNetErr.
	_, err = acceptHandler([]native.Value{newListenerValue(nscBadDeadlineListener{ln}), within}, nil, nil, reg)
	if got := nscCode(t, err); got != "transport" {
		t.Errorf("SetDeadline failure code = %q, want transport (%v)", got, err)
	}
}

// ---- connect-raw ----

func TestNSCConnectRawArms(t *testing.T) {
	reg := nscReg(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	addr := ln.Addr().String()

	// Malformed within: riding the dial options map.
	_, cErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "within", native.NewInteger(-1)),
	}, nil, nil, reg)
	nscErrContains(t, cErr, "within: must be a non-negative Integer")

	// A valid within: becomes the dial timeout on the success path.
	out, cErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "within", native.NewInteger(2000)),
	}, nil, nil, reg)
	if cErr != nil {
		t.Fatalf("dial with within: %v", cErr)
	}
	sc, ok := asSocket(out[0])
	if !ok {
		t.Fatal("connect-raw must return a Socket")
	}
	_ = sc.close()
}

// ---- serve-raw ----

func TestNSCServeRawHandlerNotFn(t *testing.T) {
	reg := nscReg(t)
	_, err := serveRawHandler([]native.Value{
		nscMap("tcp", native.NewInteger(0)), native.NewInteger(42),
	}, nil, nil, reg)
	nscErrContains(t, err, "handler must be a function")
}

func TestNSCServeRawListenErrorPropagates(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "aql:net"`,
		`Net.serve-raw {} ([conn:Any] => [conn])`,
	})
	nscErrContains(t, err, "options need tcp:")
}

// A handler whose signature does not accept a Socket is reported and
// the connection dropped — the client observes `closed`.
func TestNSCServeRawHandlerRejectsSocket(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "aql:net"`,
		`def ln (Net.serve-raw {tcp: 0} ([a:Integer] => [a]))`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.recv sock 10 {within: 5000}`,
	})
	nscErrContains(t, err, "closed")
}

// A handler that fails with a non-`closed` error (here: recv timeout on
// a silent client) is logged and its connection closed — the client
// observes `closed`.
func TestNSCServeRawHandlerErrorLogged(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "aql:net"`,
		`def nl (convert Bytes "\n")`,
		`def ln (Net.serve-raw {tcp: 0} ([conn:Any] => [
		   Net.recv-until conn nl {within: 60}
		 ]))`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.recv sock 10 {within: 5000}`,
	})
	nscErrContains(t, err, "closed")
}

// ---- recv ----

func TestNSCRecvArgArms(t *testing.T) {
	reg := nscReg(t)
	sock := newSocketValue(nscZeroConn{})

	_, err := recvHandler([]native.Value{native.NewInteger(1), native.NewInteger(1)}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	_, err = recvHandler([]native.Value{sock, native.NewInteger(-1)}, nil, nil, reg)
	nscErrContains(t, err, "non-negative Integer")

	_, err = recvHandler([]native.Value{sock, native.NewInteger(1), nscMap("within", native.NewInteger(-1))}, nil, nil, reg)
	nscErrContains(t, err, "within: must be a non-negative Integer")
}

func TestNSCRecvDeadlineSetFailsOnClosedSocket(t *testing.T) {
	reg := nscReg(t)
	client, _ := nscTCPPair(t)
	sock := newSocketValue(client)
	sc, _ := asSocket(sock)
	_ = sc.close()
	_, err := recvHandler([]native.Value{sock, native.NewInteger(1), nscMap("within", native.NewInteger(10))}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("recv on closed socket code = %q, want closed (%v)", got, err)
	}
}

func TestNSCRecvZeroCountReadsAvailable(t *testing.T) {
	reg := nscReg(t)
	client, server := nscTCPPair(t)
	if _, err := server.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	out, err := recvHandler([]native.Value{newSocketValue(client), native.NewInteger(0)}, nil, nil, reg)
	if err != nil {
		t.Fatalf("recv 0: %v", err)
	}
	got, _ := native.AsBytesValue(out[0])
	if string(got) != "abc" {
		t.Errorf("recv 0 = %q, want %q", got, "abc")
	}
}

func TestNSCRecvZeroByteReadYieldsEmptyBytes(t *testing.T) {
	reg := nscReg(t)
	// bufio's large-direct-read path forwards the underlying (0, nil) —
	// recv maps it to an empty Bytes rather than an error.
	out, err := recvHandler([]native.Value{newSocketValue(nscZeroConn{}), native.NewInteger(4096)}, nil, nil, reg)
	if err != nil {
		t.Fatalf("recv over zero-reader: %v", err)
	}
	if got, ok := native.AsBytesValue(out[0]); !ok || len(got) != 0 {
		t.Errorf("recv over zero-reader = %v (%v), want empty Bytes", got, ok)
	}
}

// ---- recv-bytes ----

func TestNSCRecvBytesArgArms(t *testing.T) {
	reg := nscReg(t)
	sock := newSocketValue(nscZeroConn{})

	_, err := recvBytesHandler([]native.Value{native.NewInteger(1), native.NewInteger(1)}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	_, err = recvBytesHandler([]native.Value{sock, native.NewInteger(-1)}, nil, nil, reg)
	nscErrContains(t, err, "non-negative Integer")

	_, err = recvBytesHandler([]native.Value{sock, native.NewInteger(1), nscMap("within", native.NewInteger(-1))}, nil, nil, reg)
	nscErrContains(t, err, "within: must be a non-negative Integer")
}

func TestNSCRecvBytesClosedArms(t *testing.T) {
	reg := nscReg(t)

	// Deadline set fails on an already-closed socket.
	client, _ := nscTCPPair(t)
	sock := newSocketValue(client)
	sc, _ := asSocket(sock)
	_ = sc.close()
	_, err := recvBytesHandler([]native.Value{sock, native.NewInteger(1)}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("recv-bytes on closed socket code = %q (%v)", got, err)
	}

	// A clean EOF (peer closed, zero bytes) maps through mapNetErr.
	client2, server2 := nscTCPPair(t)
	_ = server2.Close()
	_, err = recvBytesHandler([]native.Value{newSocketValue(client2), native.NewInteger(3)}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("recv-bytes after peer close code = %q (%v)", got, err)
	}

	// A PARTIAL frame then EOF (peer wrote fewer bytes than asked, then
	// closed) is the mid-frame arm: ReadFull's io.ErrUnexpectedEOF, not a
	// clean EOF. TCP orders the data before the FIN, so the read
	// deterministically sees the 2 bytes and then the close.
	client3, server3 := nscTCPPair(t)
	if _, wErr := server3.Write([]byte("ab")); wErr != nil {
		t.Fatal(wErr)
	}
	_ = server3.Close()
	_, err = recvBytesHandler([]native.Value{newSocketValue(client3), native.NewInteger(5)}, nil, nil, reg)
	nscErrContains(t, err, "connection closed mid-frame")
}

// ---- recv-until ----

func TestNSCRecvUntilArgArms(t *testing.T) {
	reg := nscReg(t)
	sock := newSocketValue(nscZeroConn{})
	nl := native.NewBytesValue([]byte("\n"))

	_, err := recvUntilHandler([]native.Value{native.NewInteger(1), nl}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	for _, bad := range []native.Value{native.NewInteger(1), native.NewBytesValue(nil)} {
		_, err = recvUntilHandler([]native.Value{sock, bad}, nil, nil, reg)
		nscErrContains(t, err, "delimiter must be a non-empty Bytes")
	}

	_, err = recvUntilHandler([]native.Value{sock, nl, nscMap("within", native.NewInteger(-1))}, nil, nil, reg)
	nscErrContains(t, err, "within: must be a non-negative Integer")
}

func TestNSCRecvUntilClosedArms(t *testing.T) {
	reg := nscReg(t)
	nl := native.NewBytesValue([]byte("\n"))

	// Deadline set fails on an already-closed socket.
	client, _ := nscTCPPair(t)
	sock := newSocketValue(client)
	sc, _ := asSocket(sock)
	_ = sc.close()
	_, err := recvUntilHandler([]native.Value{sock, nl}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("recv-until on closed socket code = %q (%v)", got, err)
	}

	// Peer closing mid-line (partial data, no delimiter) raises closed
	// with the broken-frame message.
	client2, server2 := nscTCPPair(t)
	if _, wErr := server2.Write([]byte("abc")); wErr != nil {
		t.Fatal(wErr)
	}
	_ = server2.Close()
	_, err = recvUntilHandler([]native.Value{newSocketValue(client2), nl}, nil, nil, reg)
	nscErrContains(t, err, "closed before delimiter")
}

// ---- send-bytes ----

func TestNSCSendBytesArms(t *testing.T) {
	reg := nscReg(t)
	data := native.NewBytesValue([]byte("x"))

	_, err := sendBytesHandler([]native.Value{native.NewInteger(1), newSocketValue(nscZeroConn{})}, nil, nil, reg)
	nscErrContains(t, err, "expected Bytes")

	_, err = sendBytesHandler([]native.Value{data, native.NewInteger(1)}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	// Writing to a closed socket maps through mapNetErr.
	client, _ := nscTCPPair(t)
	sock := newSocketValue(client)
	sc, _ := asSocket(sock)
	_ = sc.close()
	_, err = sendBytesHandler([]native.Value{data, sock}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("send-bytes on closed socket code = %q (%v)", got, err)
	}
}

// ---- shutdown ----

func TestNSCShutdownArms(t *testing.T) {
	reg := nscReg(t)

	_, err := shutdownHandler([]native.Value{native.NewInteger(1), native.NewString("read")}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	// Half-close needs a TCP socket.
	_, err = shutdownHandler([]native.Value{newSocketValue(nscZeroConn{}), native.NewString("read")}, nil, nil, reg)
	nscErrContains(t, err, "only supported on TCP")

	for _, half := range []string{"read", "write"} {
		client, _ := nscTCPPair(t)
		if _, hErr := shutdownHandler([]native.Value{newSocketValue(client), native.NewString(half)}, nil, nil, reg); hErr != nil {
			t.Errorf("shutdown %q: %v", half, hErr)
		}
	}

	// "both" closes; a later half-close on the closed conn errors and
	// maps through mapNetErr.
	client, _ := nscTCPPair(t)
	sock := newSocketValue(client)
	if _, hErr := shutdownHandler([]native.Value{sock, native.NewString("both")}, nil, nil, reg); hErr != nil {
		t.Fatalf("shutdown both: %v", hErr)
	}
	_, err = shutdownHandler([]native.Value{sock, native.NewString("read")}, nil, nil, reg)
	if got := nscCode(t, err); got != "closed" {
		t.Errorf("shutdown after close code = %q (%v)", got, err)
	}

	// Unknown half.
	client2, _ := nscTCPPair(t)
	_, err = shutdownHandler([]native.Value{newSocketValue(client2), native.NewString("sideways")}, nil, nil, reg)
	nscErrContains(t, err, `half must be "read", "write" or "both"`)
}

// ---- close ----

func TestNSCCloseArms(t *testing.T) {
	reg := nscReg(t)

	// A Service that is not a network endpoint (nil closer) is refused.
	svc := native.NewRemoteServiceValue(nil, nil)
	_, err := closeHandler([]native.Value{svc}, nil, nil, reg)
	nscErrContains(t, err, "not a network endpoint")

	// Anything else falls through to the catch-all rejection.
	_, err = closeHandler([]native.Value{native.NewInteger(1)}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket, Listener or Endpoint")
}

// ---- addr / peer ----

func TestNSCAddrArms(t *testing.T) {
	reg := nscReg(t)

	_, err := addrHandler([]native.Value{newSocketValue(nscZeroConn{})}, nil, nil, reg)
	nscErrContains(t, err, "expected a Listener")

	// A unix listener's address has no host:port shape — the whole
	// string lands in host with port 0.
	path := filepath.Join(t.TempDir(), "s.sock")
	ln, lErr := net.Listen("unix", path)
	if lErr != nil {
		t.Fatal(lErr)
	}
	t.Cleanup(func() { _ = ln.Close() })
	out, aErr := addrHandler([]native.Value{newListenerValue(ln)}, nil, nil, reg)
	if aErr != nil {
		t.Fatalf("addr on unix listener: %v", aErr)
	}
	mp, _ := native.AsMap(out[0])
	hv, _ := mp.Get("host")
	host, _ := hv.AsConcreteString()
	pv, _ := mp.Get("port")
	port, _ := pv.AsConcreteInteger()
	if host != path || port != 0 {
		t.Errorf("unix addr = {host: %q port: %d}, want {host: %q port: 0}", host, port, path)
	}
}

func TestNSCPeerArms(t *testing.T) {
	reg := nscReg(t)

	ln, lErr := net.Listen("tcp", "127.0.0.1:0")
	if lErr != nil {
		t.Fatal(lErr)
	}
	t.Cleanup(func() { _ = ln.Close() })
	_, err := peerHandler([]native.Value{newListenerValue(ln)}, nil, nil, reg)
	nscErrContains(t, err, "expected a Socket")

	// A remote address without host:port shape lands whole in host.
	out, pErr := peerHandler([]native.Value{newSocketValue(nscZeroConn{})}, nil, nil, reg)
	if pErr != nil {
		t.Fatalf("peer on portless conn: %v", pErr)
	}
	mp, _ := native.AsMap(out[0])
	hv, _ := mp.Get("host")
	host, _ := hv.AsConcreteString()
	pv, _ := mp.Get("port")
	port, _ := pv.AsConcreteInteger()
	if host != "nsc-addr-no-port" || port != 0 {
		t.Errorf("portless peer = {host: %q port: %d}", host, port)
	}

	// The positive contract: a TCP peer splits into host + real port.
	client, server := nscTCPPair(t)
	out, pErr = peerHandler([]native.Value{newSocketValue(client)}, nil, nil, reg)
	if pErr != nil {
		t.Fatalf("peer on tcp conn: %v", pErr)
	}
	mp, _ = native.AsMap(out[0])
	hv, _ = mp.Get("host")
	host, _ = hv.AsConcreteString()
	pv, _ = mp.Get("port")
	port, _ = pv.AsConcreteInteger()
	wantHost, wantPortStr, sErr := net.SplitHostPort(server.LocalAddr().String())
	if sErr != nil {
		t.Fatal(sErr)
	}
	if host != wantHost || strconv.FormatInt(port, 10) != wantPortStr {
		t.Errorf("tcp peer = {host: %q port: %d}, want {host: %q port: %s}",
			host, port, wantHost, wantPortStr)
	}
}

// netNoReturns is the check-mode ReturnsFn of the statement-form words:
// always the empty return shape.
func TestNSCNetNoReturns(t *testing.T) {
	if got := netNoReturns(nil, nil); got != nil {
		t.Errorf("netNoReturns = %v, want nil", got)
	}
}
