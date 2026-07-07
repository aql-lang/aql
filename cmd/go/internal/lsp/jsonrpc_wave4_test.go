// Wave-4 coverage for the JSON-RPC framing tails: the marshal-error
// arms of respond and notify (a result/params value that cannot be
// encoded to JSON).
package lsp

import (
	"io"
	"strings"
	"testing"
)

func TestW4RespondMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	// A channel cannot be marshalled to JSON.
	if err := c.respond(nil, make(chan int)); err == nil {
		t.Error("respond(chan) succeeded, want a marshal error")
	}
}

func TestW4NotifyMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.notify("zz/method", make(chan int)); err == nil {
		t.Error("notify(chan params) succeeded, want a marshal error")
	}
}
