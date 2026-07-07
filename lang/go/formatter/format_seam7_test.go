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

// TestFormatFnSingleLineTrailingSeam7 covers tryFnFormat's single-line
// return (format.go:529). The reconstructed `def name fn [[args] [ret]
// [body]]` fits within the width even though the whole node run (with
// trailing tokens after the fn wrapper) overflowed it. tryFnFormat only
// reformats up to the wrapper, so trailing tokens are dropped — this is
// the observed behaviour for such (malformed) input, asserted verbatim.
func TestFormatFnSingleLineTrailingSeam7(t *testing.T) {
	src := "def n fn [[a:Integer] [Integer] [a]] one two three four five six seven eight\n"
	want := "def n fn [[a:Integer] [Integer] [a]]\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	// The trimmed form is a fixed point.
	idempotentSeam7(t, want)
}

// TestFormatFnBodyGroupWrapSeam7 covers tryFnFormat's multi-line body
// path where a single body group itself exceeds the width and falls to
// wrapStatement (format.go:552-554).
func TestFormatFnBodyGroupWrapSeam7(t *testing.T) {
	src := "def f fn [[x:Integer] [Integer] [x add x add x add x add x add x add x add x add x add x add x add x add x]]\n"
	want := "def f fn [[x:Integer] [Integer] [\n" +
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
	want := "alpha, bravo, charlie, delta, echo, foxtrot, golf, hotel, india,\n" +
		"  juliet, kilo, lima, mike\n"
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

// TestFormatInlineBlockCommentSeam7 covers emitNode's comment arm
// (format.go:606-607) reached naturally: a block comment sitting inside a
// list renders inline through renderInline.
func TestFormatInlineBlockCommentSeam7(t *testing.T) {
	src := "[a ## c ## b]\n"
	want := "[a ## c ## b]\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	idempotentSeam7(t, src)
}

// TestEmitNodeRootAndCommentsSeam7 covers emitNode's NdRoot arm
// (format.go:596-597) and the NdComment/NdBlockComment arm
// (format.go:606-607) via direct calls, mirroring the existing
// emitNode direct-call test's idiom for the leaf kinds.
func TestEmitNodeRootAndCommentsSeam7(t *testing.T) {
	root := &Node{Kind: NdRoot, Children: []*Node{
		{Kind: NdWord, Text: "hi"},
		{Kind: NdNewline},
		{Kind: NdWord, Text: "bye"},
	}}
	if got := emitNode(root, 0); got != "hi\nbye\n" {
		t.Errorf("emitNode(NdRoot) = %q, want %q", got, "hi\nbye\n")
	}
	if got := emitNode(&Node{Kind: NdComment, Text: "# c"}, 0); got != "# c" {
		t.Errorf("emitNode(NdComment) = %q, want %q", got, "# c")
	}
	if got := emitNode(&Node{Kind: NdBlockComment, Text: "## b ##"}, 0); got != "## b ##" {
		t.Errorf("emitNode(NdBlockComment) = %q, want %q", got, "## b ##")
	}
}

// TestEmitStatementEmptySeam7 covers emitStatement's empty-nodes guard
// (format.go:456-458). emitRoot never calls it with an empty statement
// (blank lines are handled earlier), so the guard is exercised directly.
func TestEmitStatementEmptySeam7(t *testing.T) {
	if got := emitStatement(nil, 0); got != "" {
		t.Errorf("emitStatement(nil) = %q, want empty", got)
	}
	if got := emitStatement([]*Node{}, 4); got != "" {
		t.Errorf("emitStatement([]) = %q, want empty", got)
	}
}

// TestSplitIntoGroupsEmptySeam7 covers splitIntoGroups' empty fallback
// (format.go:795-797): with no children the loop appends nothing, so the
// function returns a single group equal to the (empty) input slice.
func TestSplitIntoGroupsEmptySeam7(t *testing.T) {
	g := splitIntoGroups(nil)
	if len(g) != 1 {
		t.Fatalf("splitIntoGroups(nil) len = %d, want 1", len(g))
	}
	if len(g[0]) != 0 {
		t.Errorf("splitIntoGroups(nil)[0] len = %d, want 0", len(g[0]))
	}
	// Boundary: a run with no statement-start word is one group holding
	// all children (the other empty-groups return path).
	one := splitIntoGroups([]*Node{{Kind: NdWord, Text: "x"}, {Kind: NdWord, Text: "y"}})
	if len(one) != 1 || len(one[0]) != 2 {
		t.Errorf("splitIntoGroups(x y) = %d groups (want 1 of len 2)", len(one))
	}
}
