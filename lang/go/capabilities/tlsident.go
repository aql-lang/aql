package capabilities

import (
	"crypto/tls"
)

// CertRequest is what the peer asked for during the TLS handshake — the
// BORU-facing projection of crypto/tls's CertificateRequestInfo. A
// ClientIdentity may use it to pick among several certificates.
type CertRequest struct {
	// Host is the peer being dialled.
	Host string
	// AcceptableCAs holds the DER-encoded X.501 distinguished names of
	// the CAs the server said it trusts. Empty means it did not say.
	AcceptableCAs [][]byte
	// SigSchemes are the signature schemes the server accepts.
	SigSchemes []uint16
}

// ClientIdentity supplies the client certificate for mutual TLS.
//
// It is an interface returning a *tls.Certificate rather than a struct
// holding bytes, and that is the whole point: a tls.Certificate's
// PrivateKey is a crypto.Signer, so an implementation may be backed by
// a file, a vault slot, an HSM, a KMS, or a SPIFFE agent that rotates
// hourly — the key need never exist in this process as bytes, and BORU
// never sees it either way. A credential modelled as cert-bytes +
// key-bytes could not express those cases.
//
// Certificate is consulted PER HANDSHAKE (it backs crypto/tls's
// GetClientCertificate callback, not the static Certificates slice), so
// rotation and per-server selection need no restart.
//
// Returning (nil, nil) declines to present a certificate, mirroring
// Go's empty-Certificate return; the server then decides whether that
// is fatal.
type ClientIdentity interface {
	Certificate(req CertRequest) (*tls.Certificate, error)
}

// IdentityFunc adapts a plain function to ClientIdentity.
type IdentityFunc func(CertRequest) (*tls.Certificate, error)

// Certificate implements ClientIdentity.
func (f IdentityFunc) Certificate(req CertRequest) (*tls.Certificate, error) { return f(req) }

// StaticIdentity builds a ClientIdentity from a PEM certificate chain
// and private key held in memory. The pair is parsed and validated
// once, here, so a mismatched cert/key fails at registration rather
// than at the first handshake.
func StaticIdentity(certPEM, keyPEM []byte) (ClientIdentity, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return IdentityFunc(func(CertRequest) (*tls.Certificate, error) { return &cert, nil }), nil
}

// FileIdentity builds a ClientIdentity from PEM files on disk.
//
// The paths are the HOST's, supplied by host Go code at registration —
// never a path chosen by guest BORU source. That distinction is the
// reason `tls: {identity: …}` names a credential instead of pointing at
// one: a guest-chosen path dereferenced under host authority would hand
// a program holding `network` a file read it is not entitled to.
func FileIdentity(certPath, keyPath string) (ClientIdentity, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return IdentityFunc(func(CertRequest) (*tls.Certificate, error) { return &cert, nil }), nil
}

// getClientCertificate adapts a ClientIdentity to crypto/tls's
// per-handshake callback.
func getClientCertificate(id ClientIdentity, host string) func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		schemes := make([]uint16, len(cri.SignatureSchemes))
		for i, s := range cri.SignatureSchemes {
			schemes[i] = uint16(s)
		}
		cert, err := id.Certificate(CertRequest{
			Host:          host,
			AcceptableCAs: cri.AcceptableCAs,
			SigSchemes:    schemes,
		})
		if err != nil {
			return nil, err
		}
		if cert == nil {
			// Decline politely — Go's documented "send no certificate".
			return &tls.Certificate{}, nil
		}
		return cert, nil
	}
}
