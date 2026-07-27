package debugger

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// Framing-layer arms the end-to-end DAP sessions cannot reach: header
// and body malformations must error the reader (closing the session),
// never panic or hang.

func TestReadDAPMessageArms(t *testing.T) {
	read := func(s string) error {
		_, err := readDAPMessage(bufio.NewReader(strings.NewReader(s)))
		return err
	}
	if err := read("Content-Length: 2\r\n\r\n{}"); err != nil {
		t.Errorf("well-formed frame: %v", err)
	}
	if err := read("Content-Length: zz\r\n\r\n{}"); err == nil ||
		!strings.Contains(err.Error(), "bad Content-Length") {
		t.Errorf("bad length must error; got %v", err)
	}
	if err := read("X-Other: 1\r\n\r\n{}"); err == nil ||
		!strings.Contains(err.Error(), "missing Content-Length") {
		t.Errorf("missing header must error; got %v", err)
	}
	if err := read("Content-Length: 99\r\n\r\n{}"); err == nil {
		t.Error("a short body must error")
	}
	if err := read("Content-Length: 7\r\n\r\nnotjson"); err == nil {
		t.Error("a non-JSON body must error")
	}
	if err := read(""); err == nil {
		t.Error("EOF must error")
	}
}

func TestDAPWriteMarshalFailureIsDropped(t *testing.T) {
	// A body json.Marshal cannot encode is dropped rather than panicking
	// or corrupting the frame stream (defensive; reachable only via a
	// programming error).
	var buf bytes.Buffer
	a := &dapAdapter{w: bufio.NewWriter(&buf)}
	a.event("bad", map[string]any{"ch": make(chan int)})
	if buf.Len() != 0 {
		t.Errorf("an unmarshalable body must emit nothing; got %q", buf.String())
	}
	a.event("ok", map[string]any{"n": 1})
	if !strings.Contains(buf.String(), `"event":"ok"`) {
		t.Errorf("the stream must survive; got %q", buf.String())
	}
}

func TestHistoryRendersBodyEntries(t *testing.T) {
	// A history entry recorded inside a body evaluation carries the
	// "(in body)" label in both the view and the list.
	s, buf := testSession(t, "")
	s.recordHistory(histEntry{row: 2, sub: true})
	s.recordHistory(histEntry{row: 3})
	s.browse = 1
	s.renderHistoryView()
	if !strings.Contains(buf.String(), "(in body)") {
		t.Errorf("view must label body entries; out = %q", buf.String())
	}
	buf.Reset()
	s.renderHistoryList()
	if !strings.Contains(buf.String(), "(in body)") {
		t.Errorf("list must label body entries; out = %q", buf.String())
	}
}

func TestStopReasonTable(t *testing.T) {
	cases := map[string]string{
		"breakpoint": "breakpoint", "marker": "breakpoint",
		"watch": "data breakpoint", "fault": "exception",
		"step": "step", "inner-step": "step",
	}
	for kind, want := range cases {
		if got := stopReason(kind); got != want {
			t.Errorf("stopReason(%q) = %q, want %q", kind, got, want)
		}
	}
}
