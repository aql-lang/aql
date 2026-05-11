package test

import (
	"testing"
)

// --- Naming rule: type names start with a capital, def names don't ---
//
// Enforced as a syntax rule at the boundary between user-supplied
// names and the registry. A misnamed binding errors before it
// installs, so the namespace stays predictable: Words starting with
// a capital are types, Words starting with anything else are values
// (or function defs).

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

// def rejects names starting with a capital.

func TestNameCase_DefUpperRejected(t *testing.T) {
	expectError(t, `def Foo 1`, "must not start with a capital letter")
}

func TestNameCase_DefFnUpperRejected(t *testing.T) {
	expectError(t, `def Doubler fn [[Integer] [Integer] [1 add]]`, "must not start with a capital letter")
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
