package native

import "testing"

// Codex round-10 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md): the plain
// immutable push / unshift grows and the list slice overloads gained check-mode
// residuals (they had none, unlike their FlexList / reverse-take counterparts),
// so a bad top-level grow and a chained write after grow/slice are diagnosed.

func TestPlainGrowCheckMirror(t *testing.T) {
	// A provably non-conforming top-level push/unshift is flagged at check.
	for _, src := range []string{
		`def xs:[:Integer] [1] (push "bad" xs)`,
		`def xs:[:Integer] [1] (unshift "bad" xs)`,
	} {
		r := tcCheck(t, src)
		if !tcHasTypeError(r, "does not conform to element type Integer") {
			t.Errorf("[%s] bad grow should be flagged in check: %+v", src, r.Check.Diagnostics)
		}
	}
	// A chained write after a grow retains the tag and is flagged.
	r := tcCheck(t, `def xs:[:Integer] [1] ((push 2 xs) set 0 "bad")`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("chained write after push should be flagged: %+v", r.Check.Diagnostics)
	}
	// Negatives: conforming grow silent; untyped grow silent.
	if r := tcCheck(t, `def xs:[:Integer] [1] (push 2 xs)`); tcHasTypeError(r, "does not conform") {
		t.Errorf("conforming push should be silent: %+v", r.Check.Diagnostics)
	}
	if r := tcCheck(t, `def xs [1] (push "ok" xs)`); tcHasTypeError(r, "does not conform") {
		t.Errorf("untyped push should be silent: %+v", r.Check.Diagnostics)
	}
}

func TestSliceCheckMirror(t *testing.T) {
	for _, src := range []string{
		`def xs:[:Integer] [1 2] ((slice xs) set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] ((slice 1 xs) set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] ((slice 0 2 xs) set 0 "bad")`,
	} {
		r := tcCheck(t, src)
		if !tcHasTypeError(r, "does not conform to element type Integer") {
			t.Errorf("[%s] chained write after slice should be flagged: %+v", src, r.Check.Diagnostics)
		}
	}
	// A string slice carries no element tag (its residual must not become a
	// spurious typed list) — the check stays silent.
	if r := tcCheck(t, `(slice 1 3 "hello")`); len(r.Check.Diagnostics) != 0 {
		t.Errorf("string slice should produce no diagnostics: %+v", r.Check.Diagnostics)
	}
}
