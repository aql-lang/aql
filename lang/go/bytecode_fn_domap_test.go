package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Pins for the fn-internal `do {…}` computed-map body (decision.aql's
// evaluator idiom — `{ok:false error:…}` result maps): the dyn-do body
// residual is runtime-counted, which used to mark the whole fn
// VARIADIC-returning, so any fixed-arity consumer of its call refused
// "consumes loop results". A DECLARED return tuple now overrides the
// marking — the VM RET enforces the declared count at runtime exactly
// where the interpreter raises — so the call seats the declared shape.
func TestFnDoMapBodyCompiles(t *testing.T) {
	// The call result feeds a FIXED-ARITY consumer (`get`) — the shape
	// that used to refuse (a program-residual consumer absorbs variadics
	// and never tripped it).
	mustCompileWithParity(t, `
def make-point fn [[x:Integer y:Integer] [Map] [
  do {x:[x] y:[y] dist2:[(x x mul) (y y mul) add]}
]]
((make-point 3 4) get "dist2")`, "[25]")

	mustCompileWithParity(t, `
def no-match fn [[reason:String] [Map] [
  do {ok:[false] error:[reason]}
]]
((no-match "no-rule") get "error")`, "[no-rule]")
}

// TestFnDoMapCountMismatchParity pins the negative that makes the
// override sound: a declared-[Map] fn whose dyn-do body nets TWO values
// at runtime raises the identical return-count type_error on the VM and
// the interpreter — the declared contract is enforced, not assumed.
func TestFnDoMapCountMismatchParity(t *testing.T) {
	const src = `def bad fn [[b:Any] [Map] [ do b ]] (bad [1 2])`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("expected a native compile, refused: reason=%q err=%v", reason, cerr)
	}
	b, _ := New()
	_, compiled, errC := b.RunCompiled(src)
	if !compiled {
		t.Fatal("expected a compiled run")
	}
	c, _ := New()
	_, errI := c.Run(src)
	if errC == nil || errI == nil {
		t.Fatalf("both runs must raise: compiled=%v interp=%v", errC, errI)
	}
	const want = "expected 1 return value(s), got 2"
	if !strings.Contains(fmt.Sprint(errC), want) || !strings.Contains(fmt.Sprint(errI), want) {
		t.Errorf("return-count error parity: compiled=%v interp=%v, both must say %q", errC, errI, want)
	}
}
