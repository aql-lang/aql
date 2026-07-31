package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Regression tests for the DX-driven checker-accuracy fixes that brought the
// voxgig-boru library corpus to zero check warnings/info: the mixed_form_call
// Any-slot gate, and the unused_def fixes (ever-used semantics, the
// construction-self-use suppression, the opaque code-body use-scan, and the
// Reach-receiver walk). Each keeps its genuine true-positive while clearing the
// false positive the libraries hit.

func checkCounts(t *testing.T, src string) (errs, warns, mixedForm, unused int) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBORU(a)
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			errs++
		}
		if d.Severity == "warning" {
			warns++
		}
		switch d.Code {
		case "mixed_form_call":
			mixedForm++
		case "unused_def":
			unused++
		}
	}
	return
}

// mixed_form_call fires only when the deepest stack-bound slot is Any-typed:
// the genuine `(cond) if [a] [b]` footgun (if3's {Any,Any,Any}) still warns,
// while the documented receiver-first idiom over a concretely-typed collection
// slot (`xs set 0 v`) is silent.
func TestMixedFormCallAnySlotGate(t *testing.T) {
	// Footgun: stacked cond lands in if3's Any slot — must fire.
	if _, _, mf, _ := checkCounts(t, "def x 5\n(x gt 3) if [10] [20]"); mf == 0 {
		t.Errorf("(cond) if [a] [b] footgun must still emit mixed_form_call")
	}
	// Receiver-first set: receiver binds the concretely-typed (List) last slot
	// — must stay silent (the ~190 false advisories the libraries hit).
	if _, _, mf, _ := checkCounts(t, "def xs [1 2 3]\nxs set 0 9"); mf != 0 {
		t.Errorf("receiver-first `xs set 0 v` must not emit mixed_form_call, got %d", mf)
	}
}

// unused_def: a genuinely dead binding is still flagged, but the read-then-
// rebind, opaque-body, and reach-receiver false positives are gone.
func TestUnusedDefAccuracy(t *testing.T) {
	// Genuine dead value def — still flagged.
	if _, _, _, u := checkCounts(t, "def deadv 5\n10"); u == 0 {
		t.Errorf("a genuinely unused def must still be flagged")
	}
	// Read-then-rebind (loop-carried / conflation shape): the read counts for
	// the name despite the later rebind — no false unused_def.
	if _, _, _, u := checkCounts(t, "def acc 1\ndef _r (acc)\ndef acc 2\nacc"); u != 0 {
		t.Errorf("a read-then-rebind name must not be flagged unused, got %d", u)
	}
	// Use only inside an opaque code body (here an if branch): the use-scan
	// records it.
	if _, _, _, u := checkCounts(t, "def yy 5\n(true) if [yy] [0]"); u != 0 {
		t.Errorf("a def used only inside a code body must not be flagged unused, got %d", u)
	}
	// Use only as a reach receiver (`er.code`): WalkBodyWords descends the
	// Reach receiver so the use is recorded.
	if _, _, _, u := checkCounts(t, "def er {code:1}\n(true) if [er.code] [0]"); u != 0 {
		t.Errorf("a def used only as a reach receiver must not be flagged unused, got %d", u)
	}
}
