package capabilities

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

// ServerTLSProfile is the resolved TLS configuration for a listening
// socket, derived from the guest's `tls:` option map on `listen`.
//
// It is a separate type from TLSProfile rather than extra fields on it,
// for two reasons. The options genuinely differ — a server does not
// verify a server chain (`verify:`), does not send SNI (`sni:`), and
// does not choose roots for one (`ca:`); it presents a certificate and
// optionally demands one back. And TLSProfile is the key of the
// outbound transport cache, so a server-only field there would be dead
// weight in every client lookup.
//
// The zero value is not usable: a TLS server with no certificate has
// nothing to present, so Identity is required.
type ServerTLSProfile struct {
	// Identity names a host-registered credential to present as the
	// SERVER certificate. Like the client side it is a NAME: guest
	// source selects a credential, it can never read or build one.
	Identity string
	// ClientCAsPEM is the trust anchor set for CLIENT certificates, in
	// PEM form. Empty means no client certificate is requested; set
	// means one is REQUIRED and verified against exactly these roots.
	// There is no "request but do not verify" middle setting — a
	// certificate nobody checks is decoration.
	ClientCAsPEM string
	// MinVersion is a crypto/tls VersionTLSxx constant; 0 means Go's
	// default minimum.
	MinVersion uint16
}

// ErrNoServerIdentity is returned when a server profile names no
// credential. It is a distinct error rather than a silent plaintext
// listener: quietly serving cleartext on a socket the program asked to
// be TLS is the worst possible reading of the mistake.
var ErrNoServerIdentity = errors.New("tls: listen: tls: needs identity: — a TLS server must present a certificate")

// ErrBadClientCAsPEM is returned when require-client: holds no usable
// certificate. An empty pool would reject every client, which reads as
// "my certificate is wrong" rather than "my CA bundle is wrong".
var ErrBadClientCAsPEM = errors.New("tls: require-client: contains no usable certificate")

// TLSServerConfigFor builds the crypto/tls configuration a server
// profile describes. It mirrors TLSConfigFor on the client side, and
// takes the same ClientIdentity — the interface is the credential seam
// for both directions, so a vault- or HSM-backed key serves a listener
// exactly as it serves a dial. (Its name predates the server side; the
// TLS role of the certificate differs, the shape of the seam does not.)
func TLSServerConfigFor(p ServerTLSProfile, id ClientIdentity) (*tls.Config, error) {
	if id == nil {
		return nil, ErrNoServerIdentity
	}
	cfg := &tls.Config{
		MinVersion:     p.MinVersion,
		GetCertificate: getServerCertificate(id),
	}
	if p.ClientCAsPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(p.ClientCAsPEM)) {
			return nil, ErrBadClientCAsPEM
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// getServerCertificate adapts a ClientIdentity to crypto/tls's
// per-handshake server callback. Per-handshake (rather than the static
// Certificates slice) for the same reason the client side is: the
// credential may be rotated, vault-held, or chosen by the name the
// client asked for.
func getServerCertificate(id ClientIdentity) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		schemes := make([]uint16, len(hello.SignatureSchemes))
		for i, s := range hello.SignatureSchemes {
			schemes[i] = uint16(s)
		}
		cert, err := id.Certificate(CertRequest{
			Host:       hello.ServerName,
			SigSchemes: schemes,
		})
		if err != nil {
			return nil, err
		}
		if cert == nil {
			// Declining is meaningful for a client ("send none") and
			// meaningless for a server — there is no handshake without
			// a certificate, so say which side failed and why.
			return nil, fmt.Errorf("tls: the identity declined to supply a server certificate for %q", hello.ServerName)
		}
		return cert, nil
	}
}
