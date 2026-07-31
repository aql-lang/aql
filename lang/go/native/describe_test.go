package native

import (
	"bytes"
	"strings"
	"testing"
)

// DescribeIndex is the body of the 0-arg `describe`: a categorised guide to
// words plus the module catalog and the drill-in forms.
func TestDescribeIndex(t *testing.T) {
	var b bytes.Buffer
	DescribeIndex(&b)
	out := b.String()
	for _, want := range []string{
		"Words:", "math", "compare", // category headers
		"Modules", "type-util", // module catalog
		"describe <category>", "describe \"boru:<module>:<word>\"", // drill-in
	} {
		if !strings.Contains(out, want) {
			t.Errorf("DescribeIndex missing %q in:\n%s", want, out)
		}
	}
}

// DescribeName resolves a bare category name to its group listing.
func TestDescribeNameCategory(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	DescribeName(reg, &b, "math")
	out := b.String()
	if !strings.Contains(out, "math —") || !strings.Contains(out, "add") {
		t.Errorf("expected math category with 'add', got:\n%s", out)
	}
	// A word from another category must not leak into the listing (anchor on
	// the line format so we don't match "concat" inside add's "concatenate").
	if strings.Contains(out, "\n  concat ") {
		t.Errorf("string word 'concat' should not be listed under math")
	}
}

// DescribeName resolves a registered word to its live docs.
func TestDescribeNameWord(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	DescribeName(reg, &b, "add")
	out := b.String()
	for _, want := range []string{"add —", "Signatures:", "Examples:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in word docs, got:\n%s", want, out)
		}
	}
}

// An unrecognised name that is neither word, category, nor loadable module
// reports that nothing is known by it.
func TestDescribeNameUnknown(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	DescribeName(reg, &b, "definitely_not_a_word_or_module")
	if !strings.Contains(b.String(), "no description available") {
		t.Errorf("expected 'no description available', got:\n%s", b.String())
	}
}

// splitModulePath separates the colon forms describe accepts.
func TestSplitModulePath(t *testing.T) {
	cases := []struct {
		in, modRef, word string
	}{
		{"boru:type-util", "boru:type-util", ""},
		{"boru:type-util:tpartial", "boru:type-util", "tpartial"},
		{"type-util:tpartial", "type-util", "tpartial"},
		{"./lib.boru:double", "./lib.boru", "double"},
		{"boru:type-util:", "boru:type-util", ""},
	}
	for _, c := range cases {
		modRef, word := splitModulePath(c.in)
		if modRef != c.modRef || word != c.word {
			t.Errorf("splitModulePath(%q) = (%q,%q); want (%q,%q)", c.in, modRef, word, c.modRef, c.word)
		}
	}
}
