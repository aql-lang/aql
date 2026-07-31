package formatter

import "testing"

func txt(s string) Doc  { return DocText{S: s} }
func ln() Doc           { return DocLine{} }
func hard() Doc         { return DocLine{Hard: true} }
func cat(ds ...Doc) Doc { return DocConcat{Parts: ds} }
func grp(d Doc) Doc     { return DocGroup{Body: d} }
func ind(n int, d Doc) Doc {
	return DocIndent{By: n, Body: d}
}

func TestRenderDoc(t *testing.T) {
	tests := []struct {
		name  string
		width int
		doc   Doc
		want  string
	}{
		{"group fits flat", 10, grp(cat(txt("a"), ln(), txt("b"))), "a b"},
		{"group breaks", 2, grp(cat(txt("a"), ln(), txt("b"))), "a\nb"},
		{
			"indent on break", 2,
			grp(cat(txt("["), ind(2, cat(ln(), txt("x"))), ln(), txt("]"))),
			"[\n  x\n]",
		},
		{"hard line always breaks", 100, grp(cat(txt("a"), hard(), txt("b"))), "a\nb"},
		{"bare text", 5, txt("hello"), "hello"},
		{
			// Outer group breaks, inner group still fits flat.
			"nested group inner flat", 8,
			grp(cat(txt("outer("), ind(2, cat(ln(), grp(cat(txt("x"), ln(), txt("y"))))), ln(), txt(")"))),
			"outer(\n  x y\n)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderDoc(tt.width, tt.doc); got != tt.want {
				t.Errorf("RenderDoc(%d)\n  got:  %q\n  want: %q", tt.width, got, tt.want)
			}
		})
	}
}

// TestFitsFlat pins fitsFlat directly, including the hard-line early-out
// and the container-descent branches (concat / indent / group).
func TestFitsFlat(t *testing.T) {
	if !fitsFlat(5, cat(txt("ab"), ln(), txt("c"))) {
		t.Error("ab c should fit in 5")
	}
	if fitsFlat(2, cat(txt("ab"), ln(), txt("cd"))) {
		t.Error("ab cd should not fit in 2")
	}
	if fitsFlat(100, cat(txt("a"), hard(), txt("b"))) {
		t.Error("a hardline b never fits flat")
	}
	// Descend through indent and a nested group.
	if !fitsFlat(9, ind(2, grp(cat(txt("a"), ln(), txt("bb"))))) {
		t.Error("nested group under indent should fit in 9")
	}
}

// TestRenderDocHostileIndent pins the pad-clamp guard (PR #298 review P1): a
// caller-supplied document node with a NEGATIVE or ABSURD `by` (reachable from
// boru via `Fmt.render {fmt:'indent' by:-1 …}`) must not panic strings.Repeat
// or force an unbounded allocation — the indent clamps to [0, maxPad].
func TestRenderDocHostileIndent(t *testing.T) {
	// Negative indent over a hard line: clamps to 0, so the break carries no
	// leading spaces (column 0) — no panic.
	if got := RenderDoc(80, ind(-5, cat(txt("a"), hard(), txt("b")))); got != "a\nb" {
		t.Errorf("negative indent = %q, want %q", got, "a\nb")
	}
	// An absurd indent is clamped to maxPad, not an unbounded allocation.
	out := RenderDoc(80, ind(maxPad*4, cat(txt("a"), hard(), txt("b"))))
	if want := len("a\n") + maxPad + len("b"); len(out) != want {
		t.Errorf("absurd indent produced %d bytes, want %d (clamped to maxPad)", len(out), want)
	}
}
