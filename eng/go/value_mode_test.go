package eng

import "testing"

// TestValueModePredicates pins the three "mode" predicates against every
// value mode the engine produces. It exists to lock in the distinctions
// that motivated draining the raw `Data == nil` probes: in particular
// that IsConcrete is NOT the negation of `Data == nil` (a list/map
// carrier carries a ChildTypeInfo payload yet is not concrete), and that
// IsBareTypeNode includes None while IsTypeLiteral excludes it.
func TestValueModePredicates(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		// expected predicate results
		concrete  bool // IsConcrete
		bareNode  bool // IsBareTypeNode == (Data == nil && !Carrier)
		typeLit   bool // IsTypeLiteral  == bareNode && !None
		dataIsNil bool // raw Data == nil — documents the divergence
	}{
		{"concrete scalar", NewInteger(5), true, false, false, false},
		{"concrete list", NewList([]Value{NewInteger(1)}), true, false, false, false},
		{"none value", NewNone(), true, false, false, false},

		{"scalar type literal", NewTypeLiteral(TInteger), false, true, true, true},
		{"map type literal", NewTypeLiteral(TMap), false, true, true, true},
		{"None type literal", NewTypeLiteral(TNone), false, true, false, true},
		{"Any type literal", NewTypeLiteral(TAny), false, true, true, true},
		{"Never type literal", NewTypeLiteral(TNever), false, true, true, true},

		{"scalar carrier", NewCarrier(TInteger), false, false, false, true},
		// The load-bearing case: a list/map carrier carries a
		// ChildTypeInfo payload (Data != nil) so it satisfies
		// positionalMatch's concrete-list/map rule, yet it is not a
		// concrete value. `Data == nil` is therefore false here while
		// IsConcrete is also false — the two are NOT inverses.
		{"list carrier", NewCarrier(TList), false, false, false, false},
		{"map carrier", NewCarrier(TMap), false, false, false, false},
		{"typed-list carrier", NewCarrierTypedList(TInteger), false, false, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsConcrete(c.v); got != c.concrete {
				t.Errorf("IsConcrete = %v, want %v", got, c.concrete)
			}
			if got := IsBareTypeNode(c.v); got != c.bareNode {
				t.Errorf("IsBareTypeNode = %v, want %v", got, c.bareNode)
			}
			if got := IsTypeLiteral(c.v); got != c.typeLit {
				t.Errorf("IsTypeLiteral = %v, want %v", got, c.typeLit)
			}
			if got := (c.v.Data == nil); got != c.dataIsNil {
				t.Errorf("Data == nil = %v, want %v", got, c.dataIsNil)
			}
			// IsBareTypeNode is exactly `Data == nil && !Carrier`.
			if want := c.v.Data == nil && !c.v.Carrier; IsBareTypeNode(c.v) != want {
				t.Errorf("IsBareTypeNode must equal (Data==nil && !Carrier): got %v want %v",
					IsBareTypeNode(c.v), want)
			}
		})
	}
}
