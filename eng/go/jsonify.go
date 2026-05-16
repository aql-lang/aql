package eng

import "errors"

// Jsonifier is an optional capability interface. Types implementing
// it expose a "project to a Node or Scalar" transformation — used by
// the `jsonify` word to map a value into AQL's data subset (Integer,
// String, Boolean, Atom, List, Map, …) without going through a JSON
// string round-trip. The result is a Value, not a serialised []byte;
// callers that need a JSON-encoded string compose this with a
// downstream string encoder.
//
// Conventions mirror Comparer: a wrapper Behavior that holds the
// capability slot but no body returns ErrNoJsonifier so the JsonifyValue
// walk continues the parent chain instead of treating the wrapper as
// the final Jsonifier.
type Jsonifier interface {
	Jsonify(v Value) (Value, error)
}

// ErrNoJsonifier signals that a Jsonifier-shaped Behavior wrapper has
// no body installed for this capability — JsonifyValue keeps walking.
var ErrNoJsonifier = errors.New("eng: no jsonifier in this Behavior")

// JsonifyValue projects v into a Node or Scalar by walking v.VType's
// parent chain looking for a Jsonifier. If none is registered, v is
// returned unchanged — the natural default for values already in
// data-shape (Integer, String, Map, List, …).
//
// No auto-recursion: the result Value is whatever the registered body
// produces. Bodies that need to project nested Object fields call
// `jsonify` again inside themselves.
func JsonifyValue(v Value) (Value, error) {
	if v.VType == nil {
		return v, nil
	}
	for t := v.VType; t != nil; t = t.Parent {
		j, ok := t.Behavior.(Jsonifier)
		if !ok {
			continue
		}
		out, err := j.Jsonify(v)
		if errors.Is(err, ErrNoJsonifier) {
			continue
		}
		return out, err
	}
	return v, nil
}
