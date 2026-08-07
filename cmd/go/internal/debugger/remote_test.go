package debugger

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/debugserve"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// stepServer runs src under a Session fronted by a Remote mounted on an
// httptest server, returning the client, the Remote, the program-output
// buffer, and a cleanup that shuts the pair down.
func stepServer(t *testing.T, src string) (*debugserve.Client, *Remote, *bytes.Buffer, func()) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	var progOut bytes.Buffer
	reg.Output = &progOut
	sess := New(reg, Config{
		In: strings.NewReader(""), Out: io.Discard,
		File: "remote.boru", Source: src,
	})
	rm := NewRemote(sess)
	toks, perr := parser.Parse(src)
	if perr != nil {
		t.Fatal(perr)
	}
	go func() {
		_, rerr := sess.RunProgram(toks, src)
		rm.Finish(rerr)
	}()
	mux := http.NewServeMux()
	passthrough := func(h http.HandlerFunc) http.HandlerFunc { return h }
	rm.Register(mux, passthrough)
	ts := httptest.NewServer(mux)
	c := debugserve.NewClient(ts.URL, "")
	return c, rm, &progOut, func() {
		rm.Shutdown()
		ts.Close()
	}
}

// waitPaused polls until the remote reports a pause with seq > after.
func waitPaused(t *testing.T, c *debugserve.Client, after int) debugserve.StepState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := c.StepPause()
		if err == nil && ((st.State == "paused" && st.Seq > after) || st.State == "exited") {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("no pause after seq %d (last: %+v, err %v)", after, st, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRemoteFullSession(t *testing.T) {
	c, _, progOut, done := stepServer(t, "def n 41\nprint \"zz-remote\"\nn add 1\n")
	defer done()

	// The program parks at the entry stop before any attach arrives.
	st := waitPaused(t, c, 0)
	if st.State != "paused" || st.Row != 1 || st.Kind != "step" ||
		st.File != "remote.boru" || st.Line != "def n 41" {
		t.Fatalf("entry stop = %+v", st)
	}

	// A breakpoint set while paused, then continue to it.
	if res, err := c.StepBreak("3", false); err != nil || res != "breakpoint set: line 3" {
		t.Fatalf("break = %q (%v)", res, err)
	}
	act, err := c.StepAction("continue")
	if err != nil {
		t.Fatalf("action error: %v", err)
	}
	switch act.State {
	case "running":
		st = waitPaused(t, c, st.Seq)
	case "paused":
		// On a loaded machine the engine can hit the breakpoint before
		// the continue ack is composed, so the ack already carries the
		// pause — same protocol outcome, merged into one response.
		st = act
	default:
		t.Fatalf("action = %+v", act)
	}
	if st.Detail != "breakpoint: line 3" || st.Row != 3 || st.Stack != "[]" {
		t.Fatalf("bp stop = %+v", st)
	}

	// eval at the pause: sees the program's defs, hook suppressed.
	if res, evalErr, eerr := c.Eval("n add 9"); eerr != nil || evalErr != "" || res != "50" {
		t.Fatalf("eval = %q / %q (%v)", res, evalErr, eerr)
	}

	// Breakpoint management: delete, then a bad action refused.
	if res, err := c.StepBreak("3", true); err != nil || res != "deleted: line 3" {
		t.Fatalf("delete = %q (%v)", res, err)
	}
	if _, err := c.StepAction("zzap"); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("bad action must refuse; got %v", err)
	}

	// step from line 3 runs the last line; the program exits cleanly.
	if _, err := c.StepAction("step"); err != nil {
		t.Fatal(err)
	}
	st = waitPaused(t, c, st.Seq)
	if st.State != "exited" || st.Exit != "" {
		t.Fatalf("end state = %+v", st)
	}

	// After exit: actions refuse; eval still works (post-run state).
	if _, err := c.StepAction("step"); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("post-exit action must refuse; got %v", err)
	}
	if res, _, err := c.Eval("n"); err != nil || res != "41" {
		t.Fatalf("post-exit eval = %q (%v)", res, err)
	}
	// Breakpoint edits after exit take the quiet path harmlessly.
	if res, err := c.StepBreak("add", false); err != nil || res != "breakpoint set: word add" {
		t.Fatalf("post-exit break = %q (%v)", res, err)
	}
	if !strings.Contains(progOut.String(), "zz-remote") {
		t.Errorf("program output missing: %q", progOut.String())
	}
}

func TestRemoteProgramErrorReported(t *testing.T) {
	c, rm, _, done := stepServer(t, "1 div 0\n")
	defer done()
	st := waitPaused(t, c, 0)
	if _, err := c.StepAction("continue"); err != nil {
		t.Fatal(err)
	}
	st = waitPaused(t, c, st.Seq)
	if st.State != "exited" || !strings.Contains(st.Exit, "division by zero") {
		t.Fatalf("end state = %+v", st)
	}
	if !strings.Contains(rm.ExitError(), "division by zero") {
		t.Errorf("ExitError = %q", rm.ExitError())
	}
}

func TestRemoteShutdownDrainsParkedProgram(t *testing.T) {
	// Shutdown with the engine parked delivers quit (detach): the
	// program drains — its side effects complete — before return.
	c, rm, progOut, _ := stepServer(t, "print \"zz-drain-remote\"\n")
	waitPaused(t, c, 0)
	rm.Shutdown()
	if !strings.Contains(progOut.String(), "zz-drain-remote") {
		t.Errorf("the detached program must drain; out = %q", progOut.String())
	}
	// A second Shutdown (post-exit) returns immediately — the deliver
	// arm is skipped.
	rm.Shutdown()
}

func TestRemoteRefusalArms(t *testing.T) {
	c, rm, _, done := stepServer(t, "1 add 2\n")
	defer done()
	waitPaused(t, c, 0)

	// A busy adapter refuses actions, evals, and quiet break edits.
	rm.mu.Lock()
	rm.busy = true
	rm.mu.Unlock()
	if _, err := c.StepAction("step"); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("busy action = %v", err)
	}
	if _, _, err := c.Eval("1"); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("busy eval = %v", err)
	}
	// A break while busy AND parked cannot TryLock (the parked engine
	// holds the session lock): the retry refusal answers.
	if _, err := c.StepBreak("2", false); err == nil || !strings.Contains(err.Error(), "retry") {
		t.Errorf("busy parked break = %v", err)
	}
	rm.mu.Lock()
	rm.busy = false
	rm.mu.Unlock()

	// A missing break spec is a bad request.
	if _, err := c.StepBreak("", false); err == nil || !strings.Contains(err.Error(), "missing breakpoint spec") {
		t.Errorf("empty spec = %v", err)
	}
	// Deleting an absent breakpoint answers through the same channel.
	if res, err := c.StepBreak("99", true); err != nil || res != "no breakpoint at line 99" {
		t.Errorf("absent delete = %q (%v)", res, err)
	}
}

func TestRemoteRunningRefusals(t *testing.T) {
	// While the program RUNS (not paused, not exited), actions and
	// evals refuse rather than blocking behind the engine. Simulated
	// directly: a fresh Remote is in the running state until its first
	// pause.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	sess := New(reg, Config{In: strings.NewReader(""), Out: io.Discard})
	rm := NewRemote(sess)
	mux := http.NewServeMux()
	rm.Register(mux, func(h http.HandlerFunc) http.HandlerFunc { return h })
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := debugserve.NewClient(ts.URL, "")

	if st, err := c.StepPause(); err != nil || st.State != "running" {
		t.Fatalf("pause = %+v (%v)", st, err)
	}
	if _, err := c.StepAction("step"); err == nil || !strings.Contains(err.Error(), "not paused") {
		t.Errorf("running action = %v", err)
	}
	if _, _, err := c.Eval("1"); err == nil || !strings.Contains(err.Error(), "not paused") {
		t.Errorf("running eval = %v", err)
	}
	// A break while running mutates under the session lock (free here).
	if res, err := c.StepBreak("7", false); err != nil || res != "breakpoint set: line 7" {
		t.Errorf("running break = %q (%v)", res, err)
	}
}
