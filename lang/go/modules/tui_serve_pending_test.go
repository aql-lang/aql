package modules

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// The accept/broadcast race (CI failure on f2e95c4, PR #410):
// tuiWriteAccept was called after admit had released the hub lock, and
// setTitle/broadcastFrame take that same lock and write to every connection in
// h.viewers. So a concurrent broadcast could reach a freshly published socket
// before the accept landed, and the client's next line was `title` or `frame`
// where the protocol requires `accept`.
//
// The race itself is timing-dependent — 40 local runs and -race were green,
// and only CI contention surfaced it — so these tests pin the MECHANISM that
// closes it rather than trying to lose the race on demand. A timing test that
// passes 40 times and fails on the 41st is not a gate.

// readLines drains conn into a channel, one line per send.
func readLines(t *testing.T, conn net.Conn, n int) chan string {
	t.Helper()
	lines := make(chan string, n)
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	return lines
}

// A PENDING viewer receives nothing at all: not a frame, not a title. It holds
// its slot and its reader token — it is in the session — but the wire stays
// silent until its accept has landed.
func TestPendingViewerReceivesNoBroadcast(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	client, server := net.Pipe()
	defer client.Close()
	lines := readLines(t, client, 8)

	id, ok := hub.admitPending(server)
	if !ok {
		t.Fatal("admitPending refused")
	}
	// Both broadcast paths, while the viewer is pending. Neither may write:
	// if either did, net.Pipe is unbuffered and the write would block until
	// the deadline, so a regression shows up as a dropped viewer too.
	hub.setTitle("served")
	if !hub.broadcastFrame([]byte(`{"w":"text","text":"x"}`)) {
		t.Fatal("a pending viewer still holds the session open")
	}
	select {
	case got := <-lines:
		t.Fatalf("a pending viewer received %q before its accept", got)
	default:
	}

	// Promotion delivers the chrome it missed, title first, then the frame —
	// the hello -> accept -> chrome order the protocol requires.
	hub.promote(id)
	first := <-lines
	if !strings.Contains(first, `"tag":"title"`) || !strings.Contains(first, `"text":"served"`) {
		t.Fatalf("first line after promote = %s, want the title", first)
	}
	second := <-lines
	if !strings.Contains(second, `"tag":"frame"`) || !strings.Contains(second, `"text":"x"`) {
		t.Fatalf("second line after promote = %s, want the frame", second)
	}

	// And once promoted it is an ordinary viewer.
	if !hub.broadcastFrame([]byte(`{"w":"text","text":"y"}`)) {
		t.Fatal("session closed after promote")
	}
	if got := <-lines; !strings.Contains(got, `"text":"y"`) {
		t.Fatalf("promoted viewer missed a live frame: %s", got)
	}
	if len(hub.pending) != 0 {
		t.Errorf("pending not cleared: %v", hub.pending)
	}
}

// A pending viewer occupies a slot — it is admitted, so it counts against max
// exactly as a live viewer does. Otherwise a burst of connections could admit
// past `viewers:` while each waits for its accept.
func TestPendingViewerHoldsItsSlot(t *testing.T) {
	hub := newTuiViewerHub(1, false)
	c1, s1 := net.Pipe()
	defer c1.Close()
	if _, ok := hub.admitPending(s1); !ok {
		t.Fatal("first admitPending refused")
	}
	c2, s2 := net.Pipe()
	defer c2.Close()
	if _, ok := hub.admit(s2); ok {
		t.Error("a pending viewer must still fill the session")
	}
}

// goodbye closes a pending viewer's connection but does NOT send it the quit
// line: `quit` before `accept` is the same protocol violation a frame would be.
// The client sees EOF, which is the honest end of a connection that never
// completed its handshake.
func TestGoodbyeSkipsQuitForPendingViewer(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	cp, sp := net.Pipe()
	defer cp.Close()
	pendingLines := readLines(t, cp, 4)
	if _, ok := hub.admitPending(sp); !ok {
		t.Fatal("admitPending refused")
	}

	cl, sl := net.Pipe()
	defer cl.Close()
	liveLines := readLines(t, cl, 4)
	if _, ok := hub.admit(sl); !ok {
		t.Fatal("admit refused")
	}

	hub.goodbye()

	// The live viewer is told; the pending one just sees the socket close.
	if got := <-liveLines; !strings.Contains(got, `"tag":"quit"`) {
		t.Fatalf("live viewer got %s, want quit", got)
	}
	if got, open := <-pendingLines; open {
		t.Fatalf("pending viewer received %q instead of a close", got)
	}
	if len(hub.pending) != 0 {
		t.Errorf("goodbye left pending entries: %v", hub.pending)
	}
}

// evict — the accept-write-failed path — clears the pending mark along with
// the viewer, so a later id reuse cannot inherit a stale "do not broadcast".
func TestEvictClearsPending(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	c, s := net.Pipe()
	defer c.Close()
	id, ok := hub.admitPending(s)
	if !ok {
		t.Fatal("admitPending refused")
	}
	hub.evict(id)
	if len(hub.pending) != 0 {
		t.Errorf("evict left pending entries: %v", hub.pending)
	}
	hub.promote(id) // an evicted id promotes nothing: the no-op arm
}

// drop — the read-side EOF path — does the same.
func TestDropClearsPending(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	c, s := net.Pipe()
	defer c.Close()
	id, ok := hub.admitPending(s)
	if !ok {
		t.Fatal("admitPending refused")
	}
	hub.drop(id)
	if len(hub.pending) != 0 {
		t.Errorf("drop left pending entries: %v", hub.pending)
	}
}
