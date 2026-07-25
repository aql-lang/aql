package capabilities

import (
	"net/http"
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
// pool, no client certificate, no server-name override. It is
// deliberately a comparable struct so HTTPOps implementations can use
// it directly as a map key.
//
// It carries no fields yet — the transport seam lands before the
// options that populate it, so this phase is a pure refactor with no
// behaviour change.
type TLSProfile struct{}

// DefaultHTTPOps is the HTTPOps used when a host installs none. It
// serves http.DefaultTransport for the default profile, which is
// exactly what an *http.Client with a nil Transport would have used —
// so installing nothing preserves the historical behaviour, including
// proxy support from the environment, the shared idle-connection pool,
// and automatic HTTP/2.
type DefaultHTTPOps struct{}

// Transport returns the shared default transport.
func (DefaultHTTPOps) Transport(_ TLSProfile) (http.RoundTripper, error) {
	return http.DefaultTransport, nil
}
