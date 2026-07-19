package native

import (
	"strings"
	"testing"
)

// Codex round-3 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	#1 param-boundary retag — a {:T}/[:T] argument binds TAGGED, so a write
//	   inside the fn body is enforced at runtime (compiled AND interpreted).
//	#2 inject re-tags its rebuilt container against the data template's {:T}.
//	#3 a flex chained-check residual retains the tag (a second write is caught).
//	#4 the copy / reorder stdlib ops (reverse / take / shed / unique / sort /
//	   sortby / slice / filter) retain the source element tag so a downstream
//	   write stays enforced and a downstream read narrows.

// --- #1: param-body write enforcement ---

// A write into a {:T}/[:T] PARAM is enforced at runtime, because the param now
// binds the argument tagged (RetagTypedContainerParam). Both a plain and a flex
// argument are tagged.
func TestParamBodyWriteEnforcedRuntime(t *testing.T) {
	for _, src := range []string{
		`def p fn [[m:{:Integer}] [Map] [(m set b/q "wrong")]] def t:{:Integer} {a:1} (p t)`,
		`def p fn [[xs:[:Integer]] [List] [(xs set 0 "wrong")]] def t:[:Integer] [1 2 3] (p t)`,
		// a flex argument is tagged too — the body write still rejects
		`def p fn [[m:{:Integer}] [Map] [(m set b/q "wrong")]] def t:{:Integer} (flex {a:1}) (p t)`,
	} {
		_, err := tcRun(t, src)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type") {
			t.Errorf("[%s] expected a param-body write reject, got %v", src, err)
		}
	}
}

// A conforming param-body write succeeds; an UNTYPED param arg is unenforced.
func TestParamBodyWriteEnforcedRuntimeNegatives(t *testing.T) {
	if _, err := tcRun(t, `def p fn [[m:{:Integer}] [Map] [(m set b/q 2)]] def t:{:Integer} {a:1} (p t)`); err != nil {
		t.Errorf("conforming param-body write should succeed: %v", err)
	}
	if _, err := tcRun(t, `def p fn [[m:Map] [Map] [(m set b/q "ok")]] def t {a:1} (p t)`); err != nil {
		t.Errorf("untyped param-body write should succeed: %v", err)
	}
}

// --- #4: copy / reorder ops retain the tag (core words, full run) ---

func TestListCopyRetainsTagRejectsBadWrite(t *testing.T) {
	for _, src := range []string{
		`def xs:[:Integer] [1 2 3] def ys (reverse xs) (ys set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] def ys (take 2 xs) (ys set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] def ys (shed 1 xs) (ys set 0 "bad")`,
		`def xs:[:Integer] [3 1 2] def ys (sort xs) (ys set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] def ys (slice 0 2 xs) (ys set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] def ys (filter [0 gt] xs) (ys set 0 "bad")`,
		// a {:T} map reordered by sort / filtered stays {:T}
		`def m:{:Integer} {a:3 b:1} def n (sort m) (n set a/q "bad")`,
		`def m:{:Integer} {a:3 b:1} def n (filter [0 gt] m) (n set a/q "bad")`,
	} {
		_, err := tcRun(t, src)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type") {
			t.Errorf("[%s] expected an element-type reject, got %v", src, err)
		}
	}
}

func TestListCopyRetainsTagPositive(t *testing.T) {
	for _, src := range []string{
		`def xs:[:Integer] [1 2 3] def ys (reverse xs) ys`,
		`def xs:[:Integer] [1 2 3] def ys (take 2 xs) ys`,
		`def xs:[:Integer] [1 2 3] def ys (shed 1 xs) ys`,
		`def xs:[:Integer] [3 1 2] def ys (sort xs) ys`,
		`def xs:[:Integer] [1 2 3] def ys (slice 0 2 xs) ys`,
		`def xs:[:Integer] [1 2 3] def ys (filter [0 gt] xs) ys`,
		`def m:{:Integer} {a:3 b:1} def n (sort m) n`,
		`def m:{:Integer} {a:3 b:1} def n (filter [0 gt] m) n`,
	} {
		out, err := tcRun(t, src)
		if err != nil {
			t.Fatalf("[%s] run: %v", src, err)
		}
		c, ok := out[len(out)-1].ElemConstraint()
		if !ok || !c.Equal(TInteger) {
			t.Errorf("[%s] copy dropped the element tag (ok=%v)", src, ok)
		}
	}
}

// The negative: an UNTYPED list/map copy carries no tag — the retention is
// scoped to genuinely-typed sources, so untyped flows stay byte-identical.
func TestUntypedCopyNoTag(t *testing.T) {
	for _, src := range []string{
		`def xs [1 2 3] def ys (reverse xs) ys`,
		`def xs [1 2 3] def ys (sort xs) ys`,
		`def xs [1 2 3] def ys (filter [0 gt] xs) ys`,
		`def m {a:3 b:1} def n (sort m) n`,
	} {
		out, err := tcRun(t, src)
		if err != nil {
			t.Fatalf("[%s] run: %v", src, err)
		}
		if _, ok := out[len(out)-1].ElemConstraint(); ok {
			t.Errorf("[%s] untyped copy unexpectedly carries a tag", src)
		}
	}
}

// --- #2 + #4 module-scoped ops (unique / sortby / inject): direct handler
// calls, since the test registry has no module resolver ---

func TestModuleCopyRetainsTagDirect(t *testing.T) {
	xs, r := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	store, _ := tcValue(t, `def s {} s`)

	assertTagged := func(name string, out []Value, err error) {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c, ok := out[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
			t.Errorf("%s dropped the [:Integer] tag (ok=%v)", name, ok)
		}
	}

	// #4 unique — args[0] is the source list.
	out, err := uniqueHandler([]Value{xs}, nil, nil, r)
	assertTagged("unique", out, err)

	// #4 sortby — args[0]=keys, args[1]=data (the tagged source).
	out, err = sortbyHandler([]Value{xs, xs}, nil, nil, r)
	assertTagged("sortby", out, err)

	// #2 inject — args[0]=val (the tagged data), args[1]=store.
	out, err = injectHandler([]Value{xs, store}, nil, nil, r)
	assertTagged("inject", out, err)
}

// #2 — inject enforces the {:T} constraint on its rebuilt result: a subsequent
// non-conforming write is rejected because the tag survives the rebuild.
func TestInjectPreservesTagWriteReject(t *testing.T) {
	m, r := tcValue(t, `def m:{:Integer} {a:1 b:2} m`)
	store, _ := tcValue(t, `def s {} s`)
	out, err := injectHandler([]Value{m, store}, nil, nil, r)
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	tagged := out[0]
	if _, werr := setMapHandler([]Value{NewString("a"), NewString("bad"), tagged}, nil, nil, r); werr == nil ||
		!strings.Contains(werr.Error(), "does not conform to element type Integer") {
		t.Errorf("write into inject result should reject: got %v", werr)
	}
	// The negative: an untyped inject result is unenforced.
	um, ur := tcValue(t, `def m {a:1} m`)
	uout, uerr := injectHandler([]Value{um, store}, nil, nil, ur)
	if uerr != nil {
		t.Fatalf("untyped inject: %v", uerr)
	}
	if _, werr := setMapHandler([]Value{NewString("a"), NewString("ok"), uout[0]}, nil, nil, ur); werr != nil {
		t.Errorf("write into untyped inject result should succeed: %v", werr)
	}
}

// #2 — inject's re-tag ENFORCES the {:T} constraint: if a resolved marker
// injects a value that violates the element type, inject rejects it. This state
// is unreachable through source (a {:T} container can't be CONSTRUCTED holding a
// marker string), so it is pinned by a direct handler call over an artificially
// tagged template — the same discipline TestD2TypedContainerBoundBranches uses.
func TestInjectRetagRejects(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := newMap("a", NewString("`bad`")) // a marker resolved from the store
	tmpl.SetElemConstraint(NewTypeLiteral(TInteger))
	store := newMap("bad", NewString("nope")) // resolves to a non-Integer
	if _, err := injectHandler([]Value{tmpl, store}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("inject resolving a non-Integer into {:Integer} should reject, got %v", err)
	}
	// A marker resolving to a CONFORMING value is accepted and stays tagged.
	store2 := newMap("bad", NewInteger(7))
	out, oerr := injectHandler([]Value{tmpl, store2}, nil, nil, r)
	if oerr != nil {
		t.Fatalf("conforming inject: %v", oerr)
	}
	if c, ok := out[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("conforming inject result lost its {:Integer} tag")
	}
}

// --- #3: a flex chained-check residual retains the tag ---

// In check mode, a flex {:T}/[:T] container's set residual keeps the element
// tag, so a SECOND write chained on the residual is still flagged — the checker
// mirrors the runtime.
func TestFlexChainedResidualCheck(t *testing.T) {
	r := tcCheck(t, `def xs:[:Integer] (flex [1]) ((xs set 0 2) set 0 "bad")`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("flex chained residual write should be flagged: %+v", r.Check.Diagnostics)
	}
	// A conforming chained write stays silent.
	r2 := tcCheck(t, `def xs:[:Integer] (flex [1]) ((xs set 0 2) set 0 3)`)
	if tcHasTypeError(r2, "does not conform") {
		t.Errorf("conforming flex chained write should be silent: %+v", r2.Check.Diagnostics)
	}
}
