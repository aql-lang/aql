package native

import (
	"encoding/json"

	eng "github.com/aql-lang/aql/eng/go"
)

// valueToKVBytes serializes an AQL value to JSON bytes for backends that
// persist to byte storage (file, Redis, S3). It reuses the jsonify
// projection (valueToAnySer, including any custom nodify behavior) so
// the on-disk form matches `jsonify`, and round-trips through
// kvBytesToValue.
func valueToKVBytes(v Value) ([]byte, error) {
	projected, err := eng.NodifyValue(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(valueToAnySer(projected))
}

// kvBytesToValue is the inverse of valueToKVBytes: it decodes JSON bytes
// back into an AQL value via the same rawToValue hydration `reify` uses.
// Empty input decodes to none.
func kvBytesToValue(b []byte) (Value, error) {
	if len(b) == 0 {
		return NewNone(), nil
	}
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return Value{}, err
	}
	return rawToValue(raw)
}
