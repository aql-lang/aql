package parser

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// TestTemplateWave3Escapes pins every escape sequence in template literal
// text, including the unknown-escape passthrough.
func TestTemplateWave3Escapes(t *testing.T) {
	cases := []struct{ src, want string }{
		{"`a\\nb`", "a\nb"},
		{"`a\\tb`", "a\tb"},
		{"`a\\rb`", "a\rb"},
		{"`a\\\\b`", "a\\b"},
		{"`a\\`b`", "a`b"},
		{"`a\\$b`", "a$b"},
		{"`a\\zb`", "a\\zb"}, // unknown escape kept as-is
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", c.src, len(vals))
		}
		s, err := eng.AsString(vals[0])
		if err != nil {
			t.Fatalf("%q: AsString: %v", c.src, err)
		}
		if s != c.want {
			t.Errorf("%q: got %q, want %q", c.src, s, c.want)
		}
	}
}

// TestProcessTemplateEscapesWave3 covers the escape processor directly,
// including the corner the lexer can't easily produce: a trailing backslash.
func TestProcessTemplateEscapesWave3(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		`a\nb\tc\rd`: "a\nb\tc\rd",
		`a\\b`:       `a\b`,
		"a\\`b":      "a`b",
		`a\$b`:       "a$b",
		`a\qb`:       `a\qb`,
		`tail\`:      `tail\`, // lone trailing backslash is preserved
	}
	for in, want := range cases {
		if got := processTemplateEscapes(in); got != want {
			t.Errorf("processTemplateEscapes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplateWave3Multiline(t *testing.T) {
	vals := mustParseWave3(t, "`line1\nline2`")
	if len(vals) != 1 {
		t.Fatalf("multiline template: got %d values, want 1", len(vals))
	}
	if s, _ := eng.AsString(vals[0]); s != "line1\nline2" {
		t.Errorf("multiline template: got %q", s)
	}
}

func TestTemplateWave3Boundaries(t *testing.T) {
	// Empty template.
	vals := mustParseWave3(t, "``")
	if len(vals) != 1 {
		t.Fatalf("``: got %d values, want 1", len(vals))
	}
	if s, _ := eng.AsString(vals[0]); s != "" {
		t.Errorf("``: got %q, want empty string", s)
	}
	// Unterminated with text: auto-closes at EOF with the literal.
	vals = mustParseWave3(t, "`abc")
	if len(vals) != 1 {
		t.Fatalf("`abc: got %d values, want 1", len(vals))
	}
	if s, _ := eng.AsString(vals[0]); s != "abc" {
		t.Errorf("`abc: got %q, want abc", s)
	}
	// A lone backtick is rejected by the grammar (no panic).
	wantParseErrWave3(t, "`", "")
	// Unterminated WITH a hole: auto-closes to an InterpString.
	for _, src := range []string{"`${x}", "`${x"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !eng.IsInterpString(vals[0]) {
			t.Errorf("%q: expected one InterpString, got %v", src, vals)
		}
	}
}

// TestTemplateWave3InterpNested pins interpolated templates inside paren
// groups, explicit lists, and map values (each context arms the interp
// grammar with different dlist/dmap state).
func TestTemplateWave3InterpNested(t *testing.T) {
	for _, src := range []string{"(`${1}`)", "[`${1}`]", "{a:`${1}`}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
	}
	// A template with NO holes collapses to a plain string even nested.
	for _, src := range []string{"[`x`]", "{a:`x`}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
	}
	// An interpolated map value stays an InterpString.
	vals := mustParseWave3(t, "{a:`${1}`}")
	m, err := eng.AsMap(vals[0])
	if err != nil {
		t.Fatalf("{a:`${1}`}: AsMap: %v", err)
	}
	v, ok := m.Get("a")
	if !ok || !eng.IsInterpString(v) {
		t.Errorf("{a:`${1}`}: value %v is not an InterpString", v)
	}
}

func TestTemplateWave3InterpErrors(t *testing.T) {
	// A malformed expression inside ${} surfaces the inner error.
	wantParseErrWave3(t, "`${1__0}`", "interpolation expression error")
	// Empty ${} holes are rejected by the grammar (no panic).
	wantParseErrWave3(t, "`${}`", "")
	wantParseErrWave3(t, "`${} tail`", "")
}
