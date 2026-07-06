package lsp

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestS7n_RespondOuterMarshalError drives the SECOND json.Marshal error
// arm in respond (the one that encodes the whole rawMessage envelope,
// not the result). Unlike the result-marshal arm (already covered by
// TestRespondMarshalError), this one needs the id itself to carry
// invalid raw JSON: json.RawMessage.MarshalJSON never validates its
// bytes, but the top-level Marshal still runs compact() over them,
// which is where the error actually originates. Driven organically —
// no seam needed, since respond's id parameter is directly caller-
// supplied.
func TestS7n_RespondOuterMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.respond(json.RawMessage("{not valid json"), "ok"); err == nil {
		t.Error("respond with an invalid raw id succeeded, want a marshal error")
	}
}

// TestS7n_RespondErrorOuterMarshalError is the respondError analogue:
// same invalid-id trick drives its lone json.Marshal call to fail.
func TestS7n_RespondErrorOuterMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.respondError(json.RawMessage("{not valid json"), errInternalError, "x"); err == nil {
		t.Error("respondError with an invalid raw id succeeded, want a marshal error")
	}
}

// TestS7n_NotifyOuterMarshalError drives notify's SECOND json.Marshal
// error arm (the envelope encode, via the jsonMarshal seam — the
// params encode a line above stays the real json.Marshal and succeeds
// trivially for a plain map). Unlike respond/respondError, notify has
// no caller-supplied RawMessage field reachable through the public
// API at that point (id is unset/omitted, params already valid), so
// only a seam can observe this arm.
func TestS7n_NotifyOuterMarshalError(t *testing.T) {
	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })
	jsonMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("boom-outer-marshal")
	}

	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.notify("textDocument/publishDiagnostics", map[string]any{"a": 1}); err == nil {
		t.Error("notify() succeeded, want the seamed outer-marshal error")
	} else if !strings.Contains(err.Error(), "boom-outer-marshal") {
		t.Errorf("err = %v, want it to contain boom-outer-marshal", err)
	}
}
