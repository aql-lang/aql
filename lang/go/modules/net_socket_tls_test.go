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
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// tlsEchoServer starts a local TLS listener that echoes one line, and
// returns its "host:port" plus its self-signed CA in PEM form. The
// certificate carries a 127.0.0.1 IP SAN so the dial verifies by
// address without an sni: override.
func tlsEchoServer(t *testing.T) (addr, caPEM string) {
	t.Helper()
	return tlsEchoServerCAs(t, nil)
}

// mtlsEchoServer is the same listener with client authentication turned
// on: only a peer holding a certificate signed by clientCAs gets in.
func mtlsEchoServer(t *testing.T, clientCAs *x509.CertPool) (addr, caPEM string) {
	t.Helper()
	return tlsEchoServerCAs(t, clientCAs)
}

// tlsEchoServerCAs backs both. A nil clientCAs leaves ClientAuth at its
// zero value (no client certificate asked for), so the server-auth-only
// and mutual cases share one implementation.
func tlsEchoServerCAs(t *testing.T, clientCAs *x509.CertPool) (addr, caPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "aql-test-server"},
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
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if clientCAs != nil {
		cfg.ClientCAs = clientCAs
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64)
				n, rErr := c.Read(buf)
				if rErr == nil {
					_, _ = c.Write(buf[:n])
				}
			}(c)
		}
	}()
	return ln.Addr().String(), string(pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// A TLS socket is the SAME Socket with the SAME words — TLS is an
// option on the dial, not a separate API (NETWORK-SERVERS.0.md §4.4).
// The negative half proves verification is real: without the CA the
// handshake fails rather than silently proceeding.
func TestConnectRawTLS(t *testing.T) {
	addr, caPEM := tlsEchoServer(t)
	r := nscReg(t)

	// Without the CA: the handshake is refused.
	if _, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap()),
	}, nil, nil, r); err == nil {
		t.Fatal("an untrusted server must not be accepted")
	}

	// With it: a plain Socket that recv/send words drive unchanged.
	out, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("ca", native.NewString(caPEM))),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("TLS connect-raw: %v", err)
	}
	sock := out[0]
	if _, ok := asSocket(sock); !ok {
		t.Fatalf("expected a Socket, got %v", sock)
	}
	if _, err = sendBytesHandler([]native.Value{
		native.NewBytesValue([]byte("ping\n")), sock,
	}, nil, nil, r); err != nil {
		t.Fatalf("send over TLS: %v", err)
	}
	got, err := recvUntilHandler([]native.Value{
		sock, native.NewBytesValue([]byte("\n")),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("recv over TLS: %v", err)
	}
	b, _ := native.AsBytesValue(got[0])
	if string(b) != "ping" {
		t.Errorf("echoed %q, want \"ping\"", b)
	}
	if _, err = closeHandler([]native.Value{sock}, nil, nil, r); err != nil {
		t.Errorf("close: %v", err)
	}
}

// verify:false reaches the same server with no CA supplied — the
// escape hatch works, and (no policy installed) is ungated.
func TestConnectRawTLSInsecure(t *testing.T) {
	addr, _ := tlsEchoServer(t)
	r := nscReg(t)
	out, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("verify", native.NewBoolean(false))),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("verify:false connect: %v", err)
	}
	_, _ = closeHandler([]native.Value{out[0]}, nil, nil, r)
}

// The socket path shares fetch's parser and policy, so a malformed
// option and a denied verify:false behave identically here.
func TestConnectRawTLSOptionsAndPolicy(t *testing.T) {
	addr, _ := tlsEchoServer(t)

	r := nscReg(t)
	_, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("verifiy", native.NewBoolean(false))),
	}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), `unknown option "verifiy"`) {
		t.Fatalf("got %v, want the shared parser's unknown-option error", err)
	}

	pol, pErr := policy.LoadInline(`{
		name: "net-no-insecure-tls"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["connect"] }] } }
		}
	}`)
	if pErr != nil {
		t.Fatal(pErr)
	}
	rPol, rErr := native.DefaultRegistryWithPolicy(pol)
	if rErr != nil {
		t.Fatal(rErr)
	}
	_, dErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"tls", nscMap("verify", native.NewBoolean(false))),
	}, nil, nil, rPol)
	if dErr == nil {
		t.Fatal("expected tls-insecure to be denied on connect-raw")
	}
	if d, ok := dErr.(*policy.Denied); !ok || d.Op != "tls-insecure" {
		t.Errorf("got %v, want a tls-insecure denial", dErr)
	}
}

// A tls: option on a unix socket is parsed, not silently dropped —
// the same reason ParseTLSOpts rejects unknown keys.
func TestParseNetAddrUnixParsesTLS(t *testing.T) {
	r := nscReg(t)
	opts, err := parseNetAddr(r, nscMap(
		"unix", native.NewString("/tmp/x.sock"),
		"tls", nscMap("sni", native.NewString("inner.example")),
	), "connect-raw", false)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.hasTLS || opts.tlsProfile.ServerName != "inner.example" {
		t.Errorf("unix tls: not parsed: %+v", opts)
	}
	// And a malformed one is rejected rather than ignored.
	if _, bad := parseNetAddr(r, nscMap(
		"unix", native.NewString("/tmp/x.sock"),
		"tls", nscMap("nope", native.NewBoolean(true)),
	), "connect-raw", false); bad == nil {
		t.Error("a malformed tls: on a unix socket must be rejected")
	}
}

// An unknown identity fails the dial before any handshake.
func TestConnectRawTLSUnknownIdentity(t *testing.T) {
	addr, caPEM := tlsEchoServer(t)
	r := nscReg(t)
	_, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap(
			"ca", native.NewString(caPEM),
			"identity", native.NewAtom("absent"),
		)),
	}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), `no client identity named "absent"`) {
		t.Fatalf("got %v, want an unknown-identity error", err)
	}
}

// A connect deadline bounds the HANDSHAKE, then is cleared so recv/send
// carry their own {within:}. Without clearing it, a socket dialled with
// a connect timeout would start failing reads once that instant passed.
func TestConnectRawTLSWithinDeadline(t *testing.T) {
	addr, caPEM := tlsEchoServer(t)
	r := nscReg(t)
	out, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr),
			"within", native.NewInteger(5000),
			"tls", nscMap("ca", native.NewString(caPEM))),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("TLS connect with within: %v", err)
	}
	sock := out[0]
	// The deadline must be cleared: sleep past a short connect budget
	// and a read still works.
	time.Sleep(20 * time.Millisecond)
	if _, err = sendBytesHandler([]native.Value{
		native.NewBytesValue([]byte("late\n")), sock,
	}, nil, nil, r); err != nil {
		t.Fatalf("send after the connect deadline: %v", err)
	}
	got, rErr := recvUntilHandler([]native.Value{
		sock, native.NewBytesValue([]byte("\n")),
	}, nil, nil, r)
	if rErr != nil {
		t.Fatalf("recv after the connect deadline: %v", rErr)
	}
	if b, _ := native.AsBytesValue(got[0]); string(b) != "late" {
		t.Errorf("echoed %q", b)
	}
	_, _ = closeHandler([]native.Value{sock}, nil, nil, r)
}

// An sni: override replaces the name derived from the dial address —
// here it names something the certificate does NOT cover, so the
// handshake must fail. That is the assertion: the override took effect.
func TestConnectRawTLSSNIOverride(t *testing.T) {
	addr, caPEM := tlsEchoServer(t)
	r := nscReg(t)
	_, err := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap(
			"ca", native.NewString(caPEM),
			"sni", native.NewString("not-in-the-cert.example"),
		)),
	}, nil, nil, r)
	if err == nil {
		t.Fatal("an sni: the certificate does not cover must fail verification")
	}
}

// The bind path shares the same tls: parser, so a malformed option is
// rejected there too rather than reaching the listener.
func TestParseNetAddrListenRejectsBadTLS(t *testing.T) {
	r := nscReg(t)
	if _, err := parseNetAddr(r, nscMap(
		"tcp", native.NewInteger(0),
		"tls", nscMap("bogus", native.NewBoolean(true)),
	), "listen", true); err == nil {
		t.Error("a malformed tls: on listen must be rejected")
	}
	// And a well-formed one is carried through.
	opts, err := parseNetAddr(r, nscMap(
		"tcp", native.NewInteger(0),
		"tls", nscMap("min", native.NewString("1.3")),
	), "listen", true)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.hasTLS || opts.tlsProfile.MinVersion == 0 {
		t.Errorf("listen tls: not parsed: %+v", opts)
	}
}

// InstallNetExports is the test/host shim for `import "aql:net"`. It
// must transplant the accessor word extensions exactly as the real
// import does — otherwise a test using the shim and a guest using
// import would disagree about whether a Response answers .status.
func TestInstallNetExportsTransplants(t *testing.T) {
	r := nscReg(t)
	if err := InstallNetExports(r); err != nil {
		t.Fatalf("InstallNetExports: %v", err)
	}
	// The bare accessor must now carry the Fetch overloads the module
	// exported; before the transplant it carried only the core sigs.
	fn := r.Lookup("dot")
	if fn == nil {
		t.Fatal("dot is not registered")
	}
	var sawFetch bool
	for _, sig := range fn.Signatures {
		for i := 0; i < sig.TotalArgs(); i++ {
			if len(sig.Args) > i && sig.Args[i] != nil &&
				strings.Contains(sig.Args[i].String(), "Fetch") {
				sawFetch = true
			}
		}
	}
	if !sawFetch {
		t.Error("InstallNetExports did not transplant the Fetch accessor overloads")
	}
}
