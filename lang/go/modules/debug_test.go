package modules

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
)

// debugRegistry builds a registry with aql:debug installed and its Output
// captured into the returned buffer so print/trace assertions can inspect
// what reached the host writer.
func debugRegistry(t *testing.T) (*native.Registry, *bytes.Buffer) {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	var buf bytes.Buffer
	r.Output = &buf
	if err := InstallDebugExports(r); err != nil {
		t.Fatal(err)
	}
	return r, &buf
}

func runDebug(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	result, err := native.NewTop(r).Run(values)
	if err != nil {
		t.Fatalf("run error for %q: %v", src, err)
	}
	return result
}

func runDebugErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = native.NewTop(r).Run(values)
	return err
}

func topInt(t *testing.T, res []native.Value) int64 {
	t.Helper()
	if len(res) == 0 {
		t.Fatal("empty result, wanted an integer")
	}
	n, err := native.AsInteger(res[len(res)-1])
	if err != nil {
		t.Fatalf("top of stack not an integer: %v", err)
	}
	return n
}

// TestDebugModuleExports pins the public surface.
func TestDebugModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildDebugModule(r)
	if err != nil {
		t.Fatal(err)
	}
	exp, ok := desc.Exports["Debug"]
	if !ok {
		t.Fatal("expected 'Debug' export namespace")
	}
	want := []string{
		"tap", "label", "dump", "assert", "todo",
		"parse", "deps", "explain", "sig", "body", "watch", "disasm",
		"words", "defs", "modules", "stack",
		"sizeof", "shape", "heap", "gc",
		"steps", "time", "bench", "trace", "profile",
		"step", "break", "break-when", "run-stepped",
		"widget", "dashboard",
	}
	for _, name := range want {
		if _, ok := exp.Get(name); !ok {
			t.Errorf("missing export: Debug.%s", name)
		}
	}
}

// ── (A) Printing ─────────────────────────────────────────────────────

func TestDebugTapReturnsAndPrints(t *testing.T) {
	r, buf := debugRegistry(t)
	res := runDebug(t, r, "42 Debug.tap")
	if got := topInt(t, res); got != 42 {
		t.Errorf("tap must pass the value through unchanged: got %d, want 42", got)
	}
	if !strings.Contains(buf.String(), "42") {
		t.Errorf("tap must print the value; output = %q", buf.String())
	}
}

func TestDebugTapComposesInPipeline(t *testing.T) {
	r, _ := debugRegistry(t)
	// The point of tap: it can sit mid-pipeline because it returns its arg.
	res := runDebug(t, r, "(10 Debug.tap) 5 add")
	if got := topInt(t, res); got != 15 {
		t.Errorf("tap mid-pipeline: got %d, want 15", got)
	}
}

func TestDebugLabelPrintsLabelAndReturns(t *testing.T) {
	r, buf := debugRegistry(t)
	res := runDebug(t, r, `7 "answer" Debug.label`)
	if got := topInt(t, res); got != 7 {
		t.Errorf("label must return the value: got %d, want 7", got)
	}
	if !strings.Contains(buf.String(), "answer: 7") {
		t.Errorf("label must print 'answer: 7'; output = %q", buf.String())
	}
}

// TestDebugLabelPipelineForm covers the intended labelled-tap use: a value
// produced by a pipeline (on the stack) is tapped with a literal label and
// flows through unchanged (PR #190 review — the pipeline form must work).
func TestDebugLabelPipelineForm(t *testing.T) {
	r, buf := debugRegistry(t)
	res := runDebug(t, r, `(5 add 5) "sum" Debug.label`)
	if got := topInt(t, res); got != 10 {
		t.Errorf("labelled tap must pass the computed value through: got %d, want 10", got)
	}
	if !strings.Contains(buf.String(), "sum: 10") {
		t.Errorf("label must print 'sum: 10'; output = %q", buf.String())
	}
}

func TestDebugAssert(t *testing.T) {
	// Positive: a true assertion is a no-op and does not raise.
	r, _ := debugRegistry(t)
	if err := runDebugErr(t, r, `Debug.assert true "must hold"`); err != nil {
		t.Errorf("assert true must not raise: %v", err)
	}
	// Negative: a false assertion raises assertion_failure with the message.
	r2, _ := debugRegistry(t)
	err := runDebugErr(t, r2, `Debug.assert false "boom"`)
	if err == nil {
		t.Fatal("assert false must raise")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("assert error must carry the message; got %v", err)
	}
}

func TestDebugTodoRaises(t *testing.T) {
	r, _ := debugRegistry(t)
	err := runDebugErr(t, r, `"later" Debug.todo`)
	if err == nil {
		t.Fatal("todo must always raise")
	}
	if !strings.Contains(err.Error(), "later") {
		t.Errorf("todo error must carry the message; got %v", err)
	}
}

// ── (C) Structural ───────────────────────────────────────────────────

func TestDebugParse(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `"1 add 2" Debug.parse`)
	if len(res) == 0 {
		t.Fatal("parse produced no value")
	}
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("parse must return a list, got %v (err %v)", res[len(res)-1], err)
	}
	if lst.Len() != 3 {
		t.Errorf("parse of '1 add 2' should yield 3 tokens, got %d", lst.Len())
	}
}

func TestDebugParseRejectsBadSource(t *testing.T) {
	r, _ := debugRegistry(t)
	if err := runDebugErr(t, r, `"(" Debug.parse`); err == nil {
		t.Error("parse of malformed source must raise parse_error")
	}
}

func TestDebugDeps(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `[1 add 2 mul 3] Debug.deps`)
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("deps must return a list: %v", err)
	}
	var got []string
	for _, v := range lst.Slice() {
		s, _ := native.AsString(v)
		got = append(got, s)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "add") || !strings.Contains(joined, "mul") {
		t.Errorf("deps should include add and mul; got %v", got)
	}
}

func TestDebugExplain(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `"add" Debug.explain`)
	s, err := native.AsString(res[len(res)-1])
	if err != nil {
		t.Fatalf("explain must return a String: %v", err)
	}
	if !strings.Contains(s, "add") {
		t.Errorf("explain(add) should mention add; got %q", s)
	}
}

func TestDebugSig(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `"add" Debug.sig`)
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("sig must return a list: %v", err)
	}
	if lst.Len() == 0 {
		t.Fatal("add must have at least one signature")
	}
	first, _ := native.AsMap(lst.Get(0))
	if first == nil {
		t.Fatal("each signature must be a map")
	}
	if _, ok := first.Get("args"); !ok {
		t.Error("signature missing 'args'")
	}
	if _, ok := first.Get("returns"); !ok {
		t.Error("signature missing 'returns'")
	}
}

func TestDebugSigUnknownWordErrors(t *testing.T) {
	r, _ := debugRegistry(t)
	if err := runDebugErr(t, r, `"no-such-word-xyz" Debug.sig`); err == nil {
		t.Error("sig of an unknown word must raise, not return Any")
	}
}

func TestDebugBody(t *testing.T) {
	r, _ := debugRegistry(t)
	// A native word reports `native`.
	res := runDebug(t, r, `"add" Debug.body`)
	if s, _ := native.AsString(res[len(res)-1]); s == "native" {
		// good — add is native
	} else if a, err := native.AsAtom(res[len(res)-1]); err == nil && a == "native" {
		// also acceptable (atom form)
	} else {
		t.Errorf("body of a native word should be `native`, got %v", res[len(res)-1])
	}
	// An AQL-defined word reports its quoted body list.
	r2, _ := debugRegistry(t)
	res2 := runDebug(t, r2, `def double fn [[x:Integer] [Integer] [x mul 2]]  "double" Debug.body`)
	if lst, err := native.AsList(res2[len(res2)-1]); err != nil || lst.IsNil() {
		t.Errorf("body of a defined word should be its quoted list, got %v", res2[len(res2)-1])
	}
}

func TestDebugWatch(t *testing.T) {
	r, buf := debugRegistry(t)
	res := runDebug(t, r, `def k 42  "k" Debug.watch`)
	if got := topInt(t, res); got != 42 {
		t.Errorf("watch must return the binding: got %d, want 42", got)
	}
	if !strings.Contains(buf.String(), "k = 42") {
		t.Errorf("watch must print the binding; output = %q", buf.String())
	}
}

func TestDebugDisasm(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `[1 add 2] Debug.disasm`)
	s, err := native.AsString(res[len(res)-1])
	if err != nil {
		t.Fatalf("disasm must return a String: %v", err)
	}
	if s == "" {
		t.Error("disasm of a non-trivial body must be non-empty")
	}
}

func TestDebugHeapAndGC(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "Debug.heap")
	m, err := native.AsMap(res[len(res)-1])
	if err != nil || m == nil {
		t.Fatalf("heap must return a map: %v", err)
	}
	for _, key := range []string{"alloc", "total-alloc", "heap-objects", "num-gc"} {
		if _, ok := m.Get(key); !ok {
			t.Errorf("heap result missing key %q", key)
		}
	}
	res2 := runDebug(t, r, "Debug.gc")
	m2, err := native.AsMap(res2[len(res2)-1])
	if err != nil || m2 == nil {
		t.Fatalf("gc must return a map: %v", err)
	}
	if _, ok := m2.Get("alloc-after"); !ok {
		t.Error("gc result missing 'alloc-after'")
	}
}

// ── (B) Interactive stepping ─────────────────────────────────────────

// scriptedController drives stepping headlessly: it records every frame it
// is shown and returns a pre-baked action per call (the last action repeats).
type scriptedController struct {
	actions []capabilities.StepAction
	frames  []capabilities.StepFrame
	calls   int
}

func (c *scriptedController) OnStep(f capabilities.StepFrame) capabilities.StepAction {
	c.frames = append(c.frames, f)
	a := capabilities.StepContinue
	if c.calls < len(c.actions) {
		a = c.actions[c.calls]
	} else if len(c.actions) > 0 {
		a = c.actions[len(c.actions)-1]
	}
	c.calls++
	return a
}

func (c *scriptedController) Controller() capabilities.StepController { return c }

func TestDebugStepDrivesController(t *testing.T) {
	r, _ := debugRegistry(t)
	r.BaseFile = "test.aql"
	// StepInto on every step → the controller is consulted at each engine step.
	ctl := &scriptedController{actions: []capabilities.StepAction{capabilities.StepInto}}
	native.SetHostDebugOps(r, ctl)
	res := runDebug(t, r, "[1 add 2] Debug.step")
	if got := topInt(t, res); got != 3 {
		t.Errorf("step must return the body result 3, got %d", got)
	}
	if ctl.calls == 0 {
		t.Fatal("the controller must be consulted at least once")
	}
	// The body `1 add 2` takes several engine steps; single-step visits each.
	if ctl.calls < 3 {
		t.Errorf("single-step should consult the controller per step; got %d calls", ctl.calls)
	}
	// Source awareness (StepFrame widening): every frame carries the
	// registry's file, and every positioned token of this single-line
	// source pins Row == 1 with a real column — so a Row/Col transposition
	// cannot survive (Col varies per token, Row must not).
	sawRow := false
	for i, f := range ctl.frames {
		if f.File != "test.aql" {
			t.Errorf("frame %d File = %q, want the registry's BaseFile", i, f.File)
		}
		if f.Row == 0 {
			continue // synthetic token — carries no position
		}
		sawRow = true
		if f.Row != 1 {
			t.Errorf("frame %d Row = %d, want 1 (single-line source)", i, f.Row)
		}
		if f.Col < 1 {
			t.Errorf("frame %d Col = %d, want >= 1 for a positioned token", i, f.Col)
		}
	}
	if !sawRow {
		t.Error("at least one stepped frame must carry a 1-based source Row")
	}
}

func TestDebugStepQuitDetaches(t *testing.T) {
	r, _ := debugRegistry(t)
	// Quit on the first frame: no more pauses, but the body still completes.
	ctl := &scriptedController{actions: []capabilities.StepAction{capabilities.StepQuit}}
	native.SetHostDebugOps(r, ctl)
	res := runDebug(t, r, "[1 add 2] Debug.step")
	if got := topInt(t, res); got != 3 {
		t.Errorf("after quit/detach the body still finishes (3), got %d", got)
	}
	if ctl.calls != 1 {
		t.Errorf("quit must stop further pauses; got %d controller calls", ctl.calls)
	}
}

func TestDebugStepNoControllerPrintsTrace(t *testing.T) {
	r, buf := debugRegistry(t)
	// With no DebugOps installed, step falls back to printing each frame.
	res := runDebug(t, r, "[1 add 2] Debug.step")
	if got := topInt(t, res); got != 3 {
		t.Errorf("step (no controller) must still return 3, got %d", got)
	}
	if !strings.Contains(buf.String(), "[") {
		t.Errorf("step with no controller should print frames; output = %q", buf.String())
	}
}

func TestDebugBreakPausesWithController(t *testing.T) {
	r, _ := debugRegistry(t)
	r.BaseFile = "brk.aql"
	ctl := &scriptedController{actions: []capabilities.StepAction{capabilities.StepContinue}}
	native.SetHostDebugOps(r, ctl)
	// A bare Debug.break in ordinary execution pauses once (the controller
	// is consulted) and then resumes; the program result is unaffected.
	res := runDebug(t, r, "Debug.break 41 add 1")
	if got := topInt(t, res); got != 42 {
		t.Errorf("break must not change the result; got %d, want 42", got)
	}
	if ctl.calls != 1 {
		t.Errorf("a single break should pause exactly once; got %d", ctl.calls)
	}
	if len(ctl.frames) == 1 && !ctl.frames[0].AtBreak {
		t.Error("the break frame must be flagged AtBreak")
	}
	// Negative (StepFrame widening): a one-shot break pause fires inside the
	// handler with no tape pointer in hand — it must carry NO source Row (0),
	// while still naming the registry's file.
	if len(ctl.frames) == 1 {
		if ctl.frames[0].Row != 0 || ctl.frames[0].Col != 0 {
			t.Errorf("a one-shot break pause must carry no position; got %d:%d",
				ctl.frames[0].Row, ctl.frames[0].Col)
		}
		if ctl.frames[0].File != "brk.aql" {
			t.Errorf("break frame File = %q, want brk.aql", ctl.frames[0].File)
		}
		if ctl.frames[0].Step != -1 {
			t.Errorf("a one-shot break pause is Step -1, got %d", ctl.frames[0].Step)
		}
	}
}

func TestDebugBreakIsNoOpWithoutController(t *testing.T) {
	r, _ := debugRegistry(t)
	// No DebugOps → break is inert (production-safe), result unchanged.
	res := runDebug(t, r, "Debug.break 41 add 1")
	if got := topInt(t, res); got != 42 {
		t.Errorf("break with no controller must be a pure no-op; got %d", got)
	}
}

func TestDebugBreakWhenConditional(t *testing.T) {
	// break-when false → never pauses.
	r, _ := debugRegistry(t)
	ctlF := &scriptedController{actions: []capabilities.StepAction{capabilities.StepContinue}}
	native.SetHostDebugOps(r, ctlF)
	runDebug(t, r, "Debug.break-when false")
	if ctlF.calls != 0 {
		t.Errorf("break-when false must not pause; got %d calls", ctlF.calls)
	}
	// break-when true → pauses once.
	r2, _ := debugRegistry(t)
	ctlT := &scriptedController{actions: []capabilities.StepAction{capabilities.StepContinue}}
	native.SetHostDebugOps(r2, ctlT)
	runDebug(t, r2, "Debug.break-when true")
	if ctlT.calls != 1 {
		t.Errorf("break-when true must pause once; got %d calls", ctlT.calls)
	}
}

func TestDebugRunStepped(t *testing.T) {
	r, _ := debugRegistry(t)
	ctl := &scriptedController{actions: []capabilities.StepAction{capabilities.StepContinue}}
	native.SetHostDebugOps(r, ctl)
	res := runDebug(t, r, `"10 add 5" Debug.run-stepped`)
	if got := topInt(t, res); got != 15 {
		t.Errorf("run-stepped must parse+run the source (15), got %d", got)
	}
}

// ── (§7.1) Dashboard ─────────────────────────────────────────────────

func TestDebugWidgetAndDashboard(t *testing.T) {
	r, buf := debugRegistry(t)
	// A widget is a plain {title, sample} map; the dashboard renders each
	// widget's sample result under its title. Widgets compose as data, so a
	// module can contribute them by exporting such maps.
	src := `def w1 (Debug.widget "Sum" "1 add 2")
	        def w2 (Debug.widget "Twice" "21 mul 2")
	        [w1 w2] Debug.dashboard`
	res := runDebug(t, r, src)
	out, err := native.AsString(res[len(res)-1])
	if err != nil {
		t.Fatalf("dashboard must return the rendered String: %v", err)
	}
	for _, want := range []string{"Sum", "3", "Twice", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard render missing %q; got:\n%s", want, out)
		}
	}
	// It also prints the snapshot to the host output.
	if !strings.Contains(buf.String(), "Sum") {
		t.Errorf("dashboard must print to output; output = %q", buf.String())
	}
}

func TestDebugDashboardSurvivesBadWidget(t *testing.T) {
	r, _ := debugRegistry(t)
	// One malformed sample must not abort the whole snapshot.
	src := `def bad (Debug.widget "Bad" "1 add (((")
	        def good (Debug.widget "Good" "40 add 2")
	        [bad good] Debug.dashboard`
	res := runDebug(t, r, src)
	out, _ := native.AsString(res[len(res)-1])
	if !strings.Contains(out, "42") {
		t.Errorf("a bad widget must not take down the good one; got:\n%s", out)
	}
	if !strings.Contains(out, "error") && !strings.Contains(out, "parse") {
		t.Errorf("the bad widget should render an inline error; got:\n%s", out)
	}
}

// ── (D) System ───────────────────────────────────────────────────────

func TestDebugWords(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "Debug.words")
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("words must return a list: %v", err)
	}
	if lst.Len() == 0 {
		t.Fatal("words must be non-empty")
	}
	got := map[string]bool{}
	for _, v := range lst.Slice() {
		if s, _ := native.AsString(v); s != "" {
			got[s] = true
		}
	}
	if !got["add"] {
		t.Error("words should include the core word 'add'")
	}
	// PR #190 review: words must reflect the LIVE registry, not the static
	// help catalog. Words that are documented but moved to an unimported
	// module (e.g. `trace`/`read`/`write`/`stdin` → aql:io) are NOT
	// dispatchable here, so they must not appear — listing them would point
	// at names that fail with undefined_word.
	for _, moved := range []string{"trace", "read", "write", "stdin", "stdout", "stderr"} {
		if got[moved] {
			t.Errorf("words must not list %q — it is not registered without `import \"aql:io\"`", moved)
		}
	}
	// Every listed word must actually be dispatchable (sample-check `add`).
	if !r.IsBuiltinWord("add") {
		t.Error("sanity: add should be a builtin in this registry")
	}
}

func TestDebugDefs(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "def my-thing 99 Debug.defs")
	m, err := native.AsMap(res[len(res)-1])
	if err != nil || m == nil {
		t.Fatalf("defs must return a map: %v", err)
	}
	if _, ok := m.Get("my-thing"); !ok {
		t.Error("defs should include a freshly-defined binding 'my-thing'")
	}
}

func TestDebugStack(t *testing.T) {
	r, _ := debugRegistry(t)
	// The data stack below the call is captured cleanly (no markers).
	res := runDebug(t, r, "1 2 3 size (Debug.stack)")
	if got := topInt(t, res); got != 3 {
		t.Errorf("stack should snapshot the 3 values below it, got %d", got)
	}
	// Empty stack → empty list.
	r2, _ := debugRegistry(t)
	res2 := runDebug(t, r2, "size (Debug.stack)")
	if got := topInt(t, res2); got != 0 {
		t.Errorf("empty stack should snapshot to size 0, got %d", got)
	}
}

func TestDebugModulesListsSelf(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "Debug.modules")
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("modules must return a list: %v", err)
	}
	found := false
	for _, v := range lst.Slice() {
		if s, _ := native.AsString(v); s == "aql:debug" {
			found = true
		}
	}
	if !found {
		t.Error("modules should include 'aql:debug'")
	}
}

// ── (E) Memory ───────────────────────────────────────────────────────

func TestDebugSizeofMonotonic(t *testing.T) {
	r, _ := debugRegistry(t)
	small := topInt(t, runDebug(t, r, `"" Debug.sizeof`))
	big := topInt(t, runDebug(t, r, `"hello world" Debug.sizeof`))
	if big <= small {
		t.Errorf("sizeof must grow with content: empty=%d, filled=%d", small, big)
	}
	emptyList := topInt(t, runDebug(t, r, `[] Debug.sizeof`))
	fullList := topInt(t, runDebug(t, r, `[1 2 3 4 5] Debug.sizeof`))
	if fullList <= emptyList {
		t.Errorf("sizeof must grow with list length: []=%d, [..]=%d", emptyList, fullList)
	}
}

func TestDebugShape(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, `[1 2 [3 4]] Debug.shape`)
	m, err := native.AsMap(res[len(res)-1])
	if err != nil || m == nil {
		t.Fatalf("shape must return a map: %v", err)
	}
	depthV, _ := m.Get("max-depth")
	depth, _ := native.AsInteger(depthV)
	if depth < 1 {
		t.Errorf("nested list should have max-depth >= 1, got %d", depth)
	}
	listsV, _ := m.Get("lists")
	lists, _ := native.AsInteger(listsV)
	if lists < 2 {
		t.Errorf("expected at least 2 lists (outer + inner), got %d", lists)
	}
}

// TestDebugMemoryNoPanicOnTypeLiterals pins the no-panic discipline: the
// memory walkers must accept bare type literals (Data==nil) and carriers
// without panicking.
func TestDebugMemoryNoPanicOnTypeLiterals(t *testing.T) {
	r, _ := debugRegistry(t)
	for _, src := range []string{
		"List Debug.sizeof", "Map Debug.sizeof", "Integer Debug.sizeof",
		"List Debug.shape", "Map Debug.shape",
		"Integer Debug.tap", "List Debug.dump",
	} {
		if err := runDebugErr(t, r, src); err != nil {
			t.Errorf("%q must not error/panic on a type literal: %v", src, err)
		}
	}
}

// ── (F) Performance ──────────────────────────────────────────────────

func TestDebugStepsDeterministicAndMonotonic(t *testing.T) {
	r, _ := debugRegistry(t)
	a := topInt(t, runDebug(t, r, "[1 add 2] Debug.steps"))
	b := topInt(t, runDebug(t, r, "[1 add 2] Debug.steps"))
	if a != b {
		t.Errorf("steps must be deterministic across runs: %d vs %d", a, b)
	}
	if a <= 0 {
		t.Errorf("steps must count real work, got %d", a)
	}
	more := topInt(t, runDebug(t, r, "[1 add 2 mul 3 sub 4] Debug.steps"))
	if more <= a {
		t.Errorf("a longer body must take more steps: short=%d, long=%d", a, more)
	}
}

func TestDebugTimeDeterministicWithFixedClock(t *testing.T) {
	r, _ := debugRegistry(t)
	native.SetHostClock(r, capabilities.FixedClock{T: time.Unix(0, 0)})
	res := runDebug(t, r, "[1 add 2] Debug.time")
	m, err := native.AsMap(res[len(res)-1])
	if err != nil || m == nil {
		t.Fatalf("time must return a map: %v", err)
	}
	for _, key := range []string{"result", "elapsed-ms", "steps"} {
		if _, ok := m.Get(key); !ok {
			t.Errorf("time result missing key %q", key)
		}
	}
	// A frozen clock means elapsed is exactly 0 — deterministic.
	elapsedV, _ := m.Get("elapsed-ms")
	elapsed, _ := native.AsFloat(elapsedV)
	if elapsed != 0 {
		t.Errorf("with a FixedClock, elapsed-ms must be 0, got %v", elapsed)
	}
	resultV, _ := m.Get("result")
	if n, _ := native.AsInteger(resultV); n != 3 {
		t.Errorf("time result should be the body's value 3, got %d", n)
	}
}

func TestDebugBench(t *testing.T) {
	r, _ := debugRegistry(t)
	native.SetHostClock(r, capabilities.FixedClock{T: time.Unix(0, 0)})
	res := runDebug(t, r, "Debug.bench [1 add 2] 5")
	m, err := native.AsMap(res[len(res)-1])
	if err != nil || m == nil {
		t.Fatalf("bench must return a map: %v", err)
	}
	nV, _ := m.Get("n")
	if n, _ := native.AsInteger(nV); n != 5 {
		t.Errorf("bench n should be 5, got %d", n)
	}
	for _, key := range []string{"total-ms", "mean-ms", "min-ms", "max-ms", "steps-per-run"} {
		if _, ok := m.Get(key); !ok {
			t.Errorf("bench result missing key %q", key)
		}
	}
}

func TestDebugBenchRejectsNonPositiveN(t *testing.T) {
	r, _ := debugRegistry(t)
	if err := runDebugErr(t, r, "Debug.bench [1 add 2] 0"); err == nil {
		t.Error("bench with n=0 must raise")
	}
}

func TestDebugTracePrintsAndReturns(t *testing.T) {
	r, buf := debugRegistry(t)
	res := runDebug(t, r, "[1 add 2] Debug.trace")
	if got := topInt(t, res); got != 3 {
		t.Errorf("trace must return the body result 3, got %d", got)
	}
	if !strings.Contains(buf.String(), "trace") {
		t.Errorf("trace must print a trace; output = %q", buf.String())
	}
}

// profileCounts runs Debug.profile and returns word -> step count.
func profileCounts(t *testing.T, r *native.Registry, src string) map[string]int64 {
	t.Helper()
	res := runDebug(t, r, src+" Debug.profile")
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("profile must return a list: %v", err)
	}
	out := map[string]int64{}
	for _, row := range lst.Slice() {
		m, _ := native.AsMap(row)
		if m == nil {
			t.Fatal("profile rows must be {word, steps} maps")
		}
		wv, ok1 := m.Get("word")
		sv, ok2 := m.Get("steps")
		if !ok1 || !ok2 {
			t.Error("profile row missing 'word'/'steps'")
			continue
		}
		w, _ := native.AsString(wv)
		n, _ := native.AsInteger(sv)
		out[w] = n
	}
	return out
}

func TestDebugProfile(t *testing.T) {
	r, _ := debugRegistry(t)
	counts := profileCounts(t, r, "[1 add 2 mul 3]")
	if len(counts) == 0 {
		t.Fatal("profile of a non-trivial body must have rows")
	}
	// Each call is counted ONCE (no forward/stack double-count).
	if counts["add"] != 1 {
		t.Errorf("add should be counted once, got %d", counts["add"])
	}
	if counts["mul"] != 1 {
		t.Errorf("mul should be counted once, got %d", counts["mul"])
	}
}

// TestDebugProfileAttributesModuleCalls pins the PR #190 fix: a dotted
// module call (Math.sqrt) is attributed to the function it resolves to,
// not just the `get` expansion a bare-word filter would see.
func TestDebugProfileAttributesModuleCalls(t *testing.T) {
	r, _ := debugRegistry(t)
	if err := InstallMathExports(r); err != nil {
		t.Fatal(err)
	}
	// Stamp module provenance the way a real `import` does, so the dispatch
	// carries its module origin (the signal the profiler attributes on).
	InstallResolver(r)
	runDebug(t, r, `import "aql:math-util"`)
	counts := profileCounts(t, r, "[16.0 MathUtil.sqrt]")
	if counts["sqrt"] < 1 {
		t.Errorf("profile must attribute the MathUtil.sqrt dispatch to 'sqrt'; got %v", counts)
	}
}
