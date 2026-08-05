package eng

// Core carrier constructors (Stage 2c of the four-piece split, seam S3b):
// the pure Value constructors the core dispatch/unification paths call
// (sigTypeMatches, the gradual arms). The check-piece carrier MACHINERY
// stays in carrier.go; these are ~20-line constructors over core Value.

// NewCarrier constructs a carrier Value for the given type. Data is
// nil for scalar types. For TList and TMap, Data is set to a
// ChildTypeInfo wrapping an Any carrier so the carrier satisfies
// positionalMatch's "concrete list/map" rule (it rejects values
// whose Data==nil when the signature requires a concrete TList or
// TMap). Typed-list carriers (element type known) are produced via
// NewCarrierTypedList / NewCarrierTypedListValue.
func NewCarrier(t *Type) Value {
	v := NewValueRaw(t, nil)
	v.Carrier = true
	if t.Equal(TList) || t.Equal(TMap) {
		v.Data = ChildTypeInfo{Child: Value{Parent: TAny, Carrier: true}}
	}
	return v
}

// NewDynamicCarrier constructs a bounded gradual carrier dynamic(t):
// a carrier whose Parent is the BOUND t and whose Dynamic flag flips
// matching to the not-disjoint rule (design/dynamic-modality-report.10.md).
// dynamic(Any) is the classic gradual `any` — compatible with every
// slot. Use this at an escape hatch where the checker has a best static
// bound but cannot prove the exact type.
func NewDynamicCarrier(t *Type) Value {
	v := NewCarrier(t)
	v.Dynamic = true
	return v
}

// carrierOfLiteral converts a bare type-literal value (the node
// itself, as stored in DisjunctInfo.Alternatives) into a carrier OF
// that node — i.e. Parent points at the literal's type, not at the
// literal's lattice parent.
func CarrierOfLiteral(lit Value) Value {
	lt := lit
	return NewCarrier(&lt)
}
