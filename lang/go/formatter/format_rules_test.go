package formatter

import "testing"

// TestDefaultRulesReproducesFormat pins the authority of the rule table:
// interpreting DefaultRules reproduces Format byte-for-byte — the
// stylesheet ENCODES the canonical style, the processor merely interprets
// it.
func TestDefaultRulesReproducesFormat(t *testing.T) {
	srcs := []string{
		"def x 1",
		"[1, 2, 3]",
		"{a:1 b:{c:2}}",
		"def handler fn [[request:Map response:Map] [Map] [request merge response merge {status:200}]]",
		"{a:1 # note\nb:2}",
		"# comment\n\ncode here\n",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen",
		"input.(foo)",
	}
	for _, src := range srcs {
		if got, want := FormatRules(src, DefaultRules()), Format(src); got != want {
			t.Errorf("FormatRules(DefaultRules) diverges from Format for %q:\n got: %q\nwant: %q", src, got, want)
		}
	}
}

// TestFormatRulesParameterized proves every rule-table dimension is
// genuinely consulted by the processor: changing it changes the output in
// the specified way, and the default output differs (the negative half —
// the rules are not decorative).
func TestFormatRulesParameterized(t *testing.T) {
	d := DefaultRules()

	w20 := d
	w20.Width = 20
	i4 := d
	i4.Indent = 4
	ang := d
	ang.ListOpen, ang.ListClose = "<", ">"
	noStrat := d
	noStrat.Strategies = nil
	noFn := d
	noFn.Strategies = []string{"comment-only", "inline", "trailing-comment", "trailing-container", "wrap"}
	fnw := d
	fnw.FnWord = "lambda"
	noStarts := d
	noStarts.StmtStartWords = nil
	commaNone := d
	commaNone.AttachPrev = []NodeKind{NdColon, NdQuestion, NdDot}
	semiPrev := d
	semiPrev.AttachPrev = append([]NodeKind{NdSemicolon}, d.AttachPrev...)
	noDotSuffix := d
	noDotSuffix.AttachDotSuffix = false

	longFn := "def handler fn [[request:Map response:Map] [Map] [request merge response merge {status:200}]]"
	longList := "[def alpha 100 def bravo 200 def charlie 300 def delta 400 def echo 500 def fox 600]"

	cases := []struct {
		name string
		ru   Rules
		src  string
		want string
	}{
		// width drives both container layout and the greedy fill.
		{"width 20 list", w20, "[alpha bravo charlie delta]",
			"[alpha bravo\n      charlie delta\n  ]\n"},
		{"width 20 fill", w20, "one two three four five six",
			"one two three four\n  five six\n"},
		// indent shifts container bodies and wrap continuations.
		{"indent 4 map", i4, "{a:1 # note\nb:2}",
			"{a:1\n    # note\n    b:2\n}\n"},
		{"indent 2 map (default)", d, "{a:1 # note\nb:2}",
			"{a:1\n  # note\n  b:2\n}\n"},
		{"indent 4 continuation", i4, "def f fn [[a:Integer b:Integer c:Integer] [Integer] [a add b add c]] end f 1 2 3",
			"def f fn [[a:Integer b:Integer c:Integer] [Integer] [a add b add c]]\n    end f 1 2 3\n"},
		// container brackets are data.
		{"angle list brackets", ang, "[1 2]", "<1 2>\n"},
		// no strategies → the statement stays inline regardless of width.
		{"no strategies stays inline", noStrat,
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen",
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen\n"},
		// the fn template is a strategy: present → fn layout; absent (or a
		// different trigger word) → the trailing-container template claims it.
		{"fn strategy on", d, longFn,
			"def handler fn [[request:Map response:Map] [Map] [\n  request merge response merge {status:200}\n]]\n"},
		{"fn strategy removed", noFn, longFn,
			"def handler fn [\n  [request:Map response:Map] [Map]\n    [request merge response merge {status:200}]\n]\n"},
		{"fn trigger word changed", fnw, longFn,
			"def handler fn [\n  [request:Map response:Map] [Map]\n    [request merge response merge {status:200}]\n]\n"},
		// statement-start words drive list grouping.
		{"statement-starts group", d, longList,
			"[def alpha 100\n    def bravo 200\n    def charlie 300\n    def delta 400\n    def echo 500\n    def fox 600\n  ]\n"},
		{"statement-starts removed", noStarts, longList,
			"[def alpha 100 def bravo 200 def charlie 300 def delta 400 def echo\n      500 def fox 600\n  ]\n"},
		// attach classes own the inter-token whitespace.
		{"comma detached", commaNone, "a, b", "a , b\n"},
		{"comma attached (default)", d, "a, b", "a, b\n"},
		{"semicolon attached", semiPrev, "a; b", "a; b\n"},
		{"semicolon detached (default)", d, "a; b", "a ; b\n"},
		{"dot-suffix off", noDotSuffix, "input.(foo)", "input. (foo)\n"},
		{"dot-suffix on (default)", d, "input.(foo)", "input.(foo)\n"},
	}
	for _, c := range cases {
		if got := FormatRules(c.src, c.ru); got != c.want {
			t.Errorf("%s: FormatRules(%q) = %q, want %q", c.name, c.src, got, c.want)
		}
	}
}

// TestFormatRulesUnknownStrategyInert pins the processor's dispatch
// contract: an unknown strategy name is skipped (later strategies still
// run), and a table with ONLY unknown names falls back to inline.
func TestFormatRulesUnknownStrategyInert(t *testing.T) {
	d := DefaultRules()
	d.Strategies = []string{"bogus", "inline"}
	if got := FormatRules("def  x  1", d); got != "def x 1\n" {
		t.Errorf("unknown strategy not skipped: %q", got)
	}
	d.Strategies = []string{"bogus"}
	long := "one two three four five six seven eight nine ten eleven twelve thirteen fourteen"
	if got := FormatRules(long, d); got != long+"\n" {
		t.Errorf("all-unknown strategies should fall back to inline: %q", got)
	}
}

// TestKindByName pins the name → NodeKind boundary a data-expressed rule
// table crosses, plus its rejection of unknown names.
func TestKindByName(t *testing.T) {
	for k := NdRoot; k <= NdNewline; k++ {
		got, ok := KindByName(NodeKindName(k))
		if !ok || got != k {
			t.Errorf("KindByName(%q) = (%v,%v), want (%v,true)", NodeKindName(k), got, ok, k)
		}
	}
	if _, ok := KindByName("nonesuch"); ok {
		t.Error("KindByName accepted an unknown kind name")
	}
}

// TestStrategyNamesMatchDefault pins that the canonical rule table uses
// exactly the known strategy set, in the canonical order.
func TestStrategyNamesMatchDefault(t *testing.T) {
	names := StrategyNames()
	def := DefaultRules().Strategies
	if len(names) != len(def) {
		t.Fatalf("StrategyNames len %d != DefaultRules().Strategies len %d", len(names), len(def))
	}
	for i := range names {
		if names[i] != def[i] {
			t.Errorf("strategy %d: %q != %q", i, names[i], def[i])
		}
	}
}

// TestRulesProcessorTotal pins the processor's no-panic totality on
// degenerate tables: the zero-value Rules and a negative indent must
// format without panicking (pad clamps, empty strategy list falls back to
// inline).
func TestRulesProcessorTotal(t *testing.T) {
	if got := FormatRules("a b", Rules{}); got != "a b\n" {
		t.Errorf("zero-value Rules: %q, want %q", got, "a b\n")
	}
	neg := DefaultRules()
	neg.Indent = -2
	neg.Width = 10
	// Forces wrap with a negative continuation indent; pad clamps to 0, so
	// each continuation line starts at column zero instead of panicking.
	if got := FormatRules("alpha bravo charlie", neg); got != "alpha\nbravo\ncharlie\n" {
		t.Errorf("negative indent: %q", got)
	}
	if pad(-3) != "" {
		t.Errorf("pad(-3) = %q, want empty", pad(-3))
	}
	// A comma-only map under a hostile width must not panic on the empty
	// entry list — it keeps the single-line render.
	zw := DefaultRules()
	zw.Width = 0
	if got := FormatRules("{,}", zw); got != "{}\n" {
		t.Errorf("comma-only map at width 0: %q, want %q", got, "{}\n")
	}
	// An absurd indent is clamped to maxPad, never an unbounded allocation.
	if got := pad(maxPad + 1000); len(got) != maxPad {
		t.Errorf("pad(maxPad+1000) len = %d, want %d", len(got), maxPad)
	}
}

// TestTrailingContainerStrategyDirect covers the trailing-container
// template's single-line branches, reachable when a rule table runs it
// BEFORE the inline template (under the canonical order, inline claims
// anything that fits first). The comment-bearing cases pin the safety
// guard: a line comment inside the container must force multi-line, never
// a single line that would swallow the closing bracket.
func TestTrailingContainerStrategyDirect(t *testing.T) {
	ru := DefaultRules()
	ru.Strategies = []string{"trailing-container", "wrap"}
	cases := []struct{ src, want string }{
		{"x {a:1}", "x {a:1}\n"},
		{"x [1 2]", "x [1 2]\n"},
		{"x {a:1 # c\nb:2}", "x {\n  a:1\n  # c\n  b:2\n}\n"},
		{"x [1 # c\n2]", "x [\n  1 # c\n  2\n]\n"},
	}
	for _, c := range cases {
		if got := FormatRules(c.src, ru); got != c.want {
			t.Errorf("trailing-container-first %q = %q, want %q", c.src, got, c.want)
		}
	}
}
