package native

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
)

// MapTransportErr classifies a Go network error into AQL's transport
// failure vocabulary, per design/NETWORK-CLIENTS.0.md §8.2:
//
//	closed     — the peer hung up (EOF, reset, use-of-closed)
//	timeout    — a deadline was exceeded
//	transport  — everything else that failed on the wire
//
// It exists so the socket layer (aql:net's connect-raw/recv/send words)
// and the HTTP layer (fetch) produce the SAME code for the same
// condition — the design's "one set from the bottom up". A guest can
// therefore write one `case` over the codes regardless of which layer
// raised it.
//
// Callers must only pass errors that came from an actual wire
// operation; usage errors (a bad option, a malformed URL) are the
// caller's own coded error, not a transport failure.
func MapTransportErr(r *Registry, word string, err error) error {
	return MapTransportErrAs(r, word, word, err)
}

// MapTransportErrAs is MapTransportErr with an explicit message prefix,
// for callers that want to say WHICH wire operation failed while still
// raising the shared code. `word` names the AQL word for the error;
// `what` prefixes the detail (e.g. "fetch: reading body").
func MapTransportErrAs(r *Registry, word, what string, err error) error {
	var nerr net.Error
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed), errors.Is(err, io.ErrClosedPipe):
		return r.AqlError("closed", what+": connection closed by peer", word)
	case errors.As(err, &nerr) && nerr.Timeout():
		return r.AqlError("timeout", what+": deadline exceeded", word)
	default:
		// Write-side peer resets surface as syscall errors; the closed
		// code is what handler loops dispatch on.
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return r.AqlError("timeout", what+": deadline exceeded", word)
		}
		msg := err.Error()
		if ContainsAny(msg, "broken pipe", "connection reset", "use of closed network connection") {
			return r.AqlError("closed", what+": "+msg, word)
		}
		return r.AqlError("transport", what+": "+msg, word)
	}
}

// ContainsAny reports whether s contains any of subs. Empty needles
// never match.
func ContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
