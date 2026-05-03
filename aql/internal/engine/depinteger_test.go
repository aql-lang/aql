package engine

import (
	"testing"
)

// --- Dependent types: DepInteger ---
//
// `Integer gt 10`, `Integer gte 10`, `Integer lt 10`, `Integer lte 10`
// each construct a DepInteger value: a Type/Dependent/DepInteger
// carrying a bit-field comparison kind plus a scalar bound. The value
// behaves as a sub-type of Integer: it satisfies any signature slot
// expecting an Integer, and unifies with a concrete Integer iff the
// concrete value satisfies the comparison; on success unification
// returns the plain Integer (not the DepInteger).

// --- Construction ---

func TestNewDepIntegerGTE(t *testing.T) {
	d := NewDepInteger(DepGTE, 10)
	if !d.VType.Equal(TDepInteger) {
		t.Fatalf("VType = %s, want %s", d.VType, TDepInteger)
	}
	info, err := d.AsDepInteger()
	if err != nil {
		t.Fatalf("AsDepInteger: %v", err)
	}
	if info.Kind != DepGTE {
		t.Errorf("Kind = %d, want DepGTE", info.Kind)
	}
	if info.Bound != 10 {
		t.Errorf("Bound = %d, want 10", info.Bound)
	}
}

func TestDepIntegerKinds(t *testing.T) {
	cases := []struct {
		kind DepKind
		name string
	}{
		{DepGT, "GT"},
		{DepGTE, "GTE"},
		{DepLT, "LT"},
		{DepLTE, "LTE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDepInteger(tc.kind, 5)
			info, _ := d.AsDepInteger()
			if info.Kind != tc.kind {
				t.Errorf("Kind = %d, want %d", info.Kind, tc.kind)
			}
		})
	}
}

// --- Unification: success returns the integer scalar ---

func TestUnifyDepIntegerGTESuccess(t *testing.T) {
	dep := NewDepInteger(DepGTE, 10)
	for _, n := range []int64{10, 11, 100} {
		val := NewInteger(n)
		got, ok := Unify(val, dep)
		if !ok {
			t.Errorf("Unify(%d, Integer gte 10) failed; want success", n)
			continue
		}
		// Result should be the plain integer scalar, not a DepInteger.
		if got.VType.Equal(TDepInteger) {
			t.Errorf("Unify result for %d still has DepInteger type: %s", n, got.VType)
		}
		if !got.VType.Matches(TInteger) {
			t.Errorf("Unify result for %d type = %s, want Integer subtype", n, got.VType)
		}
		gn, _ := got.AsInteger()
		if gn != n {
			t.Errorf("Unify result for %d = %d, want %d", n, gn, n)
		}
	}
}

func TestUnifyDepIntegerGTEFailure(t *testing.T) {
	dep := NewDepInteger(DepGTE, 10)
	for _, n := range []int64{9, 0, -5} {
		val := NewInteger(n)
		_, ok := Unify(val, dep)
		if ok {
			t.Errorf("Unify(%d, Integer gte 10) succeeded; want failure", n)
		}
	}
}

func TestUnifyDepIntegerSymmetric(t *testing.T) {
	// Unification is order-independent.
	dep := NewDepInteger(DepGTE, 10)
	val := NewInteger(15)
	if _, ok := Unify(dep, val); !ok {
		t.Error("Unify(dep, 15) failed; want success (symmetric form)")
	}
	if _, ok := Unify(val, dep); !ok {
		t.Error("Unify(15, dep) failed; want success")
	}
}

func TestUnifyDepIntegerAllKinds(t *testing.T) {
	cases := []struct {
		kind   DepKind
		bound  int64
		val    int64
		expect bool
	}{
		// gt
		{DepGT, 10, 11, true},
		{DepGT, 10, 10, false},
		{DepGT, 10, 9, false},
		// gte
		{DepGTE, 10, 11, true},
		{DepGTE, 10, 10, true},
		{DepGTE, 10, 9, false},
		// lt
		{DepLT, 10, 9, true},
		{DepLT, 10, 10, false},
		{DepLT, 10, 11, false},
		// lte
		{DepLTE, 10, 9, true},
		{DepLTE, 10, 10, true},
		{DepLTE, 10, 11, false},
	}
	for _, tc := range cases {
		dep := NewDepInteger(tc.kind, tc.bound)
		val := NewInteger(tc.val)
		_, ok := Unify(val, dep)
		if ok != tc.expect {
			t.Errorf("Unify(%d, kind=%d bound=%d) = %v; want %v",
				tc.val, tc.kind, tc.bound, ok, tc.expect)
		}
	}
}

// Non-integer values must fail to unify with any DepInteger constraint.
func TestUnifyDepIntegerRejectsNonInteger(t *testing.T) {
	dep := NewDepInteger(DepGTE, 10)
	for _, v := range []Value{
		NewString("100"),
		NewBoolean(true),
		NewDecimal(11.5),
	} {
		if _, ok := Unify(v, dep); ok {
			t.Errorf("Unify(%s, Integer gte 10) succeeded; want failure for non-integer", v.VType)
		}
	}
}

// --- Lattice: DepInteger satisfies an Integer-typed sig position ---

func TestDepIntegerMatchesIntegerType(t *testing.T) {
	dep := NewDepInteger(DepGTE, 10)
	if !dep.VType.Matches(TInteger) {
		t.Errorf("DepInteger.VType (%s) does not match TInteger", dep.VType)
	}
	if !dep.VType.Matches(TNumber) {
		t.Errorf("DepInteger.VType (%s) does not match TNumber", dep.VType)
	}
}

func TestSigTypeMatchesAcceptsDepInteger(t *testing.T) {
	dep := NewDepInteger(DepGTE, 10)
	// A sig position expecting Integer should accept a DepInteger value.
	if !sigTypeMatches(dep, TInteger) {
		t.Errorf("sigTypeMatches(dep, TInteger) = false; want true")
	}
}

// Integer is NOT a subtype of DepInteger — only the other direction.
func TestIntegerDoesNotMatchDepInteger(t *testing.T) {
	if TInteger.Matches(TDepInteger) {
		t.Error("TInteger.Matches(TDepInteger) = true; want false (asymmetric)")
	}
}

// --- Construction via the comparison-word handlers (Integer gte 10) ---

func TestRunIntegerGTEReturnsDepInteger(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Build the program: `Integer gte 10` — a type literal followed by
	// gte and an integer. The new sig [Type, Number] -> [Dependent]
	// must fire, returning a DepInteger value.
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TInteger),
		NewWord("gte"),
		NewInteger(10),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	got := result[0]
	if !got.VType.Equal(TDepInteger) {
		t.Fatalf("VType = %s, want %s", got.VType, TDepInteger)
	}
	info, err := got.AsDepInteger()
	if err != nil {
		t.Fatalf("AsDepInteger: %v", err)
	}
	if info.Kind != DepGTE || info.Bound != 10 {
		t.Errorf("got Kind=%d Bound=%d, want DepGTE 10", info.Kind, info.Bound)
	}
}

func TestRunIntegerComparisonOpsReturnDepInteger(t *testing.T) {
	cases := []struct {
		op   string
		kind DepKind
	}{
		{"gt", DepGT},
		{"gte", DepGTE},
		{"lt", DepLT},
		{"lte", DepLTE},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			result := runAQL(t, r, []Value{
				NewTypeLiteral(TInteger),
				NewWord(tc.op),
				NewInteger(7),
			})
			if len(result) != 1 || !result[0].VType.Equal(TDepInteger) {
				t.Fatalf("Integer %s 7: got %v, want a DepInteger", tc.op, result)
			}
			info, _ := result[0].AsDepInteger()
			if info.Kind != tc.kind || info.Bound != 7 {
				t.Errorf("Integer %s 7: got Kind=%d Bound=%d, want %d/7", tc.op, info.Kind, info.Bound, tc.kind)
			}
		})
	}
}

// --- Original Boolean comparison sigs still fire when neither arg is a Type ---

func TestIntegerComparisonsStillReturnBoolean(t *testing.T) {
	cases := []struct {
		expr []Value
		want bool
	}{
		{[]Value{NewInteger(15), NewWord("gte"), NewInteger(10)}, true},
		{[]Value{NewInteger(5), NewWord("gte"), NewInteger(10)}, false},
		{[]Value{NewInteger(5), NewWord("lt"), NewInteger(10)}, true},
		{[]Value{NewInteger(10), NewWord("lt"), NewInteger(10)}, false},
	}
	for _, tc := range cases {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, tc.expr)
		if len(result) != 1 || !result[0].VType.Equal(TBoolean) {
			t.Fatalf("expected boolean, got %v", result)
		}
		got, _ := result[0].AsBoolean()
		if got != tc.want {
			t.Errorf("got %v, want %v for %v", got, tc.want, tc.expr)
		}
	}
}

// --- `is` overload for DepInteger as a pattern ---

func TestIsCheckWithDepIntegerPattern(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 15 is (Integer gte 10) → true
	result := runAQL(t, r, []Value{
		NewInteger(15),
		NewWord("is"),
		NewWord("("),
		NewTypeLiteral(TInteger),
		NewWord("gte"),
		NewInteger(10),
		NewWord(")"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	got, _ := result[0].AsBoolean()
	if !got {
		t.Errorf("15 is (Integer gte 10) = false; want true")
	}
}

func TestIsCheckWithDepIntegerPatternFail(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 5 is (Integer gte 10) → false
	result := runAQL(t, r, []Value{
		NewInteger(5),
		NewWord("is"),
		NewWord("("),
		NewTypeLiteral(TInteger),
		NewWord("gte"),
		NewInteger(10),
		NewWord(")"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	got, _ := result[0].AsBoolean()
	if got {
		t.Errorf("5 is (Integer gte 10) = true; want false")
	}
}
