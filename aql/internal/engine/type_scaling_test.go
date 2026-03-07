package engine

import (
	"fmt"
	"testing"
	"time"
)

// --- Multi-level type construction and String() ---

func TestNewTypeMultiLevel(t *testing.T) {
	tests := []struct {
		path  string
		parts int
	}{
		{"a", 1},
		{"a/b", 2},
		{"a/b/c", 3},
		{"a/b/c/d", 4},
		{"a/b/c/d/e", 5},
		{"a/b/c/d/e/f", 6},
		{"a/b/c/d/e/f/g", 7},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			typ := NewType(tt.path)
			if len(typ.Parts) != tt.parts {
				t.Errorf("NewType(%q).Parts has %d elements, want %d", tt.path, len(typ.Parts), tt.parts)
			}
			if typ.String() != tt.path {
				t.Errorf("String() = %q, want %q", typ.String(), tt.path)
			}
		})
	}
}

// --- Specificity scales with depth ---

func TestSpecificityScalesWithDepth(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "a"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/%c", 'a'+i)
		}
		typ := NewType(path)
		if typ.Specificity() != depth {
			t.Errorf("depth %d: Specificity() = %d, want %d", depth, typ.Specificity(), depth)
		}
	}
}

// --- Supertype matches subtype (child matches parent pattern) ---

func TestSupertypeMatchesSubtype(t *testing.T) {
	levels := []string{
		"animal",
		"animal/mammal",
		"animal/mammal/canine",
		"animal/mammal/canine/dog",
		"animal/mammal/canine/dog/labrador",
		"animal/mammal/canine/dog/labrador/golden",
		"animal/mammal/canine/dog/labrador/golden/champion",
	}

	for i := 0; i < len(levels); i++ {
		child := NewType(levels[i])
		for j := 0; j <= i; j++ {
			parent := NewType(levels[j])
			if !child.Matches(parent) {
				t.Errorf("%q should match parent pattern %q", levels[i], levels[j])
			}
		}
	}
}

// --- Parent does NOT match child pattern ---

func TestParentDoesNotMatchChildPattern(t *testing.T) {
	levels := []string{
		"animal",
		"animal/mammal",
		"animal/mammal/canine",
		"animal/mammal/canine/dog",
		"animal/mammal/canine/dog/labrador",
		"animal/mammal/canine/dog/labrador/golden",
		"animal/mammal/canine/dog/labrador/golden/champion",
	}

	for i := 0; i < len(levels); i++ {
		parent := NewType(levels[i])
		for j := i + 1; j < len(levels); j++ {
			childPattern := NewType(levels[j])
			if parent.Matches(childPattern) {
				t.Errorf("%q should NOT match child pattern %q", levels[i], levels[j])
			}
		}
	}
}

// --- IsSubtypeOf for deep hierarchies ---

func TestIsSubtypeOfDeepHierarchy(t *testing.T) {
	levels := []string{
		"data",
		"data/numeric",
		"data/numeric/integer",
		"data/numeric/integer/signed",
		"data/numeric/integer/signed/i32",
		"data/numeric/integer/signed/i32/nonzero",
		"data/numeric/integer/signed/i32/nonzero/positive",
	}

	for i := 0; i < len(levels); i++ {
		typ := NewType(levels[i])

		// A type is NOT a subtype of itself
		if typ.IsSubtypeOf(typ) {
			t.Errorf("%q should NOT be a subtype of itself", levels[i])
		}

		// Each type is a subtype of all its ancestors
		for j := 0; j < i; j++ {
			ancestor := NewType(levels[j])
			if !typ.IsSubtypeOf(ancestor) {
				t.Errorf("%q should be a subtype of %q", levels[i], levels[j])
			}
		}

		// Each type is NOT a subtype of its descendants
		for j := i + 1; j < len(levels); j++ {
			descendant := NewType(levels[j])
			if typ.IsSubtypeOf(descendant) {
				t.Errorf("%q should NOT be a subtype of %q", levels[i], levels[j])
			}
		}
	}
}

// --- Equal for multi-level types ---

func TestEqualMultiLevel(t *testing.T) {
	paths := []string{
		"x",
		"x/y",
		"x/y/z",
		"x/y/z/w",
		"x/y/z/w/v",
		"x/y/z/w/v/u",
		"x/y/z/w/v/u/t",
	}
	for _, p := range paths {
		a := NewType(p)
		b := NewType(p)
		if !a.Equal(b) {
			t.Errorf("NewType(%q) should Equal itself", p)
		}
	}

	// Different paths should not be equal
	if NewType("a/b/c").Equal(NewType("a/b/d")) {
		t.Error("a/b/c should not equal a/b/d")
	}
	if NewType("a/b/c").Equal(NewType("a/b")) {
		t.Error("a/b/c should not equal a/b")
	}
}

// --- Sibling types do not match each other ---

func TestSiblingTypesDoNotMatch(t *testing.T) {
	siblings := []string{
		"vehicle/car/sedan",
		"vehicle/car/suv",
		"vehicle/truck/pickup",
		"vehicle/motorcycle/sport",
	}
	for i := 0; i < len(siblings); i++ {
		for j := i + 1; j < len(siblings); j++ {
			a := NewType(siblings[i])
			b := NewType(siblings[j])
			if a.Matches(b) {
				t.Errorf("%q should not match %q", siblings[i], siblings[j])
			}
			if b.Matches(a) {
				t.Errorf("%q should not match %q", siblings[j], siblings[i])
			}
		}
	}
}

// --- "any" matches deep types, deep types don't match "any" ---

func TestAnyMatchesDeepTypes(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "data"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/level%d", i)
		}
		typ := NewType(path)
		if !typ.Matches(TAny) {
			t.Errorf("depth %d: %q should match 'any'", depth, path)
		}
	}
}

// --- "scalar" matches number subtypes ---

func TestScalarMatchesNumberSubtypes(t *testing.T) {
	subtypes := []string{
		"number",
		"number/integer",
		"number/integer/5",
		"number/float",
		"number/float/double",
		"number/float/double/positive",
		"number/float/double/positive/small",
	}
	for _, path := range subtypes {
		typ := NewType(path)
		if !typ.Matches(TScalar) {
			t.Errorf("%q should match scalar", path)
		}
	}
}

// --- Well-known type relationships with number/integer ---

func TestNumberIntegerWellKnownRelationships(t *testing.T) {
	// number/integer matches number
	if !TInteger.Matches(TNumber) {
		t.Error("number/integer should match number")
	}
	// number/integer is a subtype of number
	if !TInteger.IsSubtypeOf(TNumber) {
		t.Error("number/integer should be subtype of number")
	}
	// number does NOT match number/integer
	if TNumber.Matches(TInteger) {
		t.Error("number should NOT match number/integer")
	}
	// number is NOT a subtype of number/integer
	if TNumber.IsSubtypeOf(TInteger) {
		t.Error("number should NOT be subtype of number/integer")
	}
	// number/integer matches any
	if !TInteger.Matches(TAny) {
		t.Error("number/integer should match any")
	}
	// number/integer matches scalar
	if !TInteger.Matches(TScalar) {
		t.Error("number/integer should match scalar")
	}
}

// ===== Unify tests for multi-level types =====

// --- Unify identical multi-level type literals ---

func TestUnifyIdenticalDeepTypeLiterals(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "category"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/sub%d", i)
		}
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			a := Value{VType: NewType(path), Data: nil}
			b := Value{VType: NewType(path), Data: nil}
			result, ok := Unify(a, b)
			if !ok {
				t.Fatalf("Unify should succeed for identical type %q", path)
			}
			if !result.VType.Equal(NewType(path)) {
				t.Errorf("result type = %s, want %s", result.VType, path)
			}
		})
	}
}

// --- Unify subtype with supertype returns narrower (subtype) ---

func TestUnifySubtypeWithSupertype(t *testing.T) {
	levels := []string{
		"number",
		"number/integer",
		"number/integer/42",
	}

	// Unifying each deeper type with each shallower type should return the deeper type
	for i := 1; i < len(levels); i++ {
		for j := 0; j < i; j++ {
			t.Run(fmt.Sprintf("%s_with_%s", levels[i], levels[j]), func(t *testing.T) {
				child := Value{VType: NewType(levels[i]), Data: nil}
				parent := Value{VType: NewType(levels[j]), Data: nil}

				// child unify parent
				result, ok := Unify(child, parent)
				if !ok {
					t.Fatalf("Unify(%s, %s) should succeed", levels[i], levels[j])
				}
				if !result.VType.Equal(NewType(levels[i])) {
					t.Errorf("Unify(%s, %s) = %s, want %s", levels[i], levels[j], result.VType, levels[i])
				}

				// parent unify child (reversed)
				result, ok = Unify(parent, child)
				if !ok {
					t.Fatalf("Unify(%s, %s) should succeed", levels[j], levels[i])
				}
				if !result.VType.Equal(NewType(levels[i])) {
					t.Errorf("Unify(%s, %s) = %s, want %s", levels[j], levels[i], result.VType, levels[i])
				}
			})
		}
	}
}

// --- Unify with "any" returns the specific type at all depths ---

func TestUnifyAnyWithDeepTypes(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "shape"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/sub%d", i)
		}
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			specific := Value{VType: NewType(path), Data: nil}
			any := Value{VType: TAny, Data: nil}

			// any unify specific
			result, ok := Unify(any, specific)
			if !ok {
				t.Fatal("Unify(any, specific) should succeed")
			}
			if !result.VType.Equal(NewType(path)) {
				t.Errorf("result type = %s, want %s", result.VType, path)
			}

			// specific unify any
			result, ok = Unify(specific, any)
			if !ok {
				t.Fatal("Unify(specific, any) should succeed")
			}
			if !result.VType.Equal(NewType(path)) {
				t.Errorf("result type = %s, want %s", result.VType, path)
			}
		})
	}
}

// --- Unify sibling types fails ---

func TestUnifySiblingTypesFails(t *testing.T) {
	siblings := [][2]string{
		{"number/integer", "number/float"},
		{"animal/mammal/cat", "animal/mammal/dog"},
		{"a/b/c/d/e/f/g1", "a/b/c/d/e/f/g2"},
	}
	for _, pair := range siblings {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0], pair[1]), func(t *testing.T) {
			a := Value{VType: NewType(pair[0]), Data: nil}
			b := Value{VType: NewType(pair[1]), Data: nil}
			_, ok := Unify(a, b)
			if ok {
				t.Errorf("Unify(%s, %s) should fail for siblings", pair[0], pair[1])
			}
		})
	}
}

// --- Unify concrete integer values with number/integer type ---

func TestUnifyIntegerWithNumberType(t *testing.T) {
	val := NewInteger(42) // type: number/integer/42
	numType := Value{VType: TNumber, Data: nil}

	// integer literal is subtype of number → should unify, returning the literal
	result, ok := Unify(val, numType)
	if !ok {
		t.Fatal("Unify(42, number) should succeed")
	}
	if result.AsInteger() != 42 {
		t.Errorf("expected 42, got %d", result.AsInteger())
	}
}

func TestUnifyIntegerWithIntegerType(t *testing.T) {
	val := NewInteger(7) // type: number/integer/7
	intType := Value{VType: TInteger, Data: nil}

	// number/integer/7 is subtype of number/integer → should unify
	result, ok := Unify(val, intType)
	if !ok {
		t.Fatal("Unify(7, number/integer) should succeed")
	}
	if result.AsInteger() != 7 {
		t.Errorf("expected 7, got %d", result.AsInteger())
	}
}

// --- Unify two different integer literals fails ---

func TestUnifyDifferentIntegerLiteralsFails(t *testing.T) {
	a := NewInteger(3) // number/integer/3
	b := NewInteger(5) // number/integer/5

	_, ok := Unify(a, b)
	if ok {
		t.Error("Unify(3, 5) should fail — different literal values")
	}
}

// --- Unify same integer literal succeeds ---

func TestUnifySameIntegerLiteralSucceeds(t *testing.T) {
	a := NewInteger(10)
	b := NewInteger(10)

	result, ok := Unify(a, b)
	if !ok {
		t.Fatal("Unify(10, 10) should succeed")
	}
	if result.AsInteger() != 10 {
		t.Errorf("expected 10, got %d", result.AsInteger())
	}
}

// --- Unify deeply nested custom types ---

func TestUnifyDeepHierarchy7Levels(t *testing.T) {
	// 7-level type hierarchy
	t1 := NewType("a/b/c/d/e/f/g")
	t2 := NewType("a/b/c/d/e/f")
	t3 := NewType("a/b/c/d/e")
	t4 := NewType("a/b/c/d")
	t5 := NewType("a/b/c")
	t6 := NewType("a/b")
	t7 := NewType("a")

	deepest := Value{VType: t1, Data: nil}
	ancestors := []Type{t2, t3, t4, t5, t6, t7}

	for _, anc := range ancestors {
		ancestor := Value{VType: anc, Data: nil}
		result, ok := Unify(deepest, ancestor)
		if !ok {
			t.Fatalf("Unify(%s, %s) should succeed", t1, anc)
		}
		// Should return the deeper (more specific) type
		if !result.VType.Equal(t1) {
			t.Errorf("Unify(%s, %s) = %s, want %s", t1, anc, result.VType, t1)
		}
	}
}

// --- Unify incompatible hierarchies fails ---

func TestUnifyIncompatibleHierarchiesFails(t *testing.T) {
	tests := [][2]string{
		{"number/integer", "string/proper"},
		{"a/b/c", "x/y/z"},
		{"data/num/int/i32/signed/big/huge", "data/str/utf8/ascii/printable/alpha/upper"},
	}
	for _, pair := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0], pair[1]), func(t *testing.T) {
			a := Value{VType: NewType(pair[0]), Data: nil}
			b := Value{VType: NewType(pair[1]), Data: nil}
			_, ok := Unify(a, b)
			if ok {
				t.Errorf("Unify(%s, %s) should fail for incompatible hierarchies", pair[0], pair[1])
			}
		})
	}
}

// --- Unify none with deep type fails ---

func TestUnifyNoneWithDeepTypeFails(t *testing.T) {
	none := Value{VType: TNone, Data: nil}
	deep := Value{VType: NewType("a/b/c/d/e/f/g"), Data: nil}
	_, ok := Unify(none, deep)
	if ok {
		t.Error("Unify(none, deep-type) should fail")
	}
	_, ok = Unify(deep, none)
	if ok {
		t.Error("Unify(deep-type, none) should fail")
	}
}

// --- Unify none with none succeeds ---

func TestUnifyNoneWithNoneSucceeds(t *testing.T) {
	a := Value{VType: TNone, Data: nil}
	b := Value{VType: TNone, Data: nil}
	_, ok := Unify(a, b)
	if !ok {
		t.Error("Unify(none, none) should succeed")
	}
}

// --- MatchSignature with multi-level type hierarchies ---

func TestMatchSignatureDeepTypeHierarchy(t *testing.T) {
	// Register signatures at different specificity levels
	sigs := []Signature{
		{Args: []Type{NewType("data")}, Handler: dummyHandler},                             // depth 1
		{Args: []Type{NewType("data/num")}, Handler: dummyHandler},                         // depth 2
		{Args: []Type{NewType("data/num/int")}, Handler: dummyHandler},                     // depth 3
		{Args: []Type{NewType("data/num/int/i32")}, Handler: dummyHandler},                 // depth 4
		{Args: []Type{NewType("data/num/int/i32/signed")}, Handler: dummyHandler},           // depth 5
		{Args: []Type{NewType("data/num/int/i32/signed/big")}, Handler: dummyHandler},       // depth 6
		{Args: []Type{NewType("data/num/int/i32/signed/big/huge")}, Handler: dummyHandler},  // depth 7
	}

	// A value with the deepest type should match the most specific signature
	val := Value{VType: NewType("data/num/int/i32/signed/big/huge"), Data: nil}
	stack := []Value{val}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(NewType("data/num/int/i32/signed/big/huge")) {
		t.Errorf("expected deepest match, got %s", m.Sig.Args[0])
	}
}

func TestMatchSignatureMidLevelType(t *testing.T) {
	// Only register signatures up to depth 4
	sigs := []Signature{
		{Args: []Type{NewType("data")}, Handler: dummyHandler},
		{Args: []Type{NewType("data/num")}, Handler: dummyHandler},
		{Args: []Type{NewType("data/num/int")}, Handler: dummyHandler},
		{Args: []Type{NewType("data/num/int/i32")}, Handler: dummyHandler},
	}

	// A value at depth 6 should match the deepest available signature (depth 4)
	val := Value{VType: NewType("data/num/int/i32/signed/big"), Data: nil}
	stack := []Value{val}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(NewType("data/num/int/i32")) {
		t.Errorf("expected data/num/int/i32 match, got %s", m.Sig.Args[0])
	}
}

// --- Unify with lists containing deep-typed values ---

func TestUnifyListsWithDeepTypedValues(t *testing.T) {
	// Two lists with integer values should unify element-by-element
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})

	result, ok := Unify(a, b)
	if !ok {
		t.Fatal("Unify of identical integer lists should succeed")
	}
	elems := result.AsList()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[0].AsInteger() != 1 || elems[1].AsInteger() != 2 {
		t.Errorf("unexpected values: %v", elems)
	}
}

// --- Unify maps with deep-typed values ---

func TestUnifyMapsWithDeepTypedValues(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(10))
	m1.Set("y", NewInteger(20))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(10))
	m2.Set("y", NewInteger(20))

	result, ok := Unify(NewMap(m1), NewMap(m2))
	if !ok {
		t.Fatal("Unify of identical integer-valued maps should succeed")
	}
	rm := result.AsMap()
	xv, _ := rm.Get("x")
	yv, _ := rm.Get("y")
	if xv.AsInteger() != 10 || yv.AsInteger() != 20 {
		t.Errorf("unexpected map values: x=%d y=%d", xv.AsInteger(), yv.AsInteger())
	}
}

// --- Unify maps with mismatched deep-typed values fails ---

func TestUnifyMapsWithMismatchedValuesFails(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(10))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(99))

	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("Unify of maps with different integer values should fail")
	}
}

// --- Unify symmetry: order of arguments doesn't matter ---

func TestUnifySymmetry(t *testing.T) {
	pairs := [][2]Value{
		{Value{VType: NewType("a/b/c"), Data: nil}, Value{VType: NewType("a/b"), Data: nil}},
		{Value{VType: NewType("number/integer"), Data: nil}, Value{VType: NewType("number"), Data: nil}},
		{Value{VType: TAny, Data: nil}, Value{VType: NewType("x/y/z/w"), Data: nil}},
		{NewInteger(5), Value{VType: TNumber, Data: nil}},
		{NewInteger(5), Value{VType: TInteger, Data: nil}},
	}

	for _, pair := range pairs {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0].VType, pair[1].VType), func(t *testing.T) {
			r1, ok1 := Unify(pair[0], pair[1])
			r2, ok2 := Unify(pair[1], pair[0])
			if ok1 != ok2 {
				t.Fatalf("asymmetric unification: (%s,%s)=%v but (%s,%s)=%v",
					pair[0].VType, pair[1].VType, ok1,
					pair[1].VType, pair[0].VType, ok2)
			}
			if ok1 && !r1.VType.Equal(r2.VType) {
				t.Errorf("asymmetric result: Unify(%s,%s)=%s but Unify(%s,%s)=%s",
					pair[0].VType, pair[1].VType, r1.VType,
					pair[1].VType, pair[0].VType, r2.VType)
			}
		})
	}
}

// ===== Efficiency tests: thousands of sibling types =====

// TestMatchesEfficiencyThousandsOfSiblings confirms that matching a/b/x against
// a/b is O(len(pattern)) and not affected by how many sibling types a/b/x<N> exist.
// We create 10,000 distinct types under the same parent and verify that matching
// each one against the parent pattern takes constant time per check.
func TestMatchesEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := NewType("a/b")

	// Create 10,000 sibling types: a/b/0, a/b/1, ..., a/b/9999
	siblings := make([]Type, numSiblings)
	for i := 0; i < numSiblings; i++ {
		siblings[i] = NewType(fmt.Sprintf("a/b/%d", i))
	}

	// Every sibling must match the parent
	for i, sib := range siblings {
		if !sib.Matches(parent) {
			t.Fatalf("sibling a/b/%d should match a/b", i)
		}
	}

	// Time the full pass: 10,000 Matches calls should be very fast (<50ms easily)
	start := time.Now()
	for _, sib := range siblings {
		sib.Matches(parent)
	}
	elapsed := time.Since(start)

	// 10,000 prefix comparisons should be sub-millisecond; generous 50ms limit
	if elapsed > 50*time.Millisecond {
		t.Errorf("10,000 Matches calls took %v — expected <50ms", elapsed)
	}
	t.Logf("10,000 Matches calls completed in %v", elapsed)
}

// TestIsSubtypeOfEfficiencyThousandsOfSiblings confirms IsSubtypeOf is O(len(parent)).
func TestIsSubtypeOfEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := NewType("a/b")

	siblings := make([]Type, numSiblings)
	for i := 0; i < numSiblings; i++ {
		siblings[i] = NewType(fmt.Sprintf("a/b/%d", i))
	}

	for i, sib := range siblings {
		if !sib.IsSubtypeOf(parent) {
			t.Fatalf("sibling a/b/%d should be subtype of a/b", i)
		}
	}

	start := time.Now()
	for _, sib := range siblings {
		sib.IsSubtypeOf(parent)
	}
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("10,000 IsSubtypeOf calls took %v — expected <50ms", elapsed)
	}
	t.Logf("10,000 IsSubtypeOf calls completed in %v", elapsed)
}

// TestMatchSignatureEfficiencyThousandsOfSiblings confirms that MatchSignature
// picks the right signature efficiently when a value has one of thousands of
// sibling types and only a parent-level signature is registered.
func TestMatchSignatureEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := NewType("a/b")

	sigs := []Signature{
		{Args: []Type{parent}, Handler: dummyHandler},
	}

	start := time.Now()
	for i := 0; i < numSiblings; i++ {
		val := Value{VType: NewType(fmt.Sprintf("a/b/%d", i)), Data: nil}
		m := MatchSignature(sigs, []Value{val}, WordInfo{ArgCount: -1})
		if m == nil {
			t.Fatalf("a/b/%d should match signature [a/b]", i)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("10,000 MatchSignature calls took %v — expected <200ms", elapsed)
	}
	t.Logf("10,000 MatchSignature calls completed in %v", elapsed)
}

// TestUnifyEfficiencyThousandsOfSiblings confirms Unify(a/b/x, a/b) is efficient
// across thousands of distinct sibling types.
func TestUnifyEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parentVal := Value{VType: NewType("a/b"), Data: nil}

	start := time.Now()
	for i := 0; i < numSiblings; i++ {
		child := Value{VType: NewType(fmt.Sprintf("a/b/%d", i)), Data: nil}
		result, ok := Unify(child, parentVal)
		if !ok {
			t.Fatalf("Unify(a/b/%d, a/b) should succeed", i)
		}
		// Should return the child (narrower type)
		if result.VType.Parts[2] != fmt.Sprintf("%d", i) {
			t.Fatalf("Unify(a/b/%d, a/b) returned wrong type: %s", i, result.VType)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("10,000 Unify calls took %v — expected <200ms", elapsed)
	}
	t.Logf("10,000 Unify calls completed in %v", elapsed)
}

// TestMatchConstantTimeRegardlessOfSiblingCount verifies that matching time
// does NOT grow with the number of existing sibling types. Compares timing
// with 100 siblings vs 10,000 siblings — both should be similar.
func TestMatchConstantTimeRegardlessOfSiblingCount(t *testing.T) {
	parent := NewType("prefix/mid")

	// Warm up: create and match 100 siblings
	small := make([]Type, 100)
	for i := range small {
		small[i] = NewType(fmt.Sprintf("prefix/mid/%d", i))
	}

	const iterations = 100_000
	start := time.Now()
	for n := 0; n < iterations; n++ {
		small[n%100].Matches(parent)
	}
	smallElapsed := time.Since(start)

	// Now create 10,000 siblings (types are independent structs, no registry)
	large := make([]Type, 10_000)
	for i := range large {
		large[i] = NewType(fmt.Sprintf("prefix/mid/%d", i))
	}

	start = time.Now()
	for n := 0; n < iterations; n++ {
		large[n%10_000].Matches(parent)
	}
	largeElapsed := time.Since(start)

	t.Logf("100 siblings: %v for %d iterations", smallElapsed, iterations)
	t.Logf("10,000 siblings: %v for %d iterations", largeElapsed, iterations)

	// The large set should not be more than 3x slower (generous margin for noise).
	// In reality they should be nearly identical since Matches is a simple prefix check.
	if largeElapsed > 3*smallElapsed+time.Millisecond {
		t.Errorf("matching with 10,000 siblings (%v) is significantly slower than 100 siblings (%v)",
			largeElapsed, smallElapsed)
	}
}
