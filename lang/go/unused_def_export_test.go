package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// A reference-exported word is a USE of its def. The canonical public-API
// binding form is `export "X" { name: impl/v }` (a /v reference), and a direct
// `{ name: impl }` of a fn value is equivalent. The use-tracker must count
// those as uses, or every public word is falsely flagged unused_def precisely
// because it is public — the dominant `unused_def` false positive on the
// voxgig-boru trie/decision/bloom client modules (their BORU-CHECK-REPORT
// wishlist item #1). The negative half pins that a GENUINELY unreferenced def
// is still flagged, so the fix removes false positives without introducing
// false negatives.
func TestUnusedDefReferenceExport(t *testing.T) {
	unusedDefs := func(t *testing.T, src string) []string {
		t.Helper()
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		cr, _ := a.Check(src)
		var names []string
		for _, d := range cr.Diagnostics {
			if d.Code == "unused_def" {
				names = append(names, d.Word)
			}
		}
		return names
	}

	// Positive: each export form counts as a use — no unused_def.
	clean := []struct{ name, src string }{
		{"/v reference at top level", `def mk fn [[] [Map] [{e: false}]] end mk/v`},
		{"/v reference-export", `def add1 fn [[n:Integer] [Integer] [n add 1]] end export "Demo" { inc: add1/v }`},
		{"multiple reference-exports", `def add1 fn [[n:Integer] [Integer] [n add 1]] end def dbl fn [[n:Integer] [Integer] [n mul 2]] end export "Demo" { inc: add1/v, dbl: dbl/v }`},
		{"type export", `def Color Integer end export "Demo" { Color: Color }`},
		{"ref word", `def mk fn [[] [Map] [{}]] end def r (valof mk) end r`},
	}
	for _, c := range clean {
		if got := unusedDefs(t, c.src); len(got) != 0 {
			t.Errorf("%s: expected no unused_def, got %v\n  src: %s", c.name, got, c.src)
		}
	}

	// Negative: a def referenced nowhere is still flagged (no false negative).
	got := unusedDefs(t, `def used fn [[n:Integer] [Integer] [n]] end def dead fn [[] [Integer] [1]] end export "Demo" { f: used/v }`)
	if len(got) != 1 || got[0] != "dead" {
		t.Errorf("expected only 'dead' flagged unused, got %v", got)
	}
	// And the reference-exported def is not among the flagged names.
	for _, name := range got {
		if name == "used" {
			t.Errorf("reference-exported 'used' must not be flagged unused")
		}
	}
}
