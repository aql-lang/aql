package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// runMapIter runs src and returns the rendered residual stack and captured
// stdout (for the side-effecting for-each).
func runMapIter(t *testing.T, src string) (string, string) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	reg.Output = &out
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return formatStack(res), out.String()
}

// runMapIterErr runs src expecting an error, returning its message.
func runMapIterErr(t *testing.T, src string) string {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		return err.Error()
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Output = &bytes.Buffer{}
	if _, err := native.NewTop(reg).Run(values); err != nil {
		return err.Error()
	}
	t.Fatalf("expected an error for %q, got none", src)
	return ""
}

// The lambda's KeyVal exposes k (key), v (value), i (0-based index), and n
// (total) — and each keeps the map shape throughout.
func TestMapEachKeyValFields(t *testing.T) {
	cases := []struct{ body, want string }{
		{"[kv.v mul 10]", "{a:10 b:20 c:30}"}, // value
		{"[kv.k]", "{a:'a' b:'b' c:'c'}"},     // key
		{"[kv.i]", "{a:0 b:1 c:2}"},           // index
		{"[kv.n]", "{a:3 b:3 c:3}"},           // total
	}
	for _, c := range cases {
		got, _ := runMapIter(t, "{a:1 b:2 c:3} each ([kv:KeyVal] => "+c.body+")")
		if got != c.want {
			t.Errorf("each ([kv] => %s) = %q, want %q", c.body, got, c.want)
		}
	}
}

// The quotation form receives just the value and keeps the map shape.
func TestMapEachQuotation(t *testing.T) {
	if got, _ := runMapIter(t, "{a:1 b:2 c:3} each [mul 10]"); got != "{a:10 b:20 c:30}" {
		t.Errorf("each [mul 10] over a map = %q, want {a:10 b:20 c:30}", got)
	}
}

// for-each runs for side effects, in insertion order, and produces nothing.
func TestMapForEach(t *testing.T) {
	got, out := runMapIter(t, "{a:1 b:2 c:3} for-each ([kv:KeyVal] => [print kv.v])")
	if got != "" {
		t.Errorf("for-each should leave the stack empty, got %q", got)
	}
	if order := strings.Join(strings.Fields(out), " "); order != "1 2 3" {
		t.Errorf("for-each output order = %q, want \"1 2 3\"", order)
	}
}

// filter's lambda receives a KeyVal and keeps the map shape (the fix: it used
// to project to a list).
func TestMapFilterLambdaKeepsMap(t *testing.T) {
	if got, _ := runMapIter(t, "{a:1 b:5 c:3} filter ([kv:KeyVal] => [kv.v gt 2])"); got != "{b:5 c:3}" {
		t.Errorf("filter lambda over a map = %q, want {b:5 c:3}", got)
	}
	// Filtering by key works too.
	if got, _ := runMapIter(t, "{a:1 b:5 c:3} filter ([kv:KeyVal] => [kv.k neq 'a'])"); got != "{b:5 c:3}" {
		t.Errorf("filter by key = %q, want {b:5 c:3}", got)
	}
}

// fold reduces a map's entries; the lambda receives (accumulator, KeyVal).
func TestMapFold(t *testing.T) {
	if got, _ := runMapIter(t, "fold [add] {a:1 b:2 c:3} 0"); got != "6" {
		t.Errorf("fold [add] {…} 0 = %q, want 6", got)
	}
	if got, _ := runMapIter(t, "0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:1 b:2 c:3}"); got != "6" {
		t.Errorf("fold lambda over a map = %q, want 6", got)
	}
}

// scan is the running fold over a map's values, keeping the keys; the first
// value seeds and is its key's output. scan's last value equals fold's result.
func TestMapScan(t *testing.T) {
	if got, _ := runMapIter(t, "{a:1 b:2 c:3} scan [add]"); got != "{a:1 b:3 c:6}" {
		t.Errorf("scan [add] over a map = %q, want {a:1 b:3 c:6}", got)
	}
	if got, _ := runMapIter(t, "{a:1 b:2 c:3} scan ([acc:Integer kv:KeyVal] => [acc add kv.v])"); got != "{a:1 b:3 c:6}" {
		t.Errorf("scan lambda over a map = %q, want {a:1 b:3 c:6}", got)
	}
	if got, _ := runMapIter(t, "{a:5} scan [add]"); got != "{a:5}" {
		t.Errorf("scan over a singleton map = %q, want {a:5}", got)
	}
	if got, _ := runMapIter(t, "{} scan [add]"); got != "{}" {
		t.Errorf("scan over an empty map = %q, want {}", got)
	}
	// A scan body that leaves nothing is an error (scan's body has two values).
	if msg := runMapIterErr(t, "{a:1 b:2} scan [drop drop]"); !strings.Contains(msg, "body produced no result") {
		t.Errorf("scan no-result error = %q, want 'body produced no result'", msg)
	}
}

// keys / vals project a map to a list, in insertion order.
func TestMapKeysVals(t *testing.T) {
	if got, _ := runMapIter(t, "{a:1 b:2 c:3} keys"); got != "['a' 'b' 'c']" {
		t.Errorf("keys = %q, want ['a' 'b' 'c']", got)
	}
	if got, _ := runMapIter(t, "{a:1 b:2 c:3} vals"); got != "[1 2 3]" {
		t.Errorf("vals = %q, want [1 2 3]", got)
	}
}

// Negative cases: the map forms reject what their list forms reject.
func TestMapIterNegatives(t *testing.T) {
	cases := []struct{ src, want string }{
		{"{a:1} each [drop]", "body produced no result"},
		{"{a:1 b:2} filter ([kv:KeyVal] => [kv.v])", "must produce a Boolean"},
		{"fold [add] {}", "empty map with no initial value"},
		{"keys [1 2 3]", "signature"},
		{"vals 5", "signature"},
	}
	for _, c := range cases {
		if msg := runMapIterErr(t, c.src); !strings.Contains(msg, c.want) {
			t.Errorf("%q error = %q, want substring %q", c.src, msg, c.want)
		}
	}
}
