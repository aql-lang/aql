package lang_test

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

// `typeof` of a CONCRETE argument must return the precise type literal in check
// mode (a concrete-gated ReturnsFn), so `def T (typeof v)` gets a valid type
// body — `typeof (const 1)` is the singleton type, not the bare `Type` carrier
// the def-type validator rejected. class.tsv regressed these.
func TestTypeofConcreteSingletonCheck(t *testing.T) {
	clean := []string{
		`def One (typeof (const 1)) end 1 is One`,
		`def One (typeof (const 1)) end 2 is One`,
		`def One (typeof (const 1)) end 1.0 is One`,
		`def T (typeof 5) end 7 is T`, // typeof a plain literal is a valid body too
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("typeof-of-concrete type body should check clean: got %d errors for %q", n, src)
		}
	}
}

// An abstract carrier already TAGGED as a predicate-refine (subset) type
// satisfies that type's param nominally — the tag is the contract guarantee
// (a `Big`-returning fn), and the value-level predicate cannot be re-verified
// on an abstract carrier. `def mk fn [[] [Big] [50]] use (mk)` failed because
// the Big-returning result was predicate-checked as if abstract. A CONCRETE
// value is still predicate-checked.
func TestPredicateRefineReturnCheck(t *testing.T) {
	clean := `def Big (Integer gt 10) def mk fn [[] [Big] [50]] def use fn [[n:Big] [Integer] [n]] use (mk)`
	if n := checkErrs(t, clean); n != 0 {
		t.Errorf("predicate-refine-returning fn into a same-type param: expected 0 errors, got %d", n)
	}
	// NEGATIVE: a concrete value that fails the predicate is still rejected.
	bad := `def Big (Integer gt 10) def use fn [[n:Big] [Integer] [n]] use 5`
	if n := checkErrs(t, bad); n == 0 {
		t.Errorf("a concrete 5 (not > 10) for a Big param must still be flagged")
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

// A module's exports are statically DECLARED, so the checker resolves them in
// check mode — for `import` (qualified) it already did, and now `unpack` binds
// them UNQUALIFIED too. `unpack 'aql:mod'` / `unpack Export 'aql:mod'` gained
// RunInCheckMode + a kept-concrete module-name string, so a later bare word
// (`sqrt`) resolves instead of flagging undefined_word. NOT a runtime-only
// effect — the same declared exports `import` reads. (module-struct / unpack.tsv)
func TestUnpackModuleCheck(t *testing.T) {
	clean := []string{
		`unpack 'aql:math-util' sqrt 16.0`,
		`unpack MathUtil 'aql:math-util' sqrt 16.0`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("unpack module then bare-word use: expected 0 errors, got %d for %q", n, src)
		}
	}
}

// A mini-language kind registered with a STATICALLY-PROVIDED fn
// (`MiniLang.register poly (fn …)`) is now installed in check mode (an
// idempotent ReturnsFn mirroring parselang-register), so a later
// `mini poly …` resolves the kind instead of flagging "no mini-language is
// registered". The registration is pure (the fn is literal) — not runtime-only.
func TestMiniLangRegisterCheck(t *testing.T) {
	clean := `import "aql:minilang"  MiniLang.register poly (fn [[src:String opts:Map] [Integer] [((opts.x pow 2) add (3 mul opts.y))]]) end  mini poly 'x^2 + 3*y' {x:10, y:2}`
	if n := checkErrs(t, clean); n != 0 {
		t.Errorf("minilang register then use: expected 0 errors, got %d", n)
	}
}
