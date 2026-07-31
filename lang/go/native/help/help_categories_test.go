package help

import "testing"

// TestCategoryCoverage pins the category table to the help registry: every
// registered word belongs to exactly one category, and every word listed in a
// category is a real registered Entry. This is the negative-test discipline
// from lang/go/CLAUDE.md applied to the taxonomy — without it, a new word
// could be added (or a category word misspelled) and the `describe` index
// would silently drop or duplicate it.
func TestCategoryCoverage(t *testing.T) {
	// Reverse index: word -> categories that list it. Also flags duplicates.
	seen := map[string][]string{}
	for _, c := range categories {
		// Core and module words are both members: a word that moved into
		// Category.Module is still categorised, and must still have an Entry.
		for _, w := range append(append([]string{}, c.Words...), c.ModuleWords()...) {
			seen[w] = append(seen[w], c.Name)
			if Lookup(w) == nil {
				t.Errorf("category %q lists %q, which is not a registered help Entry", c.Name, w)
			}
		}
	}

	for w, cats := range seen {
		if len(cats) > 1 {
			t.Errorf("word %q appears in multiple categories: %v", w, cats)
		}
	}

	// Every registered word must be categorised exactly once.
	for _, w := range Words() {
		if len(seen[w]) == 0 {
			t.Errorf("registered word %q is not in any category (add it to help_categories.go)", w)
		}
	}
}

// TestCategoryNamesUnique guards against two categories sharing a name, which
// would make `boru describe <name>` ambiguous.
func TestCategoryNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range categories {
		if seen[c.Name] {
			t.Errorf("duplicate category name %q", c.Name)
		}
		seen[c.Name] = true
		if c.Summary == "" {
			t.Errorf("category %q has no summary", c.Name)
		}
		// A category may legitimately have NO core words — `string` and
		// `binary` are entirely module-provided — but it must have some.
		if len(c.Words)+len(c.ModuleWords()) == 0 {
			t.Errorf("category %q has no words", c.Name)
		}
	}
}

// TestLookupCategory checks the accessor used by the describe command.
func TestLookupCategory(t *testing.T) {
	if c, ok := LookupCategory("math"); !ok || c.Name != "math" {
		t.Errorf("LookupCategory(math) = %+v, %v; want the math category", c, ok)
	}
	if _, ok := LookupCategory("not-a-category"); ok {
		t.Errorf("LookupCategory(not-a-category) returned ok=true; want false")
	}
	if got := CategoryOf("add"); got != "math" {
		t.Errorf("CategoryOf(add) = %q; want math", got)
	}
	if got := CategoryOf("not-a-word"); got != "" {
		t.Errorf("CategoryOf(not-a-word) = %q; want empty", got)
	}
	// A MODULE-provided word is categorised too. `upper` lives in
	// boru:string-util, so it is not in Category.Words — resolving it
	// requires the module arm. Omitting this case would report "" for all
	// 80 module words, which is what the pre-split table effectively did.
	if got := CategoryOf("upper"); got != "string" {
		t.Errorf("CategoryOf(upper) = %q; want string (a module word is still categorised)", got)
	}
	if got := CategoryOf("pi"); got != "math" {
		t.Errorf("CategoryOf(pi) = %q; want math", got)
	}
}
