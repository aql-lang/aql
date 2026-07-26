package debugger

import (
	"bytes"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
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
}
