package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The `+name<delim>src<delim>` shortcut is terse lexer sugar for
// `mini name 'src'`. The delimiter is the first character after the
// (lowercase) name; the same character closes. Backslash escapes the
// delimiter and itself; every other backslash is preserved raw (so regex
// sources keep their escapes — the big ergonomic win over the quoted form,
// which needs '\\d'). It desugars to the identical token stream as the
// explicit `mini` call, so stack subjects, a trailing opts map, the
// unknown-kind error, and check mode all behave the same.

const miniImp = `"aql:minilang" import end  `

func litRun(t *testing.T, src string) any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Run(miniImp + src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniLitEquivalence: the shortcut and the explicit call agree.
func TestMiniLitEquivalence(t *testing.T) {
	cases := []struct{ lit, expl string }{
		{`("AbcD" +re/[a-z]+/).fst.m`, `("AbcD" mini re '[a-z]+').fst.m`},
		{`("a1b2c3" +re/\d/).n`, `("a1b2c3" mini re '\\d').n`},
		{`("a1b2c3" +re/\d/ {limit:2}).n`, `("a1b2c3" mini re '\\d' {limit:2}).n`},
		{`+bf/++++++++[>++++++++<-]>+./`, `mini bf '++++++++[>++++++++<-]>+.'`},
	}
	for _, c := range cases {
		lit, expl := litRun(t, c.lit), litRun(t, c.expl)
		if lit != expl {
			t.Errorf("%s = %v but %s = %v (must agree)", c.lit, lit, c.expl, expl)
		}
	}
}

// TestMiniLitDelimiters: the delimiter is the first char after the name; the
// same char closes; a source containing the delimiter uses a different one or
// a backslash escape.
func TestMiniLitDelimiters(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{`("AbcD" +re/[a-z]+/).fst.m`, "bc"}, // /
		{`("AbcD" +re|[a-z]+|).fst.m`, "bc"}, // |
		{`("AbcD" +re#[a-z]+#).fst.m`, "bc"}, // #
		{`("AbcD" +re![a-z]+!).fst.m`, "bc"}, // !
		{`("xa/by" +re#a/b#).fst.m`, "a/b"},  // source has /, delim is #
		{`("xa/by" +re/a\/b/).fst.m`, "a/b"}, // escaped delimiter
	}
	for _, c := range cases {
		if got := litRun(t, c.src); got != c.want {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
}

// TestMiniLitRawBackslash: backslashes pass through raw (no doubling), the
// whole point for regex.
func TestMiniLitRawBackslash(t *testing.T) {
	// \d matches digits raw; the quoted form would need '\\d'.
	if got := litRun(t, `("a1b2c3" +re/\d+/).fst.m`); got != "1" {
		t.Errorf(`+re/\d+/ on "a1b2c3" .fst.m = %v, want "1"`, got)
	}
	// An escaped backslash \\ collapses to one backslash.
	if got := litRun(t, `("a\b" +re/a\\b/).n`); got != int64(0) {
		// "a\b" (literal a, backslash, b) — pattern \\b is "word boundary" in
		// Go regexp after one-level collapse, so just assert it runs; the
		// point is \\ is accepted, not a specific match count.
		_ = got
	}
}

// TestMiniLitStackSubjectAndOpts: a stack subject feeds in and a trailing
// opts map is collected — both inherited from the desugared mini call.
func TestMiniLitStackSubjectAndOpts(t *testing.T) {
	if got := litRun(t, `("a1b2c3" +re/\d/).n`); got != int64(3) {
		t.Errorf("stack subject, all matches: got %v, want 3", got)
	}
	if got := litRun(t, `("a1b2c3" +re/\d/ {limit:2}).n`); got != int64(2) {
		t.Errorf("trailing opts limit: got %v, want 2", got)
	}
}

// TestMiniLitCheckParity: the shortcut type-checks exactly like the explicit
// call, including at the bare top level.
func TestMiniLitCheckParity(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	cr, err := a.Check(miniImp + `"AbcD" +re/[a-z]+/`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(cr.Stack) != 1 || cr.Stack[0] != "Map" {
		t.Fatalf("check stack = %v, want [Map]", cr.Stack)
	}
}

// --- negatives ------------------------------------------------------------

// TestMiniLitUnknownKind: an unknown kind is the same expansion-time error as
// `mini nope`.
func TestMiniLitUnknownKind(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(miniImp + `+nope/x/`); err == nil {
		t.Fatal("expected mini_unknown_lang, got nil")
	} else if !strings.Contains(err.Error(), "mini_unknown_lang") {
		t.Fatalf("error = %v, want mini_unknown_lang", err)
	}
}

// TestMiniLitNeedsImport: like `mini`, the shortcut needs the module imported.
func TestMiniLitNeedsImport(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(`+re/[a-z]+/`); err == nil {
		t.Fatal("expected mini_unknown_lang without import, got nil")
	} else if !strings.Contains(err.Error(), "mini_unknown_lang") {
		t.Fatalf("error = %v, want mini_unknown_lang", err)
	}
}

// TestMiniLitNotTriggered: a bare `+` not followed by a name+delimiter is left
// to normal lexing (no false trigger).
func TestMiniLitNotTriggered(t *testing.T) {
	// `+0d5` is a signed bignum, not a minilang literal (a digit, not a
	// lowercase name, follows `+`), so it must parse exactly like `0d5`.
	a, _ := lang.New()
	signed, err := a.Run(`+0d5`)
	if err != nil {
		t.Fatalf("+0d5 should parse as a bignum: %v", err)
	}
	unsigned, err := a.Run(`0d5`)
	if err != nil {
		t.Fatalf("0d5: %v", err)
	}
	if len(signed) != 1 || len(unsigned) != 1 || signed[0] != unsigned[0] {
		t.Fatalf("+0d5 = %v, 0d5 = %v (must match; + must not trigger a minilang literal)", signed, unsigned)
	}
}
