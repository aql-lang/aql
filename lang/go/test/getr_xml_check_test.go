package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The Xml well-known field set is fixed (tag / attr / cren), so a
// strict read of any other literal field over a concrete Xml receiver
// is a guaranteed runtime not_found. getrXmlReturns mirrors it at
// check time with the handler's byte-identical text (the Xml twin of
// getrNodeReturns' strict-miss contract; flagged in PR #302 review).
func TestCheckGetrXmlMissMirror(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def m <a>text</a> end m!.nope`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "not_found" && strings.Contains(d.Detail, `Xml has no field "nope"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("check did not mirror the guaranteed Xml strict miss; diagnostics: %+v", res.Diagnostics)
	}
}

// Negative twins: the three well-known fields check clean (and narrow
// to their fixed shapes — m!.tag feeds a String slot), and a computed
// key proves nothing, so neither may be flagged.
func TestCheckGetrXmlKnownFieldsClean(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, src := range []string{
		`def m <a>text</a> end m!.tag`,
		`def m <a>text</a> end m!.attr`,
		`def m <a>text</a> end m!.cren`,
		`def m <a>text</a> end (m!.tag) add "!"`,
		`def f fn [[k:String][Any][<a>t</a> getr k]] f 'tag'`,
	} {
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("check %q: %v", src, err)
		}
		for _, d := range res.Diagnostics {
			if d.Severity == "error" {
				t.Errorf("check %q wrongly flagged: %s: %s", src, d.Code, d.Detail)
			}
		}
	}
}
