package native

import (
	"strings"
	"testing"
)

// Codex round-9 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	P1 d2AdoptTyped rebuilt an already-flex child on the WRITE path (Unify's flex
//	   branch allocates a fresh store), detaching it — a later mutation through
//	   the child was invisible through the container. Now retags the flex child
//	   IN PLACE (core.RetagFlexElem) after validating.
//	P2 the list-copy check residuals (ReturnsPreserveListAt) now copy the source
//	   ElemConstraint, so a chained write after reverse/take/... is diagnosed.

// #1 — a flex child written into a typed flex container is stored BY REFERENCE:
// a later mutation through the child is visible through the container.
func TestFlexChildWriteKeepsIdentity(t *testing.T) {
	out, err := tcRun(t, `def d:{:{:Integer}} (flex {}) def c (flex {x:1}) (d set a/q c) (c set y/q 2) (d.a)`)
	if err != nil {
		t.Fatalf("flex child write: %v", err)
	}
	// The last value is d.a — it must reflect the mutation made through c.
	if s := out[len(out)-1].String(); !strings.Contains(s, "y:2") {
		t.Errorf("flex child identity lost: d.a = %s, want the y:2 mutation made through c", s)
	}
	// A non-conforming flex child is rejected against the inner {:Integer}.
	if _, e := tcRun(t, `def d:{:{:Integer}} (flex {}) def c (flex {x:"bad"}) (d set a/q c)`); e == nil ||
		!strings.Contains(e.Error(), "does not conform to element type") {
		t.Errorf("bad flex child write should reject, got %v", e)
	}
}

// #2 — the list-copy check residuals flag a chained write.
func TestListCopyResidualCheckMirror(t *testing.T) {
	for _, src := range []string{
		`def xs:[:Integer] [1 2 3] ((reverse xs) set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] ((take 2 xs) set 0 "bad")`,
		`def xs:[:Integer] [1 2 3] ((shed 1 xs) set 0 "bad")`,
	} {
		r := tcCheck(t, src)
		if !tcHasTypeError(r, "does not conform to element type Integer") {
			t.Errorf("[%s] chained write after a list copy should be flagged in check: %+v", src, r.Check.Diagnostics)
		}
	}
	// A conforming chained write stays silent; an untyped source is unenforced.
	if r := tcCheck(t, `def xs:[:Integer] [1 2 3] ((reverse xs) set 0 9)`); tcHasTypeError(r, "does not conform") {
		t.Errorf("conforming chained write should be silent: %+v", r.Check.Diagnostics)
	}
	if r := tcCheck(t, `def xs [1 2 3] ((reverse xs) set 0 "ok")`); tcHasTypeError(r, "does not conform") {
		t.Errorf("untyped chained write should be silent: %+v", r.Check.Diagnostics)
	}
}
