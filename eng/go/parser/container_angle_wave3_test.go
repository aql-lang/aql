package parser

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
)

// --- typed lists ([:T]) with concrete elements ---

func TestTypedListWave3Elements(t *testing.T) {
	for _, src := range []string{"[:Integer 1 2]", "[:Integer, 1, 2]", "[1, 2, :Integer]"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !eng.IsTypedList(vals[0]) {
			t.Fatalf("%q: expected one typed list, got %v", src, vals)
		}
		ci, err := eng.AsChildType(vals[0])
		if err != nil {
			t.Fatalf("%q: AsChildType: %v", src, err)
		}
		// The child constraint is the Integer TYPE LITERAL (the literal IS
		// its lattice node, so compare by ID, not Parent).
		if !eng.IsTypeLiteral(ci.Child) || ci.Child.ID != eng.TInteger.ID {
			t.Errorf("%q: child constraint %v, want the Integer type literal", src, ci.Child)
		}
		if len(ci.Elements) != 2 {
			t.Fatalf("%q: got %d elements, want 2", src, len(ci.Elements))
		}
		for i, want := range []int64{1, 2} {
			if n, err := eng.AsInteger(ci.Elements[i]); err != nil || n != want {
				t.Errorf("%q: element %d = %v, want %d", src, i, ci.Elements[i], want)
			}
		}
	}
}

func TestTypedMapWave3Entries(t *testing.T) {
	// Source order is b, a — NOT alphabetical — so this pins D1 insertion
	// order for the typed-map entries path (the `ko` order channel reaches
	// convertTypedMap, not just plain maps). design/FLEX-ATTRS.1.md §3.
	vals := mustParseWave3(t, "{:Integer b:2 a:1}")
	if len(vals) != 1 || !eng.IsTypedMap(vals[0]) {
		t.Fatalf("expected one typed map, got %v", vals)
	}
	ci, err := eng.AsChildType(vals[0])
	if err != nil {
		t.Fatalf("AsChildType: %v", err)
	}
	if !eng.IsTypeLiteral(ci.Child) || ci.Child.ID != eng.TInteger.ID {
		t.Errorf("child constraint %v, want the Integer type literal", ci.Child)
	}
	if len(ci.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(ci.Entries))
	}
	// Entries are collected in SOURCE order (b before a), not sorted.
	for i, want := range []struct {
		key string
		n   int64
	}{{"b", 2}, {"a", 1}} {
		e := ci.Entries[i]
		if e.Key != want.key {
			t.Errorf("entry %d key %q, want %q", i, e.Key, want.key)
		}
		if n, err := eng.AsInteger(e.Value); err != nil || n != want.n {
			t.Errorf("entry %d value %v, want %d", i, e.Value, want.n)
		}
	}
}

func TestTypedContainerWave3Errors(t *testing.T) {
	// Malformed child constraints and malformed elements/entries all fail
	// cleanly, at the top level and nested.
	for _, src := range []string{
		"[:1__0]",           // bad child (top-level typed list)
		"{:1__0}",           // bad child (top-level typed map)
		"[:Integer 1__0]",   // bad element alongside the constraint
		"{:Integer a:1__0}", // bad entry alongside the constraint
	} {
		wantParseErrWave3(t, src, "misplaced `_`")
	}
}

func TestTypedContainerWave3Nested(t *testing.T) {
	// A typed map as a word-context list element.
	vals := mustParseWave3(t, "[{:String}]")
	if len(vals) != 1 {
		t.Fatalf("[{:String}]: got %d values, want 1", len(vals))
	}
	lst, err := eng.AsList(vals[0])
	if err != nil || lst.Len() != 1 {
		t.Fatalf("[{:String}]: expected a 1-element list, got %v (err %v)", vals[0], err)
	}
	if !eng.IsTypedMap(lst.Get(0)) {
		t.Errorf("[{:String}]: element is not a typed map: %s", lst.Get(0).String())
	}
}

func TestTypedContainerWave3DepthLimit(t *testing.T) {
	// Pathologically deep TYPED nesting takes the convertTypedList /
	// convertTypedMap recursion and must fail with the clean depth error.
	deep := func(open, close string) string {
		var b strings.Builder
		for i := 0; i < maxParseNestingDepth+50; i++ {
			b.WriteString(open)
		}
		b.WriteString("Integer")
		for i := 0; i < maxParseNestingDepth+50; i++ {
			b.WriteString(close)
		}
		return b.String()
	}
	for name, src := range map[string]string{
		"typed-lists": deep("[:", "]"),
		"typed-maps":  deep("{:", "}"),
	} {
		_, err := parseWave3(t, src)
		if err == nil {
			t.Fatalf("%s: deep typed nesting parsed without error", name)
		}
		if !strings.Contains(err.Error(), "nesting") {
			t.Errorf("%s: error %q is not the nesting-depth diagnostic", name, err)
		}
	}
}

// --- data-context container error propagation ---

func TestDataContainerWave3Errors(t *testing.T) {
	for _, src := range []string{
		"[{a:1__0}]", // map element of a word-context list
		"[a:1__0]",   // raw pair-map element
		"{a:[1__0]}", // list value inside a map
	} {
		wantParseErrWave3(t, src, "misplaced `_`")
	}
}

// --- generics angle sugar ---

func TestAngleWave3DefHead(t *testing.T) {
	// `def Name<T>` desugars to `Name gen [params]`.
	vals := mustParseWave3(t, "def Box<T> 1")
	if len(vals) != 5 {
		t.Fatalf("def Box<T> 1: got %d values, want 5: %v", len(vals), vals)
	}
	for i, want := range []string{"def", "Box", "gen"} {
		if w := wordNameWave3(t, vals[i], "def head"); w.Name != want {
			t.Errorf("def Box<T> 1: value %d word %q, want %q", i, w.Name, want)
		}
	}
	params, err := eng.AsList(vals[3])
	if err != nil || params.Len() != 1 {
		t.Fatalf("def Box<T> 1: params %v (err %v), want a 1-element list", vals[3], err)
	}
	if w := wordNameWave3(t, params.Get(0), "param"); w.Name != "T" {
		t.Errorf("def Box<T> 1: param %q, want T", w.Name)
	}
	// Multiple plain params.
	vals = mustParseWave3(t, "def Box<T Q> 1")
	params, err = eng.AsList(vals[3])
	if err != nil || params.Len() != 2 {
		t.Fatalf("def Box<T Q> 1: params %v (err %v), want 2 entries", vals[3], err)
	}
}

func TestAngleWave3DefHeadConstraints(t *testing.T) {
	// `extends` and `=` entries become (T extends C) / (T default D) spans.
	for src, kw := range map[string]string{
		"def Box<T extends Integer> 1": "extends",
		"def Box<T = Integer> 1":       "default",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 5 {
			t.Fatalf("%q: got %d values, want 5: %v", src, len(vals), vals)
		}
		params, err := eng.AsList(vals[3])
		if err != nil || params.Len() != 1 {
			t.Fatalf("%q: params %v (err %v), want 1 entry", src, vals[3], err)
		}
		entry := params.Get(0)
		if !eng.IsParenExpr(entry) {
			t.Fatalf("%q: param entry is not a ParenExpr: %v", src, entry)
		}
		if s := entry.String(); !strings.Contains(s, kw) {
			t.Errorf("%q: entry %q lacks the %q keyword", src, s, kw)
		}
	}
}

func TestAngleWave3DefHeadErrors(t *testing.T) {
	wantParseErrWave3(t, "def Box<t> 1", "capitalised")
	wantParseErrWave3(t, "def Box<T extends> 1", "needs a value")
	// A malformed constraint operand surfaces its own error.
	wantParseErrWave3(t, "def Box<T extends 1__0> 1", "misplaced `_`")
}

func TestAngleWave3UseSites(t *testing.T) {
	// Word-context list element and data-context map value both desugar to
	// the canonical `(Name of [args])` paren span.
	for _, src := range []string{"[Box<Integer>]", "{x: Box<Integer>}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, "Box") || !strings.Contains(s, "of") {
			t.Errorf("%q: rendering %q lacks the Box-of desugar", src, s)
		}
	}
	// Empty group.
	vals := mustParseWave3(t, "Box<>")
	if len(vals) != 1 || !eng.IsParenExpr(vals[0]) {
		t.Fatalf("Box<>: expected one ParenExpr, got %v", vals)
	}
	// An error inside the argument list surfaces.
	wantParseErrWave3(t, "Box<1__0>", "misplaced `_`")
}

func TestAngleWave3Unclosed(t *testing.T) {
	// Unclosed at EOF, in word and data context, names the receiver.
	wantParseErrWave3(t, "Box<Integer", "unclosed angle bracket: Box<")
	wantParseErrWave3(t, "{x: Box<Integer", "unclosed angle bracket: Box<")
}

// TestAngleWave3GateRejections pins the contextual gate: `<` after a
// non-capitalised / quoted / non-name value is NOT generics, and falls to
// the XML consumer, which rejects it — an error, never a panic.
func TestAngleWave3GateRejections(t *testing.T) {
	for _, src := range []string{
		"'Box'<1>",  // quoted receiver: not a type name
		"1<2",       // numeric receiver
		"Box<A><B>", // second angle group after a folded one
	} {
		wantParseErrWave3(t, src, "")
	}
}

// --- pair-key edge cases ---

func TestPairKeyWave3Edges(t *testing.T) {
	// A NUMERIC optional key (`{1?:Integer}`): the key comes from the raw
	// token source and the optional desugar still applies.
	vals := mustParseWave3(t, "{1?:Integer}")
	if len(vals) != 1 {
		t.Fatalf("{1?:Integer}: got %d values, want 1", len(vals))
	}
	m, err := eng.AsMap(vals[0])
	if err != nil {
		t.Fatalf("{1?:Integer}: AsMap: %v", err)
	}
	v, ok := m.Get("1")
	if !ok {
		t.Fatalf("{1?:Integer}: map lacks key \"1\": %s", vals[0].String())
	}
	if s := v.String(); !strings.Contains(s, "tor") {
		t.Errorf("{1?:Integer}: optional value %q is not a disjunct", s)
	}
	// Empty quoted keys (plain and optional) parse without error.
	for _, src := range []string{"{'':1}", "{''?:1}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
	}
	// Optional list params alongside plain ones, plain values, and other
	// optionals.
	for _, src := range []string{
		"[a?:Integer b:String]",
		"[x?:Integer 1]",
		"[a?:Integer, b?:String]",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, "tor None tor Absent") {
			t.Errorf("%q: %q lacks the parser-ordered optional disjunct", src, s)
		}
	}
}
