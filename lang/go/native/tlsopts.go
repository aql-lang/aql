package native

import (
	"crypto/tls"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/policy"
)

// ParseTLSOpts reads a guest `tls: {…}` option map into a resolved
// TLSProfile, per design/NETWORK-TLS-PLAN.0.md §4.3. One parser serves
// every call site (fetch today, connect-raw next) so the two cannot
// drift apart.
//
//	identity: Atom|String|handle  a host-registered client identity, or
//	          the opaque handle `Vault.identity` returns (mutual TLS)
//	verify: Boolean   false disables chain AND hostname checking
//	ca:     Bytes|String   additional CA roots, PEM; REPLACES the system pool
//	sni:    String    server-name override
//	min:    String    "1.2" | "1.3"
//
// Unknown keys are REJECTED rather than ignored: this is a
// security-sensitive map, and silently dropping a misspelled `verifiy:`
// would leave the caller believing they had configured something.
func ParseTLSOpts(r *Registry, v Value, word string) (capabilities.TLSProfile, error) {
	var p capabilities.TLSProfile
	mp, err := RequireConcreteMap(v, word)
	if err != nil {
		return p, err
	}
	for _, key := range mp.Keys() {
		val, _ := mp.Get(key)
		switch key {
		case "verify":
			b, bErr := val.AsConcreteBoolean()
			if bErr != nil {
				return p, r.AqlError("fetch_error",
					word+": tls: verify: must be a Boolean", word)
			}
			p.Insecure = !b
		case "ca":
			pem, ok := tlsPEMArg(val)
			if !ok {
				return p, r.AqlError("fetch_error",
					word+": tls: ca: must be Bytes or a String of PEM", word)
			}
			p.RootsPEM = pem
		case "identity":
			// Atom is the idiomatic spelling (`identity: acme/q`);
			// String is accepted so a computed name works too.
			name, ok := tlsIdentityArg(val)
			if !ok {
				return p, r.AqlError("fetch_error",
					word+": tls: identity: must be an Atom or non-empty String", word)
			}
			p.Identity = name
		case "sni":
			s, sErr := val.AsConcreteString()
			if sErr != nil || s == "" {
				return p, r.AqlError("fetch_error",
					word+": tls: sni: must be a non-empty String", word)
			}
			p.ServerName = s
		case "min":
			s, sErr := val.AsConcreteString()
			if sErr != nil {
				return p, r.AqlError("fetch_error",
					word+`: tls: min: must be "1.2" or "1.3"`, word)
			}
			switch s {
			case "1.2":
				p.MinVersion = tls.VersionTLS12
			case "1.3":
				p.MinVersion = tls.VersionTLS13
			default:
				return p, r.AqlError("fetch_error",
					word+`: tls: min: must be "1.2" or "1.3", got "`+s+`"`, word)
			}
		default:
			return p, r.AqlError("fetch_error",
				word+": tls: unknown option \""+key+"\"", word)
		}
	}
	return p, nil
}

// TLSIdentityHandles lets a module teach ParseTLSOpts an opaque handle
// shape that names a registered identity — aql:vault registers one so
// `tls: {identity: (Vault.identity "acme")}` works without a private key
// ever becoming an AQL value.
//
// It is a hook rather than a direct call because the handle types live
// in lang/go/modules, which imports this package; the dependency cannot
// run the other way.
var TLSIdentityHandles []func(Value) (string, bool)

// tlsIdentityArg accepts an Atom (the idiomatic `acme/q`), a String, or
// a registered opaque handle.
func tlsIdentityArg(v Value) (string, bool) {
	for _, h := range TLSIdentityHandles {
		if name, ok := h(v); ok {
			return name, true
		}
	}
	if a, err := v.AsConcreteAtom(); err == nil && a != "" {
		return a, true
	}
	if s, err := v.AsConcreteString(); err == nil && s != "" {
		return s, true
	}
	return "", false
}

// tlsPEMArg accepts either a Bytes value (the natural output of reading
// a .pem through aql:io) or a String holding PEM text.
func tlsPEMArg(v Value) (string, bool) {
	if b, ok := AsBytesValue(v); ok {
		return string(b), true
	}
	if s, err := v.AsConcreteString(); err == nil && s != "" {
		return s, true
	}
	return "", false
}

// CheckTLSPolicy gates the parts of a TLSProfile that carry authority:
// `verify: false` (network/tls-insecure) removes verification, and
// `identity:` (network/client-cert) presents a credential. Supplying CA
// roots, an SNI override or a minimum version narrows or re-points
// trust rather than widening it, so those are ungated. Both ops sit in
// the existing `network` scope — a declared scope default-denies ops it
// does not list, so a profile that gates network at all gates these
// without further wiring.
func CheckTLSPolicy(r *Registry, p capabilities.TLSProfile, host string, port int) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	args := policy.Args{"host": host, "port": port}
	if p.Identity != "" {
		// Presenting a credential is its own authority: the policy —
		// not the program — decides WHICH identity may be used and
		// against which host. The identity name rides in the args so a
		// profile can allow-list per host.
		args["identity"] = p.Identity
		if err := pol.Check("network", "client-cert", args); err != nil {
			return err
		}
	}
	if p.Insecure {
		return pol.Check("network", "tls-insecure", args)
	}
	return nil
}
