package formatter

import (
	"strings"
	"testing"
)

// idempotentSeam7 asserts Format is a fixed point on already-formatted
// output. Named distinctly from the package's other idempotence helper so
// the two test files coexist without a redeclaration clash.
func idempotentSeam7(t *testing.T, src string) {
	t.Helper()
	first := Format(src)
	if second := Format(first); first != second {
		t.Errorf("not idempotent for %q:\n first:  %q\n second: %q", src, first, second)
	}
}

// TestFormatRBraceUnwindSeam7 covers buildTree's TokRBrace unwind loop
// (format.go:308) — a `}` that closes while an inner list/paren is still
// open pops the intervening container(s) before closing the map. This is
// the brace-side mirror of the existing rbracket-closes-open-map case.
func TestFormatRBraceUnwindSeam7(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"brace closes open list", "{[a}\n", "{[a]}\n"},
		{"brace closes bare list", "[a}\n", "[a]\n"},
		{"brace closes open paren", "{(a}\n", "{(a)}\n"},
		// Boundary: a matched brace with no intervening container never
		// enters the unwind loop.
		{"matched brace no unwind", "{a:1}\n", "{a:1}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.in); got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatMapCommasSeam7 covers parseMapEntries' comma-skip arm
// (format.go:891) — commas between map entries are consumed and dropped,
// since nonTrivial keeps them in the child list (it only filters
// newlines). The rendered map is comma-free regardless of the input.
func TestFormatMapCommasSeam7(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single comma", "{a:1, b:2}\n", "{a:1 b:2}\n"},
		{"spaced commas", "{a:1 , b:2 , c:3}\n", "{a:1 b:2 c:3}\n"},
		// Boundary: no comma at all still renders the same.
		{"no comma", "{a:1 b:2}\n", "{a:1 b:2}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.in); got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
			idempotentSeam7(t, tt.in)
		})
	}
}

// TestFormatFnTrailingNotDropped covers tryFnFormat's two decline guards:
// a fn wrapper followed by more tokens, and a multi-overload wrapper.
// tryFnFormat reformats only `header [args][ret][body]`, so applying it to
// either shape would SILENTLY DROP content (the trailing tokens, or every
// overload past the first). Both must decline to the general path, which
// preserves every token. A multi-parameter fn is used so the spec-list form
// is kept and tryFnFormat is a candidate (a single-param fn takes the
// bracketless form).
func TestFormatFnTrailingNotDropped(t *testing.T) {
	// Trailing tokens after the wrapper are preserved, never truncated.
	src := "def n fn [[a:Integer b:Integer] [Integer] [a]] one two three four five six seven eight\n"
	got := Format(src)
	for _, tok := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		if !strings.Contains(got, tok) {
			t.Errorf("trailing token %q was dropped:\n%s", tok, got)
		}
	}
	if !strings.Contains(got, "def n fn [[a:Integer b:Integer] [Integer] [a]]") {
		t.Errorf("the fn def itself was mangled:\n%s", got)
	}
	// The formatted output is a fixed point.
	idempotentSeam7(t, got)

	// A multi-overload wrapper (no trailing tokens, but long enough to reach
	// tryFnFormat) hits the len(inner) != 3 guard and keeps every overload.
	multi := "def g fn [[a:Integer b:Integer] [String] [\"first\"] [a:String b:String] [String] [\"second\"]]\n"
	gotMulti := Format(multi)
	for _, tok := range []string{"[a:Integer b:Integer]", "[a:String b:String]", "\"first\"", "\"second\""} {
		if !strings.Contains(gotMulti, tok) {
			t.Errorf("multi-overload fmt dropped %q:\n%s", tok, gotMulti)
		}
	}
}

// TestFormatFnBodyGroupWrapSeam7 covers tryFnFormat's multi-line body
// path where a single body group itself exceeds the width and falls to
// wrapStatement (format.go:552-554).
func TestFormatFnBodyGroupWrapSeam7(t *testing.T) {
	src := "def f fn [[x:Integer y:Integer] [Integer] [x add x add x add x add x add x add x add x add x add x add x add x add x]]\n"
	want := "def f fn [[x:Integer y:Integer] [Integer] [\n" +
		"  x add x add x add x add x add x add x add x add x add x add x add x\n" +
		"    add x\n" +
		"]]\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
}

// TestFormatWrapAttachSeam7 covers wrapStatement's attach arm
// (format.go:716-719): when a long plain statement wraps, an attaching
// token (a comma here) glues onto the token before it rather than
// starting a fresh part.
func TestFormatWrapAttachSeam7(t *testing.T) {
	src := "alpha, bravo, charlie, delta, echo, foxtrot, golf, hotel, india, juliet, kilo, lima, mike\n"
	want := "alpha, bravo, charlie, delta, echo, foxtrot, golf, hotel, india, juliet,\n" +
		"  kilo, lima, mike\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	// Every emitted line stays within the width.
	for _, line := range strings.Split(want, "\n") {
		if len(line) > maxLineWidth {
			t.Errorf("line exceeds %d chars: %q", maxLineWidth, line)
		}
	}
}

// TestFormatListNonFirstGroupWrapSeam7 covers emitList's multi-line path
// where a non-first group (gi != 0) is itself too long and is emitted via
// wrapStatement without the leading-`[` prepend (format.go:769-771).
func TestFormatListNonFirstGroupWrapSeam7(t *testing.T) {
	src := "[def a 1 def bbbbbbbbbb word1 word2 word3 word4 word5 word6 word7 word8 word9 word10]\n"
	want := "[def a 1\n" +
		"    def bbbbbbbbbb word1 word2 word3 word4 word5 word6 word7 word8\n" +
		"      word9 word10\n" +
		"  ]\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
}

// TestFormatInlineCommentSeam7 covers emitNode's comment arm reached
// naturally: a trailing comment renders inline through renderInline. A `##`
// comment runs to end of line exactly like `#` (BORU has no bounded block
// comment), so the whole tail is one comment and is emitted verbatim.
func TestFormatInlineCommentSeam7(t *testing.T) {
	for _, src := range []string{"foo bar # note\n", "foo bar ## note\n"} {
		if got := Format(src); got != src {
			t.Errorf("Format(%q) = %q, want unchanged", src, got)
		}
		idempotentSeam7(t, src)
	}
}

// TestEmitNodeRootAndCommentsSeam7 covers emitNode's NdRoot arm and the
// NdComment arm via direct calls, mirroring the existing emitNode
// direct-call test's idiom for the leaf kinds.
func TestEmitNodeRootAndCommentsSeam7(t *testing.T) {
	root := &Node{Kind: NdRoot, Children: []*Node{
		{Kind: NdWord, Text: "hi"},
		{Kind: NdNewline},
		{Kind: NdWord, Text: "bye"},
	}}
	r := newRenderer(DefaultRules())
	if got := r.emitNode(root, 0); got != "hi\nbye\n" {
		t.Errorf("emitNode(NdRoot) = %q, want %q", got, "hi\nbye\n")
	}
	if got := r.emitNode(&Node{Kind: NdComment, Text: "# c"}, 0); got != "# c" {
		t.Errorf("emitNode(NdComment) = %q, want %q", got, "# c")
	}
}

// TestEmitStatementEmptySeam7 covers emitStatement's empty-nodes guard
// (format.go:456-458). emitRoot never calls it with an empty statement
// (blank lines are handled earlier), so the guard is exercised directly.
func TestEmitStatementEmptySeam7(t *testing.T) {
	r := newRenderer(DefaultRules())
	if got := r.emitStatement(nil, 0); got != "" {
		t.Errorf("emitStatement(nil) = %q, want empty", got)
	}
	if got := r.emitStatement([]*Node{}, 4); got != "" {
		t.Errorf("emitStatement([]) = %q, want empty", got)
	}
}

// TestSplitIntoGroupsEmptySeam7 covers splitIntoGroups' empty fallback
// (format.go:795-797): with no children the loop appends nothing, so the
// function returns a single group equal to the (empty) input slice.
func TestSplitIntoGroupsEmptySeam7(t *testing.T) {
	r := newRenderer(DefaultRules())
	g := r.splitIntoGroups(nil)
	if len(g) != 1 {
		t.Fatalf("splitIntoGroups(nil) len = %d, want 1", len(g))
	}
	if len(g[0]) != 0 {
		t.Errorf("splitIntoGroups(nil)[0] len = %d, want 0", len(g[0]))
	}
	// Boundary: a run with no statement-start word is one group holding
	// all children (the other empty-groups return path).
	one := r.splitIntoGroups([]*Node{{Kind: NdWord, Text: "x"}, {Kind: NdWord, Text: "y"}})
	if len(one) != 1 || len(one[0]) != 2 {
		t.Errorf("splitIntoGroups(x y) = %d groups (want 1 of len 2)", len(one))
	}
}
