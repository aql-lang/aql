package formatter

import "testing"

// TestNameSlotIsPreserved pins the guard shared by the two spelling
// rewriters (normaliseStatementEnd, capitalizeTypesInTree): a bare word in a
// NAME slot — a map key or an accessor field — is data, and rewriting its
// spelling names something else or nothing at all.
//
// Every negative row here is output the parser REJECTS or silently reads
// differently, which is the sharp end of a formatter bug: `fmt` is supposed
// to be meaning-preserving, so a wrong rewrite here corrupts working source.
// Two of the four positions shipped unguarded — the optional key (`?` sits
// between the word and its colon) and the strict-dot read (`!` breaks the
// `.`-merge, leaving a bare word after an NdDot).
func TestNameSlotIsPreserved(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// --- `end` in a name slot stays `end` ---
		{"map key", "{end:1}", "{end:1}\n"},
		{"optional map key", "{end?:1}", "{end?:1}\n"},
		{"accessor word", "m dot end", "m dot end\n"},
		{"strict-dot sugar", "m!.end", "m !.end\n"},

		// --- a type-named word in a name slot is not capitalised ---
		{"type-named key", "{integer:1}", "{integer:1}\n"},
		{"type-named optional key", "{integer?:1}", "{integer?:1}\n"},
		{"type-named accessor word", "m dot integer", "m dot integer\n"},
		{"type-named strict-dot", "m!.integer", "m !.integer\n"},
	} {
		if got := Format(c.src); got != c.want {
			t.Errorf("%s: Format(%q) = %q, want %q", c.name, c.src, got, c.want)
		}
	}
}

// TestTerminatorStillNormalises is the positive half: everywhere that is NOT
// a name slot, `end` is the statement terminator and normalises to `;`
// (STYLE-GUIDE §S6). Without these rows the guard above could be "fixed" by
// disabling the rewrite entirely.
func TestTerminatorStillNormalises(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"between statements", "def a 1 end a", "def a 1 ; a\n"},
		{"after a map that HAS an end key", "def m {end:1} end m", "def m {end:1} ; m\n"},
		{"after a strict-dot end read", "def m {end:1} end m!.end end", "def m {end:1} ; m !.end ;\n"},
		{"trailing", "def a 1 end", "def a 1 ;\n"},
	} {
		if got := Format(c.src); got != c.want {
			t.Errorf("%s: Format(%q) = %q, want %q", c.name, c.src, got, c.want)
		}
	}
}

// TestNameSlotFormatIsIdempotent pins the property that actually matters for
// a formatter: running it twice changes nothing. A guard that merely delays a
// bad rewrite to the second pass would still corrupt source in a repo that
// formats on save.
func TestNameSlotFormatIsIdempotent(t *testing.T) {
	for _, src := range []string{
		"{end?:1}", "m!.end", "{integer?:1}", "m!.integer",
		"def m {end:1} end m!.end end",
	} {
		once := Format(src)
		if twice := Format(once); twice != once {
			t.Errorf("Format not idempotent for %q: %q then %q", src, once, twice)
		}
	}
}

// TestParseTreeMatchesFormat pins ParseTree's documented invariant: emitting
// its tree with the built-in emitter reproduces Format exactly. The two paths
// each applied their own list of tree transforms, and when
// normaliseStatementEnd was added to the format path alone the invariant went
// quietly false — `Fmt.tree` handed declarative rule sets `end` word nodes for
// a terminator Format had already respelled `;`. Both now run
// canonicaliseTree, so a new transform cannot land on one path only.
func TestParseTreeMatchesFormat(t *testing.T) {
	for _, src := range []string{
		"def a 1 end a",
		"def a 1 ; a",
		"def m {end:1} end m!.end end",
		"{end?:1}",
		"def inc fn [[n:Integer] [Integer] [n add 1]] end inc 5",
	} {
		viaTree := newRenderer(DefaultRules()).emitRoot(ParseTree(src), 0)
		if direct := Format(src); viaTree != direct {
			t.Errorf("ParseTree/Format disagree for %q: tree %q, format %q",
				src, viaTree, direct)
		}
	}
}

// TestParseTreeHasNoEndWords is the same defect stated as the property a
// declarative rule set actually depends on: after the transforms there is no
// bare `end` WORD left for a terminator, so a rule table keyed on node kind
// sees NdSemicolon and needs no special case. Name slots are exempt — those
// really are words (pinned by TestNameSlotIsPreserved).
func TestParseTreeHasNoEndWords(t *testing.T) {
	var walk func(n *Node) int
	walk = func(n *Node) int {
		count := 0
		for i, ch := range n.Children {
			if ch.Kind == NdWord && ch.Text == "end" && !isNameSlot(n.Children, i) {
				count++
			}
			count += walk(ch)
		}
		return count
	}
	for _, src := range []string{
		"def a 1 end a",
		"def a 1 end def b 2 end [a b] end",
		"def m {end:1} end m",
	} {
		if n := walk(ParseTree(src)); n != 0 {
			t.Errorf("ParseTree(%q) left %d bare `end` terminator word(s); "+
				"a declarative rule set would have to special-case them", src, n)
		}
	}
}
