package parser

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
)

// --- `/` modifiers on dotted paths and paren groups ---

// TestReachWave3WordModifiers pins the word-modifier placement rule: /u /s
// /f /N desugar to higher-order words emitted BEFORE the reach group so
// they forward-collect the result.
func TestReachWave3WordModifiers(t *testing.T) {
	cases := []struct {
		src   string
		words []string // leading word tokens before the reach
	}{
		{"a.b/u", []string{"usurp"}},
		{"a.b/s", []string{"stack-args"}},
		{"a.b/f", []string{"forward-args"}},
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != len(c.words)+1 {
			t.Fatalf("%q: got %d values, want %d: %v", c.src, len(vals), len(c.words)+1, vals)
		}
		for i, name := range c.words {
			if w := wordNameWave3(t, vals[i], c.src); w.Name != name {
				t.Errorf("%q: value %d word %q, want %q", c.src, i, w.Name, name)
			}
		}
		if !eng.IsReach(vals[len(c.words)]) {
			t.Errorf("%q: last value is not a Reach: %v", c.src, vals[len(c.words)])
		}
	}
}

func TestReachWave3ForceArityModifier(t *testing.T) {
	// a.b/2 → force-arity 2 (a.b)
	vals := mustParseWave3(t, "a.b/2")
	if len(vals) != 3 {
		t.Fatalf("a.b/2: got %d values, want 3: %v", len(vals), vals)
	}
	if w := wordNameWave3(t, vals[0], "a.b/2"); w.Name != "force-arity" {
		t.Errorf("a.b/2: first word %q, want force-arity", w.Name)
	}
	if n, err := eng.AsInteger(vals[1]); err != nil || n != 2 {
		t.Errorf("a.b/2: second value %v, want Integer 2", vals[1])
	}
	if !eng.IsReach(vals[2]) {
		t.Errorf("a.b/2: third value is not a Reach: %v", vals[2])
	}
}

// TestReachWave3MarkerModifiers pins /r and /q: the Word/__DM marker is
// emitted AFTER the group, both fused (`a.b/r`) and standalone (`m.k /r`).
func TestReachWave3MarkerModifiers(t *testing.T) {
	for _, src := range []string{"a.b/r", "a.b/q", "m.k /r"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2: %v", src, len(vals), vals)
		}
		if !eng.IsReach(vals[0]) {
			t.Errorf("%q: first value is not a Reach: %v", src, vals[0])
		}
		if !eng.IsDispatchMod(vals[1]) {
			t.Errorf("%q: second value is not a Word/__DM marker: %v", src, vals[1])
		}
	}
}

// TestReachWave3TypeBoundKey pins `/t` on the final key: groupModifier
// declines (/t is word-only sugar), so the key itself desugars to the
// `(Type of [b/q])` paren expression inside the reach.
func TestReachWave3TypeBoundKey(t *testing.T) {
	vals := mustParseWave3(t, "a.b/t")
	if len(vals) != 1 || !eng.IsReach(vals[0]) {
		t.Fatalf("a.b/t: expected one Reach, got %v", vals)
	}
	if s := vals[0].String(); !strings.Contains(s, "Type of") {
		t.Errorf("a.b/t: rendering %q lacks the Type-of key", s)
	}
}

// TestParenWave3StandaloneModifier pins a standalone `/mod` token applying
// to the preceding paren group's result.
func TestParenWave3StandaloneModifier(t *testing.T) {
	// Word modifier prefixes the group.
	vals := mustParseWave3(t, "(1 add 2)/s")
	if len(vals) != 2 {
		t.Fatalf("(1 add 2)/s: got %d values, want 2: %v", len(vals), vals)
	}
	if w := wordNameWave3(t, vals[0], "(1 add 2)/s"); w.Name != "stack-args" {
		t.Errorf("(1 add 2)/s: first word %q, want stack-args", w.Name)
	}
	if !eng.IsParenExpr(vals[1]) {
		t.Errorf("(1 add 2)/s: second value is not a ParenExpr: %v", vals[1])
	}
	// Marker modifier suffixes the group.
	vals = mustParseWave3(t, "(f)/r")
	if len(vals) != 2 {
		t.Fatalf("(f)/r: got %d values, want 2: %v", len(vals), vals)
	}
	if !eng.IsParenExpr(vals[0]) || !eng.IsDispatchMod(vals[1]) {
		t.Errorf("(f)/r: want [ParenExpr, Word/__DM], got %v", vals)
	}
}

// --- computed / quoted keys, paren receivers ---

func TestReachWave3ComputedKey(t *testing.T) {
	vals := mustParseWave3(t, "m.(x add 1)")
	if len(vals) != 1 || !eng.IsReach(vals[0]) {
		t.Fatalf("m.(x add 1): expected one Reach, got %v", vals)
	}
	info, err := eng.AsReach(vals[0])
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if len(info.Segments) != 1 || !info.Segments[0].Computed {
		t.Fatalf("m.(x add 1): expected one computed segment, got %+v", info.Segments)
	}
	if len(info.Segments[0].KeyExpr) != 3 {
		t.Errorf("m.(x add 1): key expr has %d tokens, want 3", len(info.Segments[0].KeyExpr))
	}
}

func TestReachWave3QuotedKey(t *testing.T) {
	vals := mustParseWave3(t, "m.'k k'")
	if len(vals) != 1 || !eng.IsReach(vals[0]) {
		t.Fatalf("m.'k k': expected one Reach, got %v", vals)
	}
	info, _ := eng.AsReach(vals[0])
	if len(info.Segments) != 1 {
		t.Fatalf("m.'k k': got %d segments, want 1", len(info.Segments))
	}
	if s, err := eng.AsString(info.Segments[0].KeyLit); err != nil || s != "k k" {
		t.Errorf("m.'k k': key literal %v, want string \"k k\"", info.Segments[0].KeyLit)
	}
}

func TestReachWave3Errors(t *testing.T) {
	// Errors inside the receiver, a computed key, or a literal key all
	// surface cleanly.
	wantParseErrWave3(t, "(1__0).a", "misplaced `_`")
	wantParseErrWave3(t, "m.(1__0)", "misplaced `_`")
	wantParseErrWave3(t, "m.1e", "invalid numeric literal")
}

// --- dot-chains in map values (the dotchain grammar rule) ---

func TestReachWave3MapValueDotChains(t *testing.T) {
	// A getr segment (`!.`), a two-segment chain, and a QUOTED key in
	// map-value position all fold into the same paren group the explicit
	// form produces.
	for src, want := range map[string]string{
		"{a: m!.k}":    "(m !).k",
		"{a: m.k1.k2}": "(m.k1).k2",
		"{a: m.'k k'}": "m.'k k'",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, want) {
			t.Errorf("%q: rendering %q does not contain %q", src, s, want)
		}
	}
}

// --- unclosed parens ---

func TestParenWave3Unclosed(t *testing.T) {
	// In an item run, and in data (map-value) context.
	wantParseErrWave3(t, "1 (2", "unmatched opening parenthesis")
	wantParseErrWave3(t, "{a:(1", "unmatched opening parenthesis")
	// Errors inside a data-context paren group surface too.
	wantParseErrWave3(t, "{a:(1__0)}", "misplaced `_`")
	// …and inside a paren group qualified by a standalone modifier.
	wantParseErrWave3(t, "(1__0)/s", "misplaced `_`")
}

// --- minilang literals ---

func TestMiniLitWave3Inline(t *testing.T) {
	// In word context the literal expands INLINE to `mini name 'src'`.
	cases := []struct{ src, name, mini string }{
		{`1 +re/x/`, "re", "x"},
		{`1 +re/a\/b/`, "re", "a/b"},           // \<delim> → delim
		{`1 +re/a\\b/`, "re", `a\b`},           // \\ → backslash
		{`1 +re/a\ b/`, "re", "a b"},           // \<space> → space
		{`1 +re/a\db/`, "re", `a\db`},          // other escapes stay raw
		{`1 +my-kind2:abc`, "my-kind2", "abc"}, // open form, digit+dash name
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 4 {
			t.Fatalf("%q: got %d values, want 4: %v", c.src, len(vals), vals)
		}
		if w := wordNameWave3(t, vals[1], c.src); w.Name != "mini" {
			t.Errorf("%q: expected the word `mini`, got %q", c.src, w.Name)
		}
		if w := wordNameWave3(t, vals[2], c.src); w.Name != c.name {
			t.Errorf("%q: kind word %q, want %q", c.src, w.Name, c.name)
		}
		if s, err := eng.AsString(vals[3]); err != nil || s != c.mini {
			t.Errorf("%q: source %v, want string %q", c.src, vals[3], c.mini)
		}
	}
}

func TestMiniLitWave3SpliceForms(t *testing.T) {
	// A single top-level literal, and a data-context (map value) literal,
	// both become the `mini name 'src'` splice.
	for _, src := range []string{`+re/x/`, `{a:+re/x/}`} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		v := vals[0]
		if m, err := eng.AsMap(v); err == nil {
			got, ok := m.Get("a")
			if !ok {
				t.Fatalf("%q: map lacks key a", src)
			}
			v = got
		}
		if !eng.IsSplice(v) {
			t.Fatalf("%q: expected a splice value, got %s", src, v.String())
		}
		if s := v.String(); !strings.Contains(s, "mini") || !strings.Contains(s, "re") {
			t.Errorf("%q: splice rendering %q lacks the mini call", src, s)
		}
	}
}

func TestMiniLitWave3Declines(t *testing.T) {
	// An empty open source is NOT a literal — the token stays a plain word.
	vals := mustParseWave3(t, `+re/`)
	if len(vals) != 1 {
		t.Fatalf("+re/: got %d values, want 1", len(vals))
	}
	if w := wordNameWave3(t, vals[0], "+re/"); w.Name != "+re/" {
		t.Errorf("+re/: want the whole token as a word, got %q", w.Name)
	}
	// A missing delimiter (EOF or whitespace right after the name) also
	// declines: `+re` stays a plain word.
	for _, c := range []struct {
		src  string
		idx  int
		want string
	}{{"1 +re", 1, "+re"}, {"+re x", 0, "+re"}} {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2", c.src, len(vals))
		}
		if w := wordNameWave3(t, vals[c.idx], c.src); w.Name != c.want {
			t.Errorf("%q: got word %q, want %q", c.src, w.Name, c.want)
		}
	}
}

func TestMiniLitWave3OpenFormStopsAtWhitespace(t *testing.T) {
	// The open form ends at the first unescaped whitespace; following
	// tokens are untouched.
	vals := mustParseWave3(t, "1 +email:alice@x.com 2")
	if len(vals) != 5 {
		t.Fatalf("open-form mini: got %d values, want 5: %v", len(vals), vals)
	}
	if s, err := eng.AsString(vals[3]); err != nil || s != "alice@x.com" {
		t.Errorf("open-form mini: source %v, want alice@x.com", vals[3])
	}
	if n, err := eng.AsInteger(vals[4]); err != nil || n != 2 {
		t.Errorf("open-form mini: trailing token %v, want Integer 2", vals[4])
	}
}

// --- arrow (=>) folds ---

func TestArrowWave3Folds(t *testing.T) {
	// Word-context fold after a preceding value.
	vals := mustParseWave3(t, "1 x => [x]")
	if len(vals) != 2 {
		t.Fatalf("1 x => [x]: got %d values, want 2: %v", len(vals), vals)
	}
	if !eng.IsParenExpr(vals[1]) {
		t.Fatalf("1 x => [x]: expected a folded ParenExpr, got %v", vals[1])
	}
	if s := vals[1].String(); !strings.Contains(s, "afn") {
		t.Errorf("1 x => [x]: fold %q lacks afn", s)
	}

	// Top-level bare-pair fold (elem-level): the documented def-lambda form.
	vals = mustParseWave3(t, "def double x:Integer => [x mul 2]")
	if len(vals) != 3 {
		t.Fatalf("def-lambda: got %d values, want 3: %v", len(vals), vals)
	}
	if !eng.IsParenExpr(vals[2]) {
		t.Fatalf("def-lambda: expected the pair to fold into a ParenExpr, got %v", vals[2])
	}
	if s := vals[2].String(); !strings.Contains(s, "x:word(Integer)") || !strings.Contains(s, "afn") {
		t.Errorf("def-lambda: fold %q lacks the sig/afn shape", s)
	}

	// Pair fold inside an explicit list, and paren-context pair dive.
	for _, src := range []string{"[a:1 => [1]]", "[x:Integer => [x]]", "(x:Integer => [x mul 2])"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
		if s := vals[0].String(); !strings.Contains(s, "afn") {
			t.Errorf("%q: %q lacks the afn fold", src, s)
		}
	}

	// A dotted receiver: the arrow still parses (the reach folds first),
	// in flat, spaced, and paren-wrapped forms.
	for _, src := range []string{"m.k => [1]", "(m . k => [1])", "(1 x => [x])"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
	}
}

func TestArrowWave3Degenerate(t *testing.T) {
	// A trailing arrow with no body parses without panicking: the sig and
	// the afn word remain as flat tokens (nothing is silently invented).
	vals := mustParseWave3(t, "x =>")
	if len(vals) != 2 {
		t.Fatalf("x =>: got %d values, want 2: %v", len(vals), vals)
	}
	if w := wordNameWave3(t, vals[1], "x =>"); w.Name != "afn" {
		t.Errorf("x =>: second value %q, want afn", w.Name)
	}
	// A pair arrow with no body inside a list also parses (empty fold),
	// as does a plain-value arrow with no body.
	for _, src := range []string{"[x:1 =>]", "[1 x =>]"} {
		if _, err := parseWave3(t, src); err != nil {
			t.Fatalf("%q: unexpected error: %v", src, err)
		}
	}
	// Inside a paren, a bodyless arrow leaves the group unfinished — the
	// unmatched-paren diagnostic fires (no panic).
	wantParseErrWave3(t, "(1 x =>)", "unmatched opening parenthesis")
	// A top-level bare pair with arrow and body parses flat or folded —
	// never an error.
	if _, err := parseWave3(t, "x:Integer => [x]"); err != nil {
		t.Fatalf("x:Integer => [x]: unexpected error: %v", err)
	}
	// An arrow in an EXPLICIT map value stays a syntax error.
	wantParseErrWave3(t, "{a: 1 => 2}", "")
}
