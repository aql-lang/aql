package capabilities

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"sync"
)

// HTTPOps is the host capability that supplies the outbound HTTP
// transport used by aql:net's `fetch`. It exists so the transport —
// and therefore TLS configuration, the connection pool, and protocol
// negotiation — is chosen by the host rather than hard-wired into the
// word handler.
//
// It returns a RoundTripper rather than an *http.Client deliberately.
// The pool and the TLS configuration live on the transport, while the
// per-request timeout lives on the client, so `fetch` builds a cheap
// client around a shared transport for each call. Handing back a client
// instead would force the implementation to key its cache on the
// guest-supplied timeout, which is an unbounded cache keyed by
// untrusted input.
//
// The TLSProfile argument is the resolved TLS configuration for the
// request (see TLSProfile). Implementations should treat it as a cache
// key: it is comparable, and equal profiles must yield the same
// transport so connection reuse survives.
type HTTPOps interface {
	Transport(p TLSProfile) (http.RoundTripper, error)
}

// TLSProfile is the resolved TLS configuration for one outbound
// request, derived from the guest's `tls:` option map.
//
// The zero value means "Go's defaults": verify against the system root
// pool, no server-name override, Go's minimum protocol version. Note
// which way the Insecure field points — per the no-zero-value-overload
// rule (eng/go/CLAUDE.md), the SAFE state is the zero value, so a
// profile nobody configured cannot silently skip verification.
//
// Every field is comparable so the whole struct can be a map key.
type TLSProfile struct {
	// Insecure disables verification of the peer's certificate chain
	// AND host name. It is policy-gated (network/tls-insecure).
	Insecure bool
	// RootsPEM is additional CA roots in PEM form. Empty means the
	// system pool. When set, the roots REPLACE the system pool — an
	// explicit private-CA endpoint should not also trust the public
	// web.
	RootsPEM string
	// ServerName overrides SNI and the name checked against the
	// certificate. Empty means derive from the URL host.
	ServerName string
	// MinVersion is a crypto/tls VersionTLSxx constant; 0 means Go's
	// default minimum.
	MinVersion uint16
}

// ErrBadRootsPEM is returned when RootsPEM contains no usable
// certificate — a typo or a truncated file, which would otherwise
// silently produce an empty trust pool that rejects everything.
var ErrBadRootsPEM = errors.New("tls: ca contains no usable certificate")

// DefaultHTTPOps is the HTTPOps used when a host installs none. For the
// zero profile it serves http.DefaultTransport, which is exactly what
// an *http.Client with a nil Transport would have used — so installing
// nothing preserves the historical behaviour, including proxy support
// from the environment, the shared idle-connection pool, and automatic
// HTTP/2. For a configured profile it builds a transport cloned from
// http.DefaultTransport so those same defaults are kept.
type DefaultHTTPOps struct{}

// tlsTransportCacheMax bounds the profile→transport cache. The key
// space is guest-influenced (any CA PEM, any SNI), so an unbounded map
// would be a slow leak in a long-lived server. Past the cap a fresh
// transport is built per call: correctness is unaffected, only
// connection reuse for the overflow profiles.
const tlsTransportCacheMax = 64

var (
	tlsTransportMu sync.Mutex
	tlsTransports  = map[TLSProfile]http.RoundTripper{}
)

// Transport returns the transport for p, building and caching one when
// p is not the zero profile.
func (DefaultHTTPOps) Transport(p TLSProfile) (http.RoundTripper, error) {
	if p == (TLSProfile{}) {
		return http.DefaultTransport, nil
	}
	tlsTransportMu.Lock()
	if rt, ok := tlsTransports[p]; ok {
		tlsTransportMu.Unlock()
		return rt, nil
	}
	tlsTransportMu.Unlock()

	rt, err := buildTransport(p)
	if err != nil {
		return nil, err
	}
	tlsTransportMu.Lock()
	if len(tlsTransports) < tlsTransportCacheMax {
		tlsTransports[p] = rt
	}
	tlsTransportMu.Unlock()
	return rt, nil
}

// buildTransport clones http.DefaultTransport and applies p.
func buildTransport(p TLSProfile) (http.RoundTripper, error) {
	cfg := &tls.Config{
		InsecureSkipVerify: p.Insecure, //nolint:gosec // policy-gated; see network/tls-insecure
		ServerName:         p.ServerName,
		MinVersion:         p.MinVersion,
	}
	if p.RootsPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(p.RootsPEM)) {
			return nil, ErrBadRootsPEM
		}
		cfg.RootCAs = pool
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		//covergate:allow http.DefaultTransport is a *http.Transport in every supported Go release
		return nil, errors.New("tls: http.DefaultTransport is not an *http.Transport")
	}
	tr := base.Clone()
	tr.TLSClientConfig = cfg
	// Setting TLSClientConfig on a Transport disables automatic HTTP/2
	// unless this is on; without it, configuring TLS would silently
	// downgrade every request to HTTP/1.1. Clone() carries
	// DefaultTransport's true, but set it explicitly so a future change
	// to the default cannot reintroduce the downgrade unnoticed.
	tr.ForceAttemptHTTP2 = true
	return tr, nil
}
