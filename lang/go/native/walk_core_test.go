package native

import (
	"reflect"
	"strings"
	"testing"

	"github.com/boru-lang/boru/parser/go"
)

// walkRun parses and runs source against a fresh DefaultRegistry, returning the
// final stack (failing the test on parse/run error).
func walkRun(t *testing.T, src string) []Value {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

// walkPaths runs a program that accumulates visited paths into a captured
// FlexList named `acc` and ends with `acc` on top; it returns those paths.
func walkPaths(t *testing.T, src string) []string {
	t.Helper()
	out := walkRun(t, src)
	if len(out) == 0 {
		t.Fatalf("empty stack for %q", src)
	}
	fd, err := AsFlexList(out[len(out)-1])
	if err != nil {
		t.Fatalf("top of stack is not a FlexList for %q: %v", src, err)
	}
	paths := make([]string, len(fd.Elems))
	for i, e := range fd.Elems {
		s, serr := e.AsConcreteString()
		if serr != nil {
			t.Fatalf("path element %d is not a string for %q: %v", i, src, serr)
		}
		paths[i] = s
	}
	return paths
}

// descendInto accumulates m.path into the captured FlexList `acc`, dropping
// walk's (unchanged) return value so `acc` is left on top.
const accDescend = `def acc (flex [])  walk {mode: "%MODE%"} %DATA% (m:Any => [acc (m.path) append])  drop  acc`

func dfs(data string) string {
	return strings.NewReplacer("%MODE%", "depth", "%DATA%", data).Replace(accDescend)
}
func bfs(data string) string {
	return strings.NewReplacer("%MODE%", "breadth", "%DATA%", data).Replace(accDescend)
}

func TestWalkDFSDescentOrder(t *testing.T) {
	got := walkPaths(t, dfs(`{a:1 b:[2 3]}`))
	want := []string{"", "a", "b", "b.0", "b.1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DFS descent paths = %v, want %v", got, want)
	}
}

func TestWalkBFSDescentOrder(t *testing.T) {
	// a and b are both containers, so DFS and BFS differ: BFS visits both
	// depth-1 nodes before any depth-2 node.
	got := walkPaths(t, bfs(`{a:{x:1} b:{y:2}}`))
	want := []string{"", "a", "b", "a.x", "b.y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BFS descent paths = %v, want %v", got, want)
	}
	// Same data, DFS dives into a's subtree before visiting b.
	gotD := walkPaths(t, dfs(`{a:{x:1} b:{y:2}}`))
	wantD := []string{"", "a", "a.x", "b", "b.y"}
	if !reflect.DeepEqual(gotD, wantD) {
		t.Errorf("DFS descent paths = %v, want %v", gotD, wantD)
	}
}

func TestWalkDFSDescentAscentOrder(t *testing.T) {
	// Two captured lists: `dn` records descent (pre-order), `up` ascent
	// (post-order). End with `dn` then `up` for separate inspection.
	src := `def dn (flex [])  def up (flex [])
		walk {mode: "depth"} {a:{x:1}}
			(m:Any => [dn (m.path) append])
			(m:Any => [up (m.path) append])
		drop  dn  up`
	out := walkRun(t, src)
	if len(out) < 2 {
		t.Fatalf("expected dn and up on stack, got %d values", len(out))
	}
	up := flexStrings(t, out[len(out)-1])
	dn := flexStrings(t, out[len(out)-2])
	wantDn := []string{"", "a", "a.x"} // pre-order
	wantUp := []string{"a.x", "a", ""} // post-order
	if !reflect.DeepEqual(dn, wantDn) {
		t.Errorf("DFS descent = %v, want %v", dn, wantDn)
	}
	if !reflect.DeepEqual(up, wantUp) {
		t.Errorf("DFS ascent = %v, want %v", up, wantUp)
	}
}

func TestWalkBFSAscentReverseVisitation(t *testing.T) {
	src := `def dn (flex [])  def up (flex [])
		walk {mode: "breadth"} {a:{x:1} b:{y:2}}
			(m:Any => [dn (m.path) append])
			(m:Any => [up (m.path) append])
		drop  dn  up`
	out := walkRun(t, src)
	up := flexStrings(t, out[len(out)-1])
	dn := flexStrings(t, out[len(out)-2])
	// Ascent is the exact reverse of the descent (visitation) order.
	wantUp := make([]string, len(dn))
	for i := range dn {
		wantUp[len(dn)-1-i] = dn[i]
	}
	if !reflect.DeepEqual(up, wantUp) {
		t.Errorf("BFS ascent = %v, want reverse of descent %v (= %v)", up, dn, wantUp)
	}
}

func TestWalkUserDefinedObject(t *testing.T) {
	// A class instance descends generically via the IdealConverter (ToMap).
	got := walkPaths(t, `def Point (class {x:Integer y:Integer})
		def acc (flex [])
		walk {mode: "depth"} (make Point {x:1 y:2}) (m:Any => [acc (m.path) append])
		drop  acc`)
	want := []string{"", "x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("object descent paths = %v, want %v", got, want)
	}
}

func TestWalkReturnsInputUnchanged(t *testing.T) {
	// walk is visit-only: it returns its input data (here a map) unchanged,
	// so the value remains on the stack for the pipeline.
	out := walkRun(t, `walk {mode: "depth"} {a:1 b:2} (m:Any => [m.path])`)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %s", len(out), Canon(out))
	}
	if got := out[0].String(); got != "{a:1 b:2}" {
		t.Errorf("walk return = %s, want {a:1 b:2}", got)
	}
}

func TestWalkDefaultModeDepth(t *testing.T) {
	// Empty options map → default depth-first.
	got := walkPaths(t, `def acc (flex [])  walk {} {a:{x:1}} (m:Any => [acc (m.path) append])  drop  acc`)
	want := []string{"", "a", "a.x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("default-mode paths = %v, want %v", got, want)
	}
}

func TestWalkQuotationHookForm(t *testing.T) {
	// A quotation hook receives the payload map on the stack (no `m` name).
	// Pull the path off the map with `get` and accumulate it.
	got := walkPaths(t, `def acc (flex [])  walk {mode: "depth"} {a:1} [dot path/q acc swap append]  drop  acc`)
	want := []string{"", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("quotation-hook paths = %v, want %v", got, want)
	}
}

// --- negative / safety ---

func TestWalkBadMode(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, perr := parser.Parse(`walk {mode: "sideways"} {a:1} (m:Any => [m.path])`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	_, rerr := NewTop(r).Run(toks)
	if rerr == nil {
		t.Fatal("expected an error for an unknown mode, got nil")
	}
	if !strings.Contains(rerr.Error(), "walk") {
		t.Errorf("error %q should mention walk", rerr.Error())
	}
}

func TestWalkHookErrorPropagates(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// `raise` inside the hook must surface as walk's error.
	toks, perr := parser.Parse(`walk {mode: "depth"} {a:1} (m:Any => [raise boom "kaboom"])`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	_, rerr := NewTop(r).Run(toks)
	if rerr == nil {
		t.Fatal("expected the hook error to propagate, got nil")
	}
}

// TestWalkNoPanicTypeLiterals: type literals and empty containers must not
// panic — they visit only the root (no descent).
func TestWalkNoPanicTypeLiterals(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("walk panicked on a type-literal / empty input: %v", r)
		}
	}()
	for _, data := range []string{`Map`, `List`, `{}`, `[]`, `42`} {
		got := walkPaths(t, dfs(data))
		if !reflect.DeepEqual(got, []string{""}) {
			t.Errorf("walk %s descent paths = %v, want [\"\"] (root only)", data, got)
		}
	}
}

// flexStrings reads a FlexList value into a []string of its (string) elements.
func flexStrings(t *testing.T, v Value) []string {
	t.Helper()
	fd, err := AsFlexList(v)
	if err != nil {
		t.Fatalf("value is not a FlexList: %v", err)
	}
	out := make([]string, len(fd.Elems))
	for i, e := range fd.Elems {
		s, serr := e.AsConcreteString()
		if serr != nil {
			t.Fatalf("element %d is not a string: %v", i, serr)
		}
		out[i] = s
	}
	return out
}
