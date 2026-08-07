package modules

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	parser "github.com/boru-lang/boru/parser/go"
)

// ---- helpers ----

// mintServerCert issues a self-signed server certificate carrying a
// 127.0.0.1 IP SAN, so a dial verifies by address with no sni: override.
// Returned as a registered identity (what `tls: {identity:}` names) plus
// the CA PEM a client trusts it by.
func mintServerCert(t *testing.T) (id capabilities.ClientIdentity, caPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{CommonName: "boru-listen-server"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
			return cert, nil
		}),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// tlsListenReg returns a registry with "srv" registered as the server
// credential, plus the CA a client must trust to reach it.
func tlsListenReg(t *testing.T) (r *native.Registry, caPEM string) {
	t.Helper()
	r = nscReg(t)
	id, ca := mintServerCert(t)
	native.RegisterClientIdentity(r, "srv", id)
	return r, ca
}

// listenTLS binds a TLS listener on an ephemeral port and returns the
// Listener value plus its "host:port".
func listenTLS(t *testing.T, r *native.Registry, tlsOpts native.Value) (native.Value, string) {
	t.Helper()
	out, err := listenHandler([]native.Value{
		nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1"),
			"tls", tlsOpts),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("listen with tls: %v", err)
	}
	nl, ok := asListener(out[0])
	if !ok {
		t.Fatalf("expected a Listener, got %v", out[0])
	}
	t.Cleanup(func() { _ = nl.close() })
	return out[0], nl.ln.Addr().String()
}

// ---- server TLS ----

// The whole point of the phase, end to end: a boru-bound listener
// terminates TLS, and the Socket that comes out of `accept` is the same
// Socket every other word already drives.
func TestListenTLSRoundTrip(t *testing.T) {
	r, caPEM := tlsListenReg(t)
	lv, addr := listenTLS(t, r, nscMap("identity", native.NewAtom("srv")))

	type accepted struct {
		sock native.Value
		err  error
	}
	done := make(chan accepted, 1)
	go func() {
		out, aErr := acceptHandler([]native.Value{lv}, nil, nil, r)
		if aErr != nil {
			done <- accepted{err: aErr}
			return
		}
		done <- accepted{sock: out[0]}
	}()

	cli, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("ca", native.NewString(caPEM))),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("dial the boru-bound TLS listener: %v", err)
	}
	got := <-done
	if got.err != nil {
		t.Fatalf("accept: %v", got.err)
	}

	if _, sErr := sendBytesHandler([]native.Value{
		native.NewBytesValue([]byte("hello\n")), cli[0],
	}, nil, nil, r); sErr != nil {
		t.Fatalf("send: %v", sErr)
	}
	echo, rErr := recvUntilHandler([]native.Value{
		got.sock, native.NewBytesValue([]byte("\n")),
	}, nil, nil, r)
	if rErr != nil {
		t.Fatalf("recv on the server Socket: %v", rErr)
	}
	if b, _ := native.AsBytesValue(echo[0]); string(b) != "hello" {
		t.Errorf("server read %q, want \"hello\"", b)
	}

	// No client certificate was demanded, so there is no verified peer
	// identity to report — None, not a half-trusted map.
	pc, pErr := peerCertHandler([]native.Value{got.sock}, nil, nil, r)
	if pErr != nil {
		t.Fatalf("peer-cert: %v", pErr)
	}
	if !pc[0].Is(native.TNone) {
		t.Errorf("peer-cert = %v, want None when no client cert was required", pc[0])
	}

	_, _ = closeHandler([]native.Value{cli[0]}, nil, nil, r)
	_, _ = closeHandler([]native.Value{got.sock}, nil, nil, r)
}

// Mutual TLS: require-client: makes a client certificate mandatory, the
// verified chain reaches boru through peer-cert, and a client without one
// is refused. The negative half is what proves the requirement is real.
func TestListenTLSRequireClient(t *testing.T) {
	r, caPEM := tlsListenReg(t)
	clientCert, clientKey, clientCAs := mintTLSClientCert(t)
	_ = clientCAs
	clientCAPEM := string(clientCert)

	clientID, err := capabilities.StaticIdentity(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	native.RegisterClientIdentity(r, "cli", clientID)

	lv, addr := listenTLS(t, r, nscMap(
		"identity", native.NewAtom("srv"),
		"require-client", native.NewString(clientCAPEM),
	))

	dial := func(tlsOpts native.Value) (native.Value, error) {
		out, dErr := connectRawHandler([]native.Value{
			nscMap("tcp", native.NewString(addr), "tls", tlsOpts),
		}, nil, nil, r)
		if dErr != nil {
			return native.NewNone(), dErr
		}
		return out[0], nil
	}

	// With a client identity: accepted, and the server can read who it is.
	type accepted struct {
		sock native.Value
		err  error
	}
	done := make(chan accepted, 1)
	go func() {
		out, aErr := acceptHandler([]native.Value{lv}, nil, nil, r)
		if aErr != nil {
			done <- accepted{err: aErr}
			return
		}
		done <- accepted{sock: out[0]}
	}()
	cli, err := dial(nscMap(
		"ca", native.NewString(caPEM),
		"identity", native.NewAtom("cli"),
	))
	if err != nil {
		t.Fatalf("mutual-TLS dial: %v", err)
	}
	srv := <-done
	if srv.err != nil {
		t.Fatalf("accept a verified client: %v", srv.err)
	}

	pc, pErr := peerCertHandler([]native.Value{srv.sock}, nil, nil, r)
	if pErr != nil {
		t.Fatalf("peer-cert: %v", pErr)
	}
	mp, mErr := native.AsMap(pc[0])
	if mErr != nil {
		t.Fatalf("peer-cert must be a Map for a verified client: %v (%v)", pc[0], mErr)
	}
	cn, _ := mp.Get("common-name")
	if s, _ := cn.AsConcreteString(); s != "boru-vault-client" {
		t.Errorf("peer-cert common-name = %q, want the client's CN", s)
	}
	for _, key := range []string{"subject", "issuer", "serial", "not-before", "not-after"} {
		if _, ok := mp.Get(key); !ok {
			t.Errorf("peer-cert is missing %q", key)
		}
	}
	// The SANs an authorization rule would actually match on.
	dns, _ := mp.Get("dns-names")
	if l, lErr := native.AsList(dns); lErr != nil || l.Len() != 1 {
		t.Errorf("peer-cert dns-names = %v", dns)
	}
	emails, _ := mp.Get("emails")
	if l, lErr := native.AsList(emails); lErr != nil || l.Len() != 1 {
		t.Errorf("peer-cert emails = %v", emails)
	}
	_, _ = closeHandler([]native.Value{cli}, nil, nil, r)
	_, _ = closeHandler([]native.Value{srv.sock}, nil, nil, r)

	// Without one: the handshake fails. The rejection may surface on
	// either side depending on which flight completes first, so accept
	// in the background and require that at least one end reports it —
	// and that no usable server Socket comes out.
	failed := make(chan error, 1)
	go func() {
		_, aErr := acceptHandler([]native.Value{lv}, nil, nil, r)
		failed <- aErr
	}()
	anon, dErr := dial(nscMap("ca", native.NewString(caPEM)))
	aErr := <-failed
	if dErr == nil && aErr == nil {
		t.Error("a client with no certificate must not complete a require-client handshake")
	}
	if dErr == nil {
		_, _ = closeHandler([]native.Value{anon}, nil, nil, r)
	}
}

// serve-raw inherits server TLS from listen and completes the handshake
// on the per-connection goroutine, so the handler never sees an
// unauthenticated peer and a stalled client cannot pin the acceptor.
// Driven through real boru because that is the surface that has to work:
// the whole option lives in guest source.
func TestServeRawTLS(t *testing.T) {
	r, caPEM := tlsListenReg(t)
	r.ParseFunc = parser.Parse
	InstallResolver(r)
	// The CA is a host value here for the same reason a credential is:
	// PEM in source is noise, and binding it proves `ca:` accepts a
	// computed String, not only a literal.
	r.Defs.Push("capem", native.NewString(caPEM))

	out, err := runNetStepsOn(t, r, []string{
		`import "boru:net"`,
		`def nl (convert Bytes "\n")`,
		`def l (Net.serve-raw {tcp: 0  host: "127.0.0.1"  tls: {identity: srv/q}} ([c:Any] => [
		   Net.send-bytes (Net.recv-until c nl {within: 5000}) c
		 ]))`,
		`def a (Net.addr l)`,
		`def s (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String a.port)])  tls: {ca: capem}})`,
		`Net.send-bytes (convert Bytes "over-tls\n") s`,
		`def got (convert String (Net.recv-bytes s 8 {within: 5000}))`,
		`Net.close s; Net.close l; got`,
	})
	if err != nil {
		t.Fatalf("serve-raw over TLS: %v", err)
	}
	if s, _ := out[len(out)-1].AsConcreteString(); s != "over-tls" {
		t.Errorf("echoed %q, want \"over-tls\"", s)
	}
}

// runNetStepsOn runs boru steps on a caller-supplied registry, so a test
// can register a host credential first.
func runNetStepsOn(t *testing.T, reg *native.Registry, steps []string) ([]native.Value, error) {
	t.Helper()
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		var err error
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ---- option and policy rejections ----

// listen rejects the dial-side keys BY NAME rather than as generic
// unknowns: a `verify: false` silently accepted on a listener would read
// as configured when it configures nothing.
func TestListenTLSOptionRejections(t *testing.T) {
	r, _ := tlsListenReg(t)

	cases := []struct {
		opts native.Value
		want string
	}{
		{nscMap("identity", native.NewAtom("srv"), "verify", native.NewBoolean(false)),
			"verify: is not available here"},
		{nscMap("identity", native.NewAtom("srv"), "ca", native.NewString("x")),
			"ca: is not available here"},
		{nscMap("identity", native.NewAtom("srv"), "sni", native.NewString("x")),
			"sni: is not available here"},
		{nscMap("identity", native.NewAtom("srv"), "nope", native.NewBoolean(true)),
			`unknown option "nope"`},
		{nscMap(), "needs identity:"},
		{nscMap("identity", native.NewInteger(1)), "identity: must be an Atom"},
		{nscMap("identity", native.NewAtom("srv"), "require-client", native.NewInteger(1)),
			"require-client: must be Bytes"},
		{nscMap("identity", native.NewAtom("srv"), "min", native.NewString("1.1")),
			`min: must be "1.2" or "1.3"`},
		{nscMap("identity", native.NewAtom("srv"), "min", native.NewInteger(12)),
			`min: must be "1.2" or "1.3"`},
		{nscMap("identity", native.NewAtom("nobody")),
			`no client identity named "nobody"`},
		{nscMap("identity", native.NewAtom("srv"), "require-client", native.NewString("not pem")),
			"require-client: contains no usable certificate"},
	}
	for _, c := range cases {
		_, err := listenHandler([]native.Value{
			nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1"),
				"tls", c.opts),
		}, nil, nil, r)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("listen %v: got %v, want an error containing %q", c.opts, err, c.want)
		}
	}

	// `tls:` must be a Map at all — an Integer there is a shape error,
	// not an unknown option.
	if _, err := listenHandler([]native.Value{
		nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1"),
			"tls", native.NewInteger(1)),
	}, nil, nil, r); err == nil {
		t.Error("a non-Map tls: must be rejected")
	}

	// min: is shared with the dial side and does take effect.
	_, addr := listenTLS(t, r, nscMap(
		"identity", native.NewAtom("srv"), "min", native.NewString("1.3")))
	if addr == "" {
		t.Error("a valid min: must bind")
	}
}

// Presenting a SERVER certificate is its own authority: a profile may
// allow outbound client certs and still refuse to let a program answer
// AS a service. The op is server-cert, not client-cert.
func TestListenTLSPolicyDenied(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "net-no-server-cert"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["listen", "connect", "client-cert"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r, rErr := native.DefaultRegistryWithPolicy(pol)
	if rErr != nil {
		t.Fatal(rErr)
	}
	id, _ := mintServerCert(t)
	native.RegisterClientIdentity(r, "srv", id)

	_, dErr := listenHandler([]native.Value{
		nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1"),
			"tls", nscMap("identity", native.NewAtom("srv"))),
	}, nil, nil, r)
	if dErr == nil {
		t.Fatal("expected server-cert to be denied")
	}
	d, ok := dErr.(*policy.Denied)
	if !ok || d.Op != "server-cert" {
		t.Fatalf("got %v, want a server-cert denial", dErr)
	}
	if d.Args["identity"] != "srv" {
		t.Errorf("the denial must name the identity, got %v", d.Args)
	}

	// Plain listen (no tls:) on the same profile still works — the gate
	// is on the credential, not on binding.
	if _, pErr := listenHandler([]native.Value{
		nscMap("tcp", native.NewInteger(0), "host", native.NewString("127.0.0.1")),
	}, nil, nil, r); pErr != nil {
		t.Errorf("plain listen must stay allowed: %v", pErr)
	}
}

// peer-cert on a plain (non-TLS) Socket is None, and on a non-Socket an
// error — never a panic, never a fabricated identity.
func TestPeerCertNonTLS(t *testing.T) {
	r := nscReg(t)
	cli, _ := nscTCPPair(t)
	sock := newSocketValue(cli)
	out, err := peerCertHandler([]native.Value{sock}, nil, nil, r)
	if err != nil {
		t.Fatalf("peer-cert on a plain socket: %v", err)
	}
	if !out[0].Is(native.TNone) {
		t.Errorf("peer-cert = %v, want None on a plain socket", out[0])
	}
	if _, bad := peerCertHandler([]native.Value{native.NewInteger(1)}, nil, nil, r); bad == nil {
		t.Error("peer-cert must reject a non-Socket")
	}
}

// An accept deadline bounds the HANDSHAKE too, then is cleared so the
// accepted Socket's own {within:} governs its reads. A peer that
// connects and says nothing must not pin the acceptor forever.
func TestAcceptTLSHandshakeDeadline(t *testing.T) {
	r, _ := tlsListenReg(t)
	lv, addr := listenTLS(t, r, nscMap("identity", native.NewAtom("srv")))

	// A raw TCP connection that never speaks TLS: the handshake stalls.
	stalled, dErr := net.Dial("tcp", addr)
	if dErr != nil {
		t.Fatal(dErr)
	}
	t.Cleanup(func() { _ = stalled.Close() })

	_, err := acceptHandler([]native.Value{
		lv, nscMap("within", native.NewInteger(150)),
	}, nil, nil, r)
	if err == nil {
		t.Fatal("a silent peer must time out the handshake, not hang")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("got %v, want a timeout", err)
	}
	_ = stalled.Close()

	// And the deadline is CLEARED once a real handshake completes, so
	// the accepted Socket's own {within:} governs its reads — otherwise
	// a listener accepted with a short budget would start failing reads
	// the moment that instant passed.
	type accepted struct {
		sock native.Value
		err  error
	}
	done := make(chan accepted, 1)
	go func() {
		out, aErr := acceptHandler([]native.Value{
			lv, nscMap("within", native.NewInteger(5000)),
		}, nil, nil, r)
		if aErr != nil {
			done <- accepted{err: aErr}
			return
		}
		done <- accepted{sock: out[0]}
	}()
	cli, dErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("verify", native.NewBoolean(false))),
	}, nil, nil, r)
	if dErr != nil {
		t.Fatalf("dial: %v", dErr)
	}
	got := <-done
	if got.err != nil {
		t.Fatalf("accept with within: %v", got.err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, sErr := sendBytesHandler([]native.Value{
		native.NewBytesValue([]byte("late\n")), cli[0],
	}, nil, nil, r); sErr != nil {
		t.Fatalf("send: %v", sErr)
	}
	echo, rErr := recvUntilHandler([]native.Value{
		got.sock, native.NewBytesValue([]byte("\n")),
	}, nil, nil, r)
	if rErr != nil {
		t.Fatalf("read after the accept deadline: %v", rErr)
	}
	if b, _ := native.AsBytesValue(echo[0]); string(b) != "late" {
		t.Errorf("server read %q", b)
	}
	_, _ = closeHandler([]native.Value{cli[0]}, nil, nil, r)
	_, _ = closeHandler([]native.Value{got.sock}, nil, nil, r)
}

// A peer that never speaks TLS to a serve-raw TLS listener is logged and
// dropped on its own goroutine — the acceptor keeps serving, and the
// handler is never handed an unauthenticated Socket.
func TestServeRawTLSHandshakeFailure(t *testing.T) {
	r, caPEM := tlsListenReg(t)
	r.ParseFunc = parser.Parse
	InstallResolver(r)
	r.Defs.Push("capem", native.NewString(caPEM))

	out, err := runNetStepsOn(t, r, []string{
		`import "boru:net"`,
		`def nl (convert Bytes "\n")`,
		`def l (Net.serve-raw {tcp: 0  host: "127.0.0.1"  tls: {identity: srv/q}} ([c:Any] => [
		   Net.send-bytes (Net.recv-until c nl {within: 5000}) c
		 ]))`,
		`def a (Net.addr l)`,
		`a.port`,
	})
	if err != nil {
		t.Fatalf("serve-raw: %v", err)
	}
	port, _ := out[len(out)-1].AsConcreteInteger()
	addr := "127.0.0.1:" + strconv.FormatInt(port, 10)

	// Plaintext into a TLS ear: the handshake fails on that connection.
	junk, dErr := net.Dial("tcp", addr)
	if dErr != nil {
		t.Fatal(dErr)
	}
	_, _ = junk.Write([]byte("GET / HTTP/1.0\r\n\r\n"))
	_ = junk.Close()

	// The acceptor survives it: a proper TLS client still gets served.
	out, err = runNetStepsOn(t, r, []string{
		`def s (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String a.port)])  tls: {ca: capem}})`,
		`Net.send-bytes (convert Bytes "still-here\n") s`,
		`def got (convert String (Net.recv-bytes s 10 {within: 5000}))`,
		`Net.close s; Net.close l; got`,
	})
	if err != nil {
		t.Fatalf("the acceptor did not survive a bad handshake: %v", err)
	}
	if s, _ := out[len(out)-1].AsConcreteString(); s != "still-here" {
		t.Errorf("echoed %q", s)
	}
}

// The server config builder is shared with the client one, so its
// failure modes are pinned directly too.
func TestTLSServerConfigFor(t *testing.T) {
	id, _ := mintServerCert(t)

	if _, err := capabilities.TLSServerConfigFor(capabilities.ServerTLSProfile{}, nil); err != capabilities.ErrNoServerIdentity {
		t.Errorf("a nil identity must be ErrNoServerIdentity, got %v", err)
	}
	if _, err := capabilities.TLSServerConfigFor(
		capabilities.ServerTLSProfile{ClientCAsPEM: "garbage"}, id); err != capabilities.ErrBadClientCAsPEM {
		t.Errorf("a malformed CA bundle must be ErrBadClientCAsPEM, got %v", err)
	}

	certPEM, _, _ := mintTLSClientCert(t)
	cfg, err := capabilities.TLSServerConfigFor(capabilities.ServerTLSProfile{
		ClientCAsPEM: string(certPEM), MinVersion: tls.VersionTLS13,
	}, id)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("require-client: must REQUIRE and verify — an unchecked certificate is decoration")
	}
	if cfg.ClientCAs == nil || cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("config did not carry the profile: %+v", cfg)
	}

	// GetCertificate is per-handshake and reports the identity's own
	// failures, including a decline (meaningless for a server).
	if _, gErr := cfg.GetCertificate(&tls.ClientHelloInfo{
		ServerName:       "x.example",
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
	}); gErr != nil {
		t.Errorf("GetCertificate: %v", gErr)
	}
	declining, _ := capabilities.TLSServerConfigFor(capabilities.ServerTLSProfile{},
		capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
			return nil, nil
		}))
	if _, gErr := declining.GetCertificate(&tls.ClientHelloInfo{ServerName: "x.example"}); gErr == nil ||
		!strings.Contains(gErr.Error(), "declined") {
		t.Errorf("a declining identity must fail the server handshake, got %v", gErr)
	}
	failing, _ := capabilities.TLSServerConfigFor(capabilities.ServerTLSProfile{},
		capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
			return nil, errFakeVaultMiss
		}))
	if _, gErr := failing.GetCertificate(&tls.ClientHelloInfo{}); gErr == nil {
		t.Error("an identity error must fail the server handshake")
	}
}

// A nil registry reaches the server-side helpers from direct-call
// tests; neither may dereference it.
func TestServerTLSNilRegistry(t *testing.T) {
	if err := native.CheckServerTLSPolicy(nil, capabilities.ServerTLSProfile{Identity: "x"}, "h", 1); err != nil {
		t.Errorf("no registry means no policy: %v", err)
	}
	r := nscReg(t)
	if err := native.CheckServerTLSPolicy(r, capabilities.ServerTLSProfile{Identity: "x"}, "h", 1); err != nil {
		t.Errorf("no policy installed means allow: %v", err)
	}
}
