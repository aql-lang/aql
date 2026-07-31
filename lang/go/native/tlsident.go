package native

import (
	"crypto/tls"
	"net/http"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// HostClientIdents returns the registry's client-identity registry, or
// nil when the host registered none.
func HostClientIdents(r *Registry) map[string]capabilities.ClientIdentity {
	ids, _, _ := eng.Cap[map[string]capabilities.ClientIdentity](r, CapClientIdents)
	return ids
}

// SetHostClientIdents installs the whole identity map. When the policy
// uninstalls the network scope the slot is removed — unlike the
// transport (which is not itself an authority), a client identity IS a
// credential, so a profile that disables networking must not leave one
// reachable.
func SetHostClientIdents(r *Registry, ids map[string]capabilities.ClientIdentity) {
	if r == nil {
		return
	}
	if pol := HostPolicy(r); pol != nil && !pol.Installed("network") {
		_, _ = r.Capabilities.Delete(CapClientIdents)
		return
	}
	_ = r.Capabilities.Set(CapClientIdents, ids)
}

// RegisterClientIdentity adds one named identity, creating the map on
// first use — the same lazy shape RegisterFormat uses.
func RegisterClientIdentity(r *Registry, name string, id capabilities.ClientIdentity) {
	if r == nil || name == "" || id == nil {
		return
	}
	ids := HostClientIdents(r)
	if ids == nil {
		ids = map[string]capabilities.ClientIdentity{}
	}
	ids[name] = id
	SetHostClientIdents(r, ids)
}

// resolveIdentity looks up the identity a profile names. An unknown
// name is an error rather than a silent "no certificate": a program
// that asked to authenticate and silently did not would fail later,
// somewhere less obvious, as an opaque server-side rejection.
func resolveIdentity(r *Registry, p capabilities.TLSProfile, word string) (capabilities.ClientIdentity, error) {
	if p.Identity == "" {
		return nil, nil
	}
	id, ok := HostClientIdents(r)[p.Identity]
	if !ok || id == nil {
		return nil, r.BoruErrorHint("net_error",
			word+`: tls: no client identity named "`+p.Identity+`" is registered`, word,
			"the host registers identities with (*Boru).RegisterClientIdentity; "+
				"boru source can name one but cannot create one")
	}
	return id, nil
}

// TLSConfigForProfile resolves the profile's identity against r and
// builds the crypto/tls configuration. It is the seam the raw-socket
// words use: they need a *tls.Config rather than an http.RoundTripper,
// but must resolve identities and interpret options identically to
// fetch — so both go through one parser (ParseTLSOpts) and one config
// builder (capabilities.TLSConfigFor).
func TLSConfigForProfile(r *Registry, p capabilities.TLSProfile, word string) (*tls.Config, error) {
	id, err := resolveIdentity(r, p, word)
	if err != nil {
		return nil, err
	}
	return capabilities.TLSConfigFor(p, id)
}

// TLSServerConfigForProfile is the listen-side twin of
// TLSConfigForProfile: it resolves the named credential against r and
// builds the server configuration. The identity is REQUIRED here (a
// server with no certificate cannot handshake), so an unnamed or
// unregistered one is an error rather than "present none".
func TLSServerConfigForProfile(r *Registry, p capabilities.ServerTLSProfile, word string) (*tls.Config, error) {
	id, err := resolveIdentity(r, capabilities.TLSProfile{Identity: p.Identity}, word)
	if err != nil {
		return nil, err
	}
	return capabilities.TLSServerConfigFor(p, id)
}

// transportCacheMax bounds the per-registry profile→transport cache.
// The key space is guest-influenced (any CA PEM, any SNI), so an
// unbounded map is a slow leak in a long-lived server. Past the cap a
// transport is built per call: correctness is unaffected, only
// connection reuse for the overflow profiles.
const transportCacheMax = 64

// ResolveTransport resolves a profile to a transport, caching per
// REGISTRY rather than per process. The cache cannot be global: a
// profile names its identity, and two registries may bind the same name
// to different credentials, so a process-wide cache keyed on the
// profile would serve one registry's client certificate to another's
// request.
func ResolveTransport(r *Registry, p capabilities.TLSProfile, word string) (http.RoundTripper, error) {
	id, err := resolveIdentity(r, p, word)
	if err != nil {
		return nil, err
	}
	cache, _, _ := eng.Cap[map[capabilities.TLSProfile]http.RoundTripper](r, CapHTTPTransports)
	if rt, ok := cache[p]; ok {
		return rt, nil
	}
	rt, err := EffectiveHTTPOps(r).Transport(p, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		// The cache lives on the registry; without one there is nowhere
		// to put it. Handlers are reachable with a nil registry from
		// direct-call tests, so this returns the transport uncached
		// rather than dereferencing nil.
		return rt, nil
	}
	if cache == nil {
		cache = map[capabilities.TLSProfile]http.RoundTripper{}
		_ = r.Capabilities.Set(CapHTTPTransports, cache)
	}
	if len(cache) < transportCacheMax {
		cache[p] = rt
	}
	return rt, nil
}
