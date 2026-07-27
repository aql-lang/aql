package debugger

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
)

// The end-to-end debugger transcripts live with the CLI entry
// (debugcmd's launch tests); this file unit-tests the session's
// defensive arms and pure helpers directly — the paths a healthy
// end-to-end run cannot reach.

func testSession(t *testing.T, in string) (*Session, *bytes.Buffer) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	return New(reg, Config{
		In:     strings.NewReader(in),
		Out:    &buf,
		File:   "unit.aql",
		Source: "line one\nline two\nline three",
	}), &buf
}

func TestControllerIsSelf(t *testing.T) {
	s, _ := testSession(t, "")
	if s.Controller() != s {
		t.Error("the session must be its own step controller")
	}
}

func TestPromptEOFDetaches(t *testing.T) {
	s, buf := testSession(t, "")
	if got := s.prompt(); got != modeDetached {
		t.Errorf("prompt on EOF = %v, want detach", got)
	}
	if !strings.Contains(buf.String(), detachNotice) {
		t.Errorf("EOF must print the detach notice; out = %q", buf.String())
	}
	// Negative: once detached, the trace never pauses again.
	before := buf.Len()
	s.onTrace(0, 0, nil, "")
	if buf.Len() != before {
		t.Error("a detached session must not render trace pauses")
	}
}

func TestPostMortemGuards(t *testing.T) {
	// A detached session gets no post-mortem — the user already left.
	s, buf := testSession(t, "q\n")
	s.mode = modeDetached
	s.PostMortem(errors.New("boom"))
	if buf.Len() != 0 {
		t.Errorf("detached PostMortem must render nothing; out = %q", buf.String())
	}
	// With no retained snapshot the banner still renders, locationless.
	s2, buf2 := testSession(t, "q\n")
	s2.PostMortem(errors.New("boom-2"))
	out := buf2.String()
	if !strings.Contains(out, "post-mortem\n") || !strings.Contains(out, "boom-2") {
		t.Errorf("out = %q", out)
	}
	if !strings.Contains(out, "(post-mortem closed)") {
		t.Errorf("quit at a post-mortem must not claim the program continues; out = %q", out)
	}
}

func TestDataStackSnapshotFallback(t *testing.T) {
	// With no engine running and no snapshot, there is nothing to show.
	s, _ := testSession(t, "")
	if _, ok := s.dataStack(); ok {
		t.Error("a session that never ran has no data stack")
	}
	// A retained snapshot reconstructs the resolved region, markers
	// filtered, and clamps a pointer beyond the snapshot.
	s.curStack = []native.Value{
		native.NewInteger(6), eng.NewOpenParen(), native.NewInteger(0),
	}
	s.curPointer = 99
	vals, ok := s.dataStack()
	if !ok || len(vals) != 2 {
		t.Fatalf("got (%v, %v), want the 2 data values", vals, ok)
	}
}

// errReader always fails, driving the Scanner's error (not-EOF) arm.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom-read") }

func TestPromptInputErrorNamesItself(t *testing.T) {
	// Negative pair for the EOF detach: a read FAILURE must say so before
	// detaching, not masquerade as a clean end of input.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	s := New(reg, Config{In: errReader{}, Out: &buf})
	if got := s.prompt(); got != modeDetached {
		t.Errorf("prompt on input error = %v, want detach", got)
	}
	if !strings.Contains(buf.String(), "command input error: boom-read") {
		t.Errorf("the failure must be named; out = %q", buf.String())
	}
}

func TestOnStepContendedContinues(t *testing.T) {
	// A pause delivered while another goroutine holds the session (at the
	// prompt, or mid-pause) must continue immediately — exactly one
	// goroutine may ever read the command input.
	s, buf := testSession(t, "")
	s.mu.Lock()
	defer s.mu.Unlock()
	if got := s.OnStep(capabilities.StepFrame{AtBreak: true}); got != capabilities.StepContinue {
		t.Errorf("contended OnStep = %v, want StepContinue", got)
	}
	if buf.Len() != 0 {
		t.Errorf("a contended pause must render nothing; out = %q", buf.String())
	}
}

func TestRunProgramRestoresRegistry(t *testing.T) {
	// The session BORROWS the registry: after RunProgram the prior
	// DebugOps (none, or a pre-installed host controller) and Source are
	// restored, so a reused registry doesn't pause into a dead session.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	reg.Source = "prior-source"
	toks, perr := parser.Parse("1 add 2")
	if perr != nil {
		t.Fatal(perr)
	}
	var buf bytes.Buffer
	s := New(reg, Config{In: strings.NewReader(""), Out: &buf, Source: "1 add 2"})
	if _, rerr := s.RunProgram(toks, "1 add 2"); rerr != nil {
		t.Fatal(rerr)
	}
	if _, ok := native.EffectiveDebugOps(reg); ok {
		t.Error("no prior DebugOps: the slot must be empty again after the run")
	}
	if reg.Source != "prior-source" {
		t.Errorf("Source must be restored; got %q", reg.Source)
	}
	// With a PRIOR controller installed, it is what must come back.
	prior := &fakeOps{}
	native.SetHostDebugOps(reg, prior)
	s2 := New(reg, Config{In: strings.NewReader(""), Out: &buf, Source: "1 add 2"})
	if _, rerr := s2.RunProgram(toks, "1 add 2"); rerr != nil {
		t.Fatal(rerr)
	}
	if got, ok := native.EffectiveDebugOps(reg); !ok || got != capabilities.DebugOps(prior) {
		t.Error("a pre-installed DebugOps must be restored after the run")
	}
	// RemoveHostDebugOps nil-guard (panic prevention).
	native.RemoveHostDebugOps(nil)
}

// fakeOps is a stand-in prior DebugOps for the restore test.
type fakeOps struct{}

func (f *fakeOps) Controller() capabilities.StepController { return nil }

func TestOnStepDetachedQuitsInnerSteppers(t *testing.T) {
	s, buf := testSession(t, "")
	s.mode = modeDetached
	if got := s.OnStep(capabilities.StepFrame{}); got != capabilities.StepQuit {
		t.Errorf("detached OnStep = %v, want StepQuit", got)
	}
	if buf.Len() != 0 {
		t.Errorf("detached OnStep must render nothing; out = %q", buf.String())
	}
}

func TestOnStepInPromptNeverNests(t *testing.T) {
	s, buf := testSession(t, "")
	s.inPrompt = true
	if got := s.OnStep(capabilities.StepFrame{AtBreak: true}); got != capabilities.StepContinue {
		t.Errorf("in-prompt OnStep = %v, want StepContinue (no nesting)", got)
	}
	if buf.Len() != 0 {
		t.Errorf("in-prompt OnStep must render nothing; out = %q", buf.String())
	}
}

func TestOnTraceInPromptOnlyRecords(t *testing.T) {
	s, buf := testSession(t, "")
	s.inPrompt = true
	s.onTrace(0, 0, []native.Value{native.NewInteger(1)}, "")
	if buf.Len() != 0 {
		t.Errorf("an in-prompt trace fire must not pause; out = %q", buf.String())
	}
	if len(s.curStack) != 1 {
		t.Error("the trace fire must still record the pause context")
	}
}

func TestFrameLoc(t *testing.T) {
	if got := frameLoc(capabilities.StepFrame{}); got != "" {
		t.Errorf("Row 0 renders no location, got %q", got)
	}
	if got := frameLoc(capabilities.StepFrame{Row: 3}); got != " at ?:3" {
		t.Errorf("empty File renders ?, got %q", got)
	}
	if got := frameLoc(capabilities.StepFrame{Row: 3, File: "x.aql"}); got != " at x.aql:3" {
		t.Errorf("got %q", got)
	}
}

func TestSplitCommand(t *testing.T) {
	if c, a := splitCommand("print 1 add 2"); c != "print" || a != "1 add 2" {
		t.Errorf("got (%q, %q)", c, a)
	}
	if c, a := splitCommand("step"); c != "step" || a != "" {
		t.Errorf("got (%q, %q)", c, a)
	}
}

func TestSummarizeStack(t *testing.T) {
	short := []native.Value{native.NewInteger(1), native.NewInteger(2)}
	if got := summarizeStack(short); got != "[1 2]" {
		t.Errorf("got %q", got)
	}
	long := make([]native.Value, maxStackShown+2)
	for i := range long {
		long[i] = native.NewInteger(int64(i))
	}
	got := summarizeStack(long)
	if !strings.Contains(got, "… 2 more") {
		t.Errorf("a deep stack must truncate with a count; got %q", got)
	}
	// Negative: the truncated render keeps the TOP entries, not the bottom.
	if strings.Contains(got, "[0 ") {
		t.Errorf("the bottom of a deep stack must be elided; got %q", got)
	}
	// Boundary: exactly maxStackShown renders in full, no ellipsis.
	exact := make([]native.Value, maxStackShown)
	for i := range exact {
		exact[i] = native.NewInteger(int64(i))
	}
	if got := summarizeStack(exact); got != "[0 1 2 3 4 5 6 7]" {
		t.Errorf("a stack of exactly maxStackShown renders in full; got %q", got)
	}
}

func TestShowStackOutsideRunRendersNothing(t *testing.T) {
	s, buf := testSession(t, "")
	s.showStack() // no engine running → CurrentStack reports none
	if buf.Len() != 0 {
		t.Errorf("no live stack must render nothing; out = %q", buf.String())
	}
}

func TestRenderFullStackOutsideRun(t *testing.T) {
	s, buf := testSession(t, "")
	s.renderFullStack()
	if !strings.Contains(buf.String(), "(stack empty)") {
		t.Errorf("out = %q", buf.String())
	}
}

func TestRenderDefs(t *testing.T) {
	// With no launch baseline, every binding is "new since launch": the
	// pushed def shows under the default program-only filter.
	s, buf := testSession(t, "")
	s.reg.Defs.Push("zz-unit", native.NewInteger(7))
	s.renderDefs(false)
	out := buf.String()
	if !strings.Contains(out, "defs (") || !strings.Contains(out, "zz-unit = 7") {
		t.Errorf("out = %q", out)
	}
	// Negative: with the baseline snapshotting that binding, the filtered
	// view hides it — only `all` shows it again.
	s.baseDefs = s.reg.Defs.Snapshot()
	buf.Reset()
	s.renderDefs(false)
	if strings.Contains(buf.String(), "zz-unit") {
		t.Errorf("a baseline binding must be filtered; out = %q", buf.String())
	}
	buf.Reset()
	s.renderDefs(true)
	if !strings.Contains(buf.String(), "zz-unit = 7") {
		t.Errorf("defs all must show baseline bindings; out = %q", buf.String())
	}
	// DEEPENED past the baseline (a shadow) is program state and shows
	// under the bare filter — the "or deepened" half of the contract.
	s.reg.Defs.Push("zz-unit", native.NewInteger(8))
	buf.Reset()
	s.renderDefs(false)
	if !strings.Contains(buf.String(), "zz-unit = 8") {
		t.Errorf("a shadow deepened past the baseline must show; out = %q", buf.String())
	}
}

func TestEvalExprArms(t *testing.T) {
	// No parser configured (a bare registry): the guard reports, not panics.
	s, buf := testSession(t, "")
	s.evalExpr("1 add 2")
	if !strings.Contains(buf.String(), "no parser configured") {
		t.Errorf("out = %q", buf.String())
	}
	// Empty expression: usage.
	buf.Reset()
	s.evalExpr("")
	if !strings.Contains(buf.String(), "usage: print") {
		t.Errorf("out = %q", buf.String())
	}
	// An expression that leaves nothing on the stack reports "(no result)"
	// rather than printing an empty line.
	s.reg.SetParseFunc(parser.Parse)
	buf.Reset()
	s.evalExpr("def zz-eval 9")
	if !strings.Contains(buf.String(), "(no result)") {
		t.Errorf("out = %q", buf.String())
	}
	// A leaked flow signal is discarded with a notice — and the session
	// still evaluates normally afterwards (FlowCtrl restored).
	buf.Reset()
	s.evalExpr("break")
	if !strings.Contains(buf.String(), "discarded") {
		t.Errorf("a loop-less break must be discarded with a notice; out = %q", buf.String())
	}
	buf.Reset()
	s.evalExpr("1 add 2")
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("evaluation must recover after a discarded signal; out = %q", buf.String())
	}
}

func TestEvalExprHonorsGrants(t *testing.T) {
	// §8.1/§9: an eval can never exceed the program's grants. With the
	// fileops capability uninstalled, a read attempted from the prompt is
	// refused, not silently satisfied.
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	native.SetHostFileOps(reg, nil)
	var buf bytes.Buffer
	reg.Output = &buf
	s := New(reg, Config{In: strings.NewReader(""), Out: &buf})
	s.evalExpr(`import "aql:io" IO.read (make Pathon "/zz-no-such-file")`)
	if !strings.Contains(buf.String(), "error:") ||
		!strings.Contains(buf.String(), "capability") {
		t.Errorf("an ungranted read must be refused as a capability error; out = %q", buf.String())
	}
}

func TestRenderListUnknownRow(t *testing.T) {
	s, buf := testSession(t, "")
	s.curRow = 0
	s.renderList()
	if !strings.Contains(buf.String(), "source line unknown") {
		t.Errorf("out = %q", buf.String())
	}
	// Negative: a row past the source is unknown too, never a panic.
	buf.Reset()
	s.curRow = 99
	s.renderList()
	if !strings.Contains(buf.String(), "source line unknown") {
		t.Errorf("out = %q", buf.String())
	}
}

func TestSourceLineOutOfRange(t *testing.T) {
	s, _ := testSession(t, "")
	if got := s.sourceLine(2); got != "line two" {
		t.Errorf("got %q", got)
	}
	if got := s.sourceLine(0); got != "" {
		t.Errorf("row 0 must render empty, got %q", got)
	}
	if got := s.sourceLine(999); got != "" {
		t.Errorf("an out-of-range row must render empty, got %q", got)
	}
}

// ── liveFrames ──────────────────────────────────────────────────────────

func TestLiveFramesEdges(t *testing.T) {
	meta := &eng.FnFrameMeta{Name: "f", InstallNames: []string{"x"}}
	openF := eng.NewFrameOpen(meta)
	closeP := eng.NewCloseParen()
	val := native.NewInteger(1)

	// A frame whose close paren sits before the pointer is not live.
	if got := liveFrames([]native.Value{openF, closeP, val}, 3); len(got) != 0 {
		t.Errorf("a closed frame must not appear; got %v", got)
	}
	// A frame with no close paren yet IS live.
	got := liveFrames([]native.Value{openF, val}, 2)
	if len(got) != 1 || got[0].name != "f" || len(got[0].installs) != 1 {
		t.Errorf("got %v", got)
	}
	// A pointer beyond the snapshot is clamped, not a panic.
	if got := liveFrames([]native.Value{openF, val}, 99); len(got) != 1 {
		t.Errorf("clamped scan must still find the live frame; got %v", got)
	}
	// Negative: a meta-less frame open (defensive) contributes nothing.
	if got := liveFrames([]native.Value{eng.NewFrameOpen(nil)}, 1); len(got) != 0 {
		t.Errorf("a nil-meta frame must be skipped; got %v", got)
	}
	// Negative: a plain grouping paren is not a frame.
	if got := liveFrames([]native.Value{eng.NewOpenParen(), val}, 2); len(got) != 0 {
		t.Errorf("a plain paren must not appear; got %v", got)
	}
	// Nested parens: the frame's close is the DEPTH-MATCHED one — an inner
	// group's close must not be mistaken for it.
	nested := []native.Value{
		openF, eng.NewOpenParen(), val, closeP, val, eng.NewCloseParen(),
	}
	if got := liveFrames(nested, 5); len(got) != 1 || got[0].name != "f" {
		t.Errorf("inner close must not close the frame; got %v", got)
	}
	// …and once the pointer passes the frame's true close, it is dead.
	if got := liveFrames(nested, 6); len(got) != 0 {
		t.Errorf("a frame closed before the pointer is not live; got %v", got)
	}
}
