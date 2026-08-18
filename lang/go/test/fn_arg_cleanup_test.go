package test

import (
	"testing"
)

// Tests for unnamed argument cleanup in fn-defined words.
// Unconsumed unnamed args should be discarded after body execution,
// leaving only the declared return values on the stack.

// --- Unnamed args, body ignores them ---

func TestFnArgCleanup_UnnamedOneArg_OneReturn(t *testing.T) {
	// def f fn [[Atom] [Integer] [1]]; f (quote a) → 1
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [Integer] [1]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_UnnamedTwoArgs_TwoReturns(t *testing.T) {
	// def g fn [[Atom Atom] [Integer Integer] [1 2]]; g a b → 1 2
	result, err := runSteps(t, []string{
		`def g fn [[Atom Atom] [Integer Integer] [1 2]]`,
		`g (quote a) (quote b)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1 2")
}

func TestFnArgCleanup_UnnamedOneArg_TwoReturns(t *testing.T) {
	// Body produces 2 values, 1 unnamed arg discarded.
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [Integer Integer] [10 20]]`,
		`f (quote x)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10 20")
}

func TestFnArgCleanup_UnnamedTwoArgs_OneReturn(t *testing.T) {
	// 2 unnamed args, body ignores them, produces 1 return.
	result, err := runSteps(t, []string{
		`def f fn [[Atom Integer] [Integer] [99]]`,
		`f (quote x) 5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "99")
}

func TestFnArgCleanup_UnnamedThreeArgs_OneReturn(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[Atom Atom Atom] [Integer] [42]]`,
		`f (quote a) (quote b) (quote c)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Unnamed args, body consumes them ---

func TestFnArgCleanup_UnnamedConsumed_Upper(t *testing.T) {
	// Body consumes the unnamed Atom via upper.
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [String] [upper]]`,
		`f (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO'")
}

func TestFnArgCleanup_UnnamedConsumed_Add(t *testing.T) {
	// Body consumes both unnamed Integer args via add.
	result, err := runSteps(t, []string{
		`def f fn [[Integer Integer] [Integer] [add]]`,
		`f 3 5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "8")
}

func TestFnArgCleanup_UnnamedPartiallyConsumed(t *testing.T) {
	// 2 unnamed args, body consumes 1 (via upper), other discarded.
	result, err := runSteps(t, []string{
		`def f fn [[String Atom] [String] [upper]]`,
		`f "ignore" (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO'")
}

// --- Named args (no cleanup needed, bound via def/undef) ---

func TestFnArgCleanup_NamedOneArg_OneReturn(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[x:Atom] [Integer] [1]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_NamedTwoArgs_TwoReturns(t *testing.T) {
	result, err := runSteps(t, []string{
		`def g fn [[x:Atom y:Atom] [Integer Integer] [1 2]]`,
		`g (quote a) (quote b)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1 2")
}

func TestFnArgCleanup_NamedUsed(t *testing.T) {
	// Named arg used in body.
	result, err := runSteps(t, []string{
		`def f fn [[x:Integer] [Integer] [x mul 2]]`,
		`f 5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestFnArgCleanup_NamedTwoUsed(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[x:Integer y:Integer] [Integer] [x add y]]`,
		`f 3 7`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestFnArgCleanup_NamedUnused(t *testing.T) {
	// Named arg not referenced in body — still cleaned up.
	result, err := runSteps(t, []string{
		`def f fn [[x:Atom] [Integer] [42]]`,
		`f (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Mixed named and unnamed ---

func TestFnArgCleanup_MixedNamedUnnamed(t *testing.T) {
	// Named x used in body, unnamed Atom ignored and discarded.
	result, err := runSteps(t, []string{
		`def f fn [[x:Integer Atom] [Integer] [x mul 2]]`,
		`f 5 (quote ignored)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestFnArgCleanup_MixedUnnamedNamed(t *testing.T) {
	// Unnamed Integer ignored, named x used.
	result, err := runSteps(t, []string{
		`def f fn [[Integer x:Atom] [String] [x upper]]`,
		`f 99 (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO'")
}

// --- No return type declared (no cleanup) ---

func TestFnArgCleanup_NoReturnType_UnnamedPreserved(t *testing.T) {
	// Without return types, unnamed args remain (no ReturnCheck inserted).
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [] []]`,
		`f (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "hello")
}

// --- Zero args ---

func TestFnArgCleanup_ZeroArgs_OneReturn(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[] [Integer] [42]]`,
		`f`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestFnArgCleanup_ZeroArgs_TwoReturns(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[] [Integer Integer] [1 2]]`,
		`f`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1 2")
}

// --- Error: too few return values ---

func TestFnArgCleanup_TooFewReturns(t *testing.T) {
	// Body produces nothing, but 1 Integer return declared.
	// Even with 1 unnamed arg, its type (Atom) doesn't match Integer.
	_, err := runSteps(t, []string{
		`def f fn [[Atom] [Integer] [drop]]`,
		`f (quote hello)`,
	})
	if err == nil {
		t.Fatal("expected error for too few return values, got nil")
	}
}

// --- Error: too many values beyond unnamed + returns ---

func TestFnArgCleanup_TooManyReturns(t *testing.T) {
	// Body produces 3 values (dup dup), but 1 unnamed + 1 return = 2 max.
	_, err := runSteps(t, []string{
		`def f fn [[Number] [Number] [dup dup]]`,
		`f 5`,
	})
	if err == nil {
		t.Fatal("expected return count error, got nil")
	}
}

// --- Abbreviation: non-list input/output treated as single-element list ---

func TestFnArgCleanup_AbbreviatedSig(t *testing.T) {
	// fn [Atom Integer [1]] is equivalent to fn [[Atom] [Integer] [1]]
	result, err := runSteps(t, []string{
		`def f fn [Atom Integer [1]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_AbbreviatedTwoArgs(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[Integer Integer] Integer [add]]`,
		`f 3 4`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "7")
}

// --- Print side effect with return ---

func TestFnArgCleanup_PrintThenReturn(t *testing.T) {
	// def f fn [[Atom] [Integer] [print; 1]]; f a → prints "a", returns 1
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [Integer] [print; 1]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// --- Multiple unnamed args, body uses some ---

func TestFnArgCleanup_ThreeArgs_BodyUsesOne(t *testing.T) {
	// 3 unnamed args, body only uses top one (MathUtil.negate), other 2 discarded.
	result, err := runSteps(t, []string{
		`def f fn [[Atom Atom Integer] [Integer] [MathUtil.negate]]`,
		`f (quote x) (quote y) 5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "-5")
}

func TestFnArgCleanup_ThreeArgs_ThreeReturns(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[Atom Atom Atom] [Integer Integer Integer] [10 20 30]]`,
		`f (quote a) (quote b) (quote c)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10 20 30")
}

// --- Concrete return values (non-type output sig appended to body) ---

// --- Literal output types: a value IS a type (ADR-010) ---
//
// A literal in the output sig declares a return the body must MATCH. It
// used to mean the opposite — an all-literal output sig was
// "return-by-value sugar" and the literals were appended to the body,
// so `fn [Atom 1 [print]]` returned 1 from a body that returned nothing.
// That reading made an output sig mean two different things depending on
// a classification that could not be made correct, and it silently
// degraded every such declaration's static return type to Any. The sig
// is now always types, and a literal type admits exactly itself.

func TestFnArgCleanup_LiteralReturnType_Simple(t *testing.T) {
	// Output `1` constrains the single return to the value 1; the body
	// has to produce it. `end` stops `print` collecting the 1 forward,
	// so it consumes the Atom argument instead — which is exactly the
	// token the deleted sugar used to splice in for you.
	result, err := runSteps(t, []string{
		`def f fn [Atom 1 [print end 1]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_LiteralReturnType_Violated(t *testing.T) {
	// The body returns 2 where the declaration says 1 — the whole point
	// of the change: this is now a type error instead of 1 being
	// appended to the body and 2 becoming a phantom extra return.
	_, err := runSteps(t, []string{
		`def f fn [Atom 1 [print end 2]]`,
		`f (quote a)`,
	})
	if err == nil {
		t.Fatal("expected a return-type error for a body that violates its literal return type")
	}
}

func TestFnArgCleanup_LiteralReturnType_MultipleValues(t *testing.T) {
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [1 2] [print end 1 2]]`,
		`f (quote a)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1 2")
}

func TestFnArgCleanup_LiteralReturnType_Factorial(t *testing.T) {
	// The base case declares AND returns 1; the recursive case uses a
	// named param and a type return.
	result, err := runSteps(t, []string{
		`def fact fn [[0] 1 [1] [n:Integer] [Integer] [n mul (n sub 1 fact)]]`,
		`fact 5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFnArgCleanup_LiteralReturnType_FactorialZero(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[0] 1 [1] [n:Integer] [Integer] [n mul (n sub 1 fact)]]`,
		`fact 0`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_LiteralReturnType_FactorialOne(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[0] 1 [1] [n:Integer] [Integer] [n mul (n sub 1 fact)]]`,
		`fact 1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFnArgCleanup_LiteralReturnType_FactorialTen(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[0] 1 [1] [n:Integer] [Integer] [n mul (n sub 1 fact)]]`,
		`fact 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

func TestFnArgCleanup_ConcreteReturn_TypeOutputUnchanged(t *testing.T) {
	// String IS a type, so normal return type checking applies.
	result, err := runSteps(t, []string{
		`def f fn [[Atom] [String] [upper]]`,
		`f (quote hello)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO'")
}

func TestFnArgCleanup_LiteralReturnType_EmptyBodyIsAnError(t *testing.T) {
	// The sugar's headline case: an EMPTY body against a literal output
	// used to "succeed", returning the spliced 42. A body that produces
	// nothing satisfies no declared return, so it is an error now — the
	// silent wrong answer this shape used to give is the reason the
	// sugar went.
	_, err := runSteps(t, []string{
		`def f fn [[Atom] 42 []]`,
		`f (quote x)`,
	})
	if err == nil {
		t.Fatal("expected an error for a body that produces no return value")
	}
}

func TestFnArgCleanup_LiteralReturnType_StringValue(t *testing.T) {
	// A String literal names no type, so it is a literal type — the same
	// rule the Integer literals above follow.
	result, err := runSteps(t, []string{
		`def f fn [[Integer] "done" [print end "done"]]`,
		`f 7`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'done'")
}
