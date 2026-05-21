package native

import "testing"

func TestCompareValuesIntegers(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		want int
	}{
		{"less", 1, 2, -1},
		{"greater", 3, 1, 1},
		{"equal", 5, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewInteger(tt.a), NewInteger(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesStrings(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"less", "abc", "def", -1},
		{"greater", "xyz", "abc", 1},
		{"equal", "same", "same", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewString(tt.a), NewString(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesBooleans(t *testing.T) {
	tests := []struct {
		name string
		a, b bool
		want int
	}{
		{"false_lt_true", false, true, -1},
		{"true_gt_false", true, false, 1},
		{"equal_true", true, true, 0},
		{"equal_false", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewBoolean(tt.a), NewBoolean(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompareValuesAtoms(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"less", "abc", "def", -1},
		{"greater", "xyz", "abc", 1},
		{"equal", "foo", "foo", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewAtom(tt.a), NewAtom(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompareValuesCrossScalar(t *testing.T) {
	// Cross-branch scalar pairs are ordered by the Scalar root's
	// Comparer: Path < String < Number < Boolean < Atom.
	path := NewPath([]string{"a"}, false)
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"path_lt_string", path, NewString("a"), -1},
		{"string_gt_path", NewString("a"), path, 1},
		{"string_lt_number", NewString("a"), NewInteger(1), -1},
		{"number_gt_string", NewInteger(1), NewString("a"), 1},
		{"number_lt_boolean", NewInteger(1), NewBoolean(false), -1},
		{"boolean_gt_number", NewBoolean(false), NewInteger(1), 1},
		{"boolean_lt_atom", NewBoolean(true), NewAtom("z"), -1},
		{"atom_gt_boolean", NewAtom("z"), NewBoolean(true), 1},
		{"path_lt_atom", path, NewAtom("z"), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesPaths(t *testing.T) {
	// Path-vs-Path falls through to the Scalar comparator, which
	// orders paths by segment count (longer first), then compares
	// equal-length paths segment by segment.
	abc := NewPath([]string{"a", "b", "c"}, false) // 3 segments
	ab := NewPath([]string{"a", "b"}, false)       // 2 segments
	ac := NewPath([]string{"a", "c"}, false)       // 2 segments
	zzz := NewPath([]string{"z", "z", "z"}, false) // 3 segments
	aDashA := NewPath([]string{"a-", "a"}, false)  // 2 segments
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"longer_sorts_first", abc, ab, -1},
		{"shorter_sorts_after", ab, abc, 1},
		{"length_beats_lexical", zzz, ab, -1},
		{"equal_len_segment", ab, ac, -1},
		{"equal_len_segment_rev", ac, ab, 1},
		{"per_element_beats_render", ab, aDashA, -1}, // "a" < "a-" at segment 0
		{"identical", ab, ab, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesIncomparableTypes(t *testing.T) {
	// A scalar and a non-scalar share no comparable ancestor (their
	// LCA is the bare Any root) and still error.
	if _, err := CompareValues(NewInteger(1), NewList([]Value{NewInteger(2)})); err == nil {
		t.Fatal("expected error comparing Integer with List")
	}
}

func TestCompareValuesListError(t *testing.T) {
	_, err := CompareValues(NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(2)}))
	if err == nil {
		t.Fatal("expected error for list comparison")
	}
}

func TestExactEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"int_equal", NewInteger(1), NewInteger(1), true},
		{"int_notequal", NewInteger(1), NewInteger(2), false},
		{"str_equal", NewString("abc"), NewString("abc"), true},
		{"str_notequal", NewString("a"), NewString("b"), false},
		{"bool_equal", NewBoolean(true), NewBoolean(true), true},
		{"bool_notequal", NewBoolean(true), NewBoolean(false), false},
		{"atom_equal", NewAtom("x"), NewAtom("x"), true},
		{"atom_notequal", NewAtom("x"), NewAtom("y"), false},
		{"none_equal", NewTypeLiteral(TNone), NewTypeLiteral(TNone), true},
		{"cross_type", NewInteger(1), NewString("1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExactEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ExactEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExactEqualDifferentTypes(t *testing.T) {
	// Different types should not be equal
	if ExactEqual(NewInteger(1), NewBoolean(true)) {
		t.Error("int and bool should not be exactly equal")
	}
	if ExactEqual(NewString("1"), NewAtom("1")) {
		t.Error("string and atom should not be exactly equal")
	}
}

func TestExactEqualMapIdentity(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	a := NewMap(m)
	if !ExactEqual(a, a) {
		t.Error("same map should be equal to itself")
	}
}

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"int_equal", NewInteger(1), NewInteger(1), true},
		{"int_notequal", NewInteger(1), NewInteger(2), false},
		{"str_equal", NewString("abc"), NewString("abc"), true},
		{"str_notequal", NewString("a"), NewString("b"), false},
		{"bool_equal", NewBoolean(true), NewBoolean(true), true},
		{"bool_notequal", NewBoolean(true), NewBoolean(false), false},
		{"atom_equal", NewAtom("x"), NewAtom("x"), true},
		{"atom_notequal", NewAtom("x"), NewAtom("y"), false},
		{"none_equal", NewTypeLiteral(TNone), NewTypeLiteral(TNone), true},
		{"cross_type", NewInteger(1), NewString("1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("DeepEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeepEqualLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	b := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	c := NewList([]Value{NewInteger(1), NewInteger(2)})
	d := NewList([]Value{NewInteger(1), NewInteger(9), NewInteger(3)})

	if !DeepEqual(a, b) {
		t.Error("equal lists should be deeply equal")
	}
	if DeepEqual(a, c) {
		t.Error("different length lists should not be deeply equal")
	}
	if DeepEqual(a, d) {
		t.Error("different element lists should not be deeply equal")
	}
}

func TestDeepEqualMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m1.Set("y", NewInteger(2))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("y", NewInteger(2))

	m3 := NewOrderedMap()
	m3.Set("x", NewInteger(1))
	m3.Set("y", NewInteger(9))

	m4 := NewOrderedMap()
	m4.Set("x", NewInteger(1))

	if !DeepEqual(NewMap(m1), NewMap(m2)) {
		t.Error("equal maps should be deeply equal")
	}
	if DeepEqual(NewMap(m1), NewMap(m3)) {
		t.Error("different value maps should not be deeply equal")
	}
	if DeepEqual(NewMap(m1), NewMap(m4)) {
		t.Error("different size maps should not be deeply equal")
	}
}

func TestDeepEqualMapsMissingKey(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m1.Set("y", NewInteger(2))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("z", NewInteger(2))

	if DeepEqual(NewMap(m1), NewMap(m2)) {
		t.Error("maps with different keys should not be deeply equal")
	}
}
