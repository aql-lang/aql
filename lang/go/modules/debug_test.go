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
		"parse", "deps", "explain",
		"words", "defs", "modules",
		"sizeof", "shape",
		"steps", "time", "bench", "trace", "profile",
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
	res := runDebug(t, r, `Debug.label 7 "answer"`)
	if got := topInt(t, res); got != 7 {
		t.Errorf("label must return the value: got %d, want 7", got)
	}
	if !strings.Contains(buf.String(), "answer:") {
		t.Errorf("label must print the label; output = %q", buf.String())
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
	found := false
	for _, v := range lst.Slice() {
		if s, _ := native.AsString(v); s == "add" {
			found = true
		}
	}
	if !found {
		t.Error("words should include the core word 'add'")
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

func TestDebugProfile(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "[1 add 2 mul 3] Debug.profile")
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() {
		t.Fatalf("profile must return a list: %v", err)
	}
	if lst.Len() == 0 {
		t.Fatal("profile of a non-trivial body must have rows")
	}
	// Each row is a {word, steps} map.
	first, _ := native.AsMap(lst.Get(0))
	if first == nil {
		t.Fatal("profile rows must be maps")
	}
	if _, ok := first.Get("word"); !ok {
		t.Error("profile row missing 'word'")
	}
	if _, ok := first.Get("steps"); !ok {
		t.Error("profile row missing 'steps'")
	}
}
