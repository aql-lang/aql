package test

import (
	"testing"
)

// --- Naming rule: capitalisation selects type vs value binding ---
//
// `def` is the universal binder (lang/doc/design/TYPE-UNIFORM.0.md
// Phase 2). The *name's capitalisation* selects what is bound:
// a capitalised name is a TYPE binding (`def` delegates to the same
// kernel installer the `type` word uses); a lowercase name is a
// VALUE binding. `type` itself still rejects lowercase names.

// type accepts capitalised names.

func TestNameCase_TypeUpperOK(t *testing.T) {
	got := runOne(t, `type Mid Integer
def n:Mid 5
n`)
	if len(got) != 1 || got[0] != int64(5) {
		t.Errorf("got %v, want [5]", got)
	}
}

// type rejects names that don't start with a capital.

func TestNameCase_TypeLowerRejected(t *testing.T) {
	expectError(t, `type foo Integer`, "must start with a capital letter")
}

func TestNameCase_TypeUnderscoreRejected(t *testing.T) {
	expectError(t, `type _foo Integer`, "must start with a capital letter")
}

func TestNameCase_TypeDigitRejected(t *testing.T) {
	expectError(t, `type 1foo Integer`, "must start with a capital letter")
}

// def accepts non-capitalised names.

func TestNameCase_DefLowerOK(t *testing.T) {
	got := runOne(t, `def x 1
x`)
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}

func TestNameCase_DefHyphenOK(t *testing.T) {
	got := runOne(t, `def my-thing 42
my-thing`)
	if len(got) != 1 || got[0] != int64(42) {
		t.Errorf("got %v, want [42]", got)
	}
}

// def with a capitalised name is a TYPE binding — equivalent to
// `type`. `def Mid Integer` installs Mid as a type, usable in a
// type position exactly like `type Mid Integer` would.

func TestNameCase_DefUpperIsTypeBinding(t *testing.T) {
	got := runOne(t, `def Mid Integer
def n:Mid 5
n`)
	if len(got) != 1 || got[0] != int64(5) {
		t.Errorf("got %v, want [5]", got)
	}
}

// A capitalised def binding an object type absorbs the lattice-minting
// that `type` does: typeof reports the bound name.
func TestNameCase_DefUpperObjectMints(t *testing.T) {
	got := runOne(t, `def Acct (maketype Object {bal:Number})
make Acct {bal:1} typeof`)
	if len(got) != 1 || got[0] != "Acct" {
		t.Errorf("got %v, want [Acct]", got)
	}
}

// Typed-def shares the same rule.

func TestNameCase_TypedDefUpperRejected(t *testing.T) {
	expectError(t, `def X:Integer 1`, "must not start with a capital letter")
}

func TestNameCase_TypedDefLowerOK(t *testing.T) {
	got := runOne(t, `def x:Integer 1
x`)
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}
