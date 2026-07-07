package tui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestS7n_RunUsesDiscoveryFileForAPIAndToken drives the `aql tui`
// Run function's "apiURL empty → discovery fills apiURL, and token
// empty → discovery fills token too" arms (tui.go). The analogous
// arms in Server.Start are already covered by
// TestServerStartUsesDiscoveryFile; Run's own copy was untouched
// because every existing Run test either passes --api explicitly or
// has no discovery file at all.
func TestS7n_RunUsesDiscoveryFileForAPIAndToken(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	setDiscovery(t, ts.URL, "disc-tok")

	var out bytes.Buffer
	code := New().Run(nil, strings.NewReader("q"), &out, io.Discard)
	if code != 0 {
		t.Errorf("Run() = %d, want 0", code)
	}
}

// failReader fails every Read with a non-EOF error, to drive
// bubbletea's input error path.
type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("boom-stdin-read") }

// TestS7n_RunTeaProgramError drives Run's "prog.Run() returned an
// error" arm (tui.go) by giving bubbletea a stdin whose Read always
// fails: bubbletea's cancelreader surfaces that as a program error
// ("program was killed: error reading input: ..."), which Run must
// report and exit 1 for.
func TestS7n_RunTeaProgramError(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)

	var out, stderr bytes.Buffer
	code := New().Run([]string{"--api", ts.URL}, failReader{}, &out, &stderr)
	if code != 1 {
		t.Errorf("Run() = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "tui:") {
		t.Errorf("stderr = %q, want the tui: error prefix", stderr.String())
	}
}
