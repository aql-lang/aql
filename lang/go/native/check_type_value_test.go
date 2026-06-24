package native_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// checkErrs counts error-severity diagnostics from `aql check` over src.
func checkErrs(t *testing.T, src string) int {
	t.Helper()
	a, _ := lang.New()
	cr, _ := a.Check(src)
	n := 0
	for _, d := range cr.Diagnostics {
		if d.Severity == "error" {
			n++
		}
	}
	return n
}

// A FLEX container's check-mode type must be its precise subtype (FlexMap /
// FlexList / FlexXml), not the bare Node supertype, so a downstream Flex* op
// matches its overload. `flex` / `node` carry a ReturnsFn modelling the input's
// node family (mirroring FlexDeepCopy). flex.tsv regressed all 40 of these.
func TestFlexCheckSubtype(t *testing.T) {
	clean := []string{
		`set b/q 2 (flex {a:1})`,                                 // FlexMap set
		`append 4 (flex [1 2])`,                                  // FlexList append
		`push 3 (flex [1 2])`,                                    // FlexList push
		`pop (flex [1 2])`,                                       // FlexList pop
		`sort (flex [3 1 2])`,                                    // FlexList sort
		`each [ add 1 ] (flex [1 2 3])`,                          // FlexList each
		`def f (flex {a:1}) def n (node f) set a/q 9 f drop n.a`, // node → plain Map
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("flex op should check clean: got %d errors for %q", n, src)
		}
	}
	// NEGATIVE: a scalar is not flexable — the [TNode] arg slot rejects it
	// (this is a static signature error, not a runtime one).
	if n := checkErrs(t, `flex 1`); n == 0 {
		t.Errorf("flex of a scalar must still be flagged")
	}
}

// A user DISJUNCT / ENUM type must be usable as a `def` type body and a type-
// algebra operand in check mode. `tor` mirrors its handler's disjunct (not a
// branch-merge that mishandles the nil-Parent None literal), `enum` runs its
// pure handler so the result carries its members, and toCarrier preserves the
// DisjunctInfo. compare.tsv regressed 15 of these.
func TestDisjunctEnumTypeBodyCheck(t *testing.T) {
	clean := []string{
		`String tor None`, // union builds, no halt
		`def Maybe (String tor None) Maybe tcmp Maybe`,                           // disjunct def + tcmp
		`def Maybe (String tor None) def g fn [[m:Maybe] [Integer] [1]] ('x' g)`, // disjunct param
		`def Color enum [red green blue] Color tcmp Color`,                       // enum def + tcmp
		`def Color enum [red green blue] Color tcmp Enum`,                        // enum vs builtin Enum
		`def Cat class {x:Integer} def Maybe (String tor None) Cat tcmp Maybe`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("type-value op should check clean: got %d errors for %q", n, src)
		}
	}
}

// A dispatch modifier (usurp / stack-args / forward-args / force-arity, the
// `/u` `/s` `/f` `/N` desugarings) over a stored fn-ref read via dot-access
// (`m.a` where `m = {a:add/r}`) sees a dynamic(Any) carrier — getNodeReturns
// cannot narrow a dispatch-bearing field — and must yield a gradual Function
// carrier in check mode rather than an illegal_ref. path-modifier.tsv regressed
// 12 of these.
func TestStoredFnRefModifierCheck(t *testing.T) {
	clean := []string{
		`def m {a:add/r} end m.a/u 1 2`,
		`def m {s:sub/r} end m.s/f 10 3`,
		`def m {s:sub/r} end 10 3 m.s/s`,
		`def m {a:add/r} end m.a/2 1 2`,
		`def o {m:{a:add/r}} end o.m.a/u 1 2`,
		`def m {s:sub/r} end force-arity 2 (usurp (m.s)) 10 3`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("stored-fn-ref modifier should check clean: got %d errors for %q", n, src)
		}
	}
	// NEGATIVE: usurp of a CONCRETE non-fn value is still an illegal_ref — the
	// check-mode gradual fallback fires only for a dynamic-Any / Function
	// carrier, never a concrete-typed one.
	if n := checkErrs(t, `usurp 5`); n == 0 {
		t.Errorf("usurp of a concrete non-fn value must still be flagged")
	}
}
