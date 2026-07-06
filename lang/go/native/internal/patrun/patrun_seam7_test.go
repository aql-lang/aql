package patrun

import "testing"

// Seam-7 coverage for the remaining patrun.go arms
// (design/TEST-SEAMS.10.md).

func TestPatrunSeam7RootCatchAll(t *testing.T) {
	p := New()
	p.Add(map[string]string{}, covStr("root"))
	// Also register a keyed rule so the trie descends past the root before
	// falling back to the root catch-all.
	p.Add(map[string]string{"a": "1"}, covStr("a1"))

	// A pattern whose key value does not match any registered branch
	// descends, finds nothing specific, and falls back to the root data.
	d, ok := p.Find(map[string]string{"a": "999"})
	if !ok || d == nil || *d != "root" {
		t.Fatalf("root catch-all: got %v / %v", d, ok)
	}

	// Positive pair: the exact keyed rule still wins.
	d, ok = p.Find(map[string]string{"a": "1"})
	if !ok || d == nil || *d != "a1" {
		t.Fatalf("keyed match: got %v / %v", d, ok)
	}
}

func TestPatrunSeam7ListEmpty(t *testing.T) {
	// An empty trie descends into a nil-valued root keymap and returns no
	// entries (the km.v == nil guard).
	p := New()
	if got := p.List(map[string]string{}); len(got) != 0 {
		t.Fatalf("empty List must be empty, got %v", got)
	}

	// Positive pair: a populated trie lists its entries.
	p.Add(map[string]string{"a": "1"}, covStr("a1"))
	if got := p.List(map[string]string{}); len(got) == 0 {
		t.Fatal("populated List must return entries")
	}
}
