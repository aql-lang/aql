package engine

import (
	"fmt"
	"strings"
)

// DepKind is a bit-field selector for the comparison encoded in a
// DepScalar value. One bit per primitive comparison; declaring it as
// a bit field (rather than a small enum) leaves room for combined
// constraints — a future range constraint can OR DepGTE | DepLTE into
// a single value with a low and a high bound. The current slice
// implements only the four single-comparison cases.
type DepKind uint8

const (
	DepGT  DepKind = 1 << iota // strictly greater than the bound
	DepGTE                     // greater than or equal to the bound
	DepLT                      // strictly less than the bound
	DepLTE                     // less than or equal to the bound
)

// String returns a short human-readable name for the comparison kind.
// Multiple bits set are joined with '|' so future combined constraints
// stay legible in error messages.
func (k DepKind) String() string {
	parts := make([]string, 0, 4)
	if k&DepGT != 0 {
		parts = append(parts, "gt")
	}
	if k&DepGTE != 0 {
		parts = append(parts, "gte")
	}
	if k&DepLT != 0 {
		parts = append(parts, "lt")
	}
	if k&DepLTE != 0 {
		parts = append(parts, "lte")
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, "|")
}

// DepScalarInfo is the payload carried by a Value of a Type/Dependent/Dep<X>
// type, where <X> is the leaf name of the base scalar type. The
// bit-field Kind selects which comparison(s) to apply against the
// concrete Bound value. The Bound's VType pins the base scalar type.
type DepScalarInfo struct {
	Kind  DepKind
	Bound Value
}

// NewDepScalar builds a DepScalar Value from a comparison kind and a
// concrete bound. The bound's VType determines the base type of the
// dependent constraint and selects the leaf name in the resulting
// Type/Dependent/Dep<Leaf> path. Returns a Value with a None VType
// (and no payload) if the bound is not a scalar — callers are
// expected to validate types before constructing.
func NewDepScalar(kind DepKind, bound Value) Value {
	leaf := dependentLeafFromBoundType(bound.VType)
	if leaf == "" {
		return Value{VType: TNone}
	}
	t, _ := NewType("Type/Dependent/Dep" + leaf)
	return newValue(t, DepScalarInfo{Kind: kind, Bound: bound})
}

// dependentLeafFromBoundType returns the leaf name to use in a
// Type/Dependent/Dep<Leaf> path for a given bound type. The leaf is
// the last *named* part of the bound's lattice path, looked up
// against the well-known scalar bases. Returns "" for unsupported
// bound types.
func dependentLeafFromBoundType(t Type) string {
	// Walk from the most specific path down: Integer/42 should yield
	// "Integer", not "42". The well-known scalar bases live at depth
	// ≤ 3, so any value-tagged subtype (e.g. Number/Integer/42) is
	// stripped down to its last named ancestor.
	for n := len(t.Parts); n >= 1; n-- {
		prefix := t.Parts[:n]
		switch {
		case len(prefix) == 3 && prefix[0] == "Scalar" && prefix[1] == "Number" && prefix[2] == "Integer":
			return "Integer"
		case len(prefix) == 3 && prefix[0] == "Scalar" && prefix[1] == "Number" && prefix[2] == "Decimal":
			return "Decimal"
		case len(prefix) == 2 && prefix[0] == "Scalar" && prefix[1] == "Number":
			return "Number"
		case len(prefix) == 2 && prefix[0] == "Scalar" && prefix[1] == "String":
			return "String"
		case len(prefix) == 2 && prefix[0] == "Scalar" && prefix[1] == "Boolean":
			return "Boolean"
		case len(prefix) == 2 && prefix[0] == "Scalar" && prefix[1] == "Atom":
			return "Atom"
		}
	}
	return ""
}

// dependentLeafBaseType returns the scalar base type for a given
// dependent leaf name, or (TNone, false) if the leaf is unknown.
func dependentLeafBaseType(leaf string) (Type, bool) {
	switch leaf {
	case "Integer":
		return TInteger, true
	case "Decimal":
		return TDecimal, true
	case "Number":
		return TNumber, true
	case "String":
		return TString, true
	case "Boolean":
		return TBoolean, true
	case "Atom":
		return TAtom, true
	}
	return TNone, false
}

// dependentLeafFromType extracts the leaf name from a Type/Dependent/
// Dep<Leaf> path, or "" if the type is not a dependent scalar path.
// Accepts trailing path components (forward-compat) so a future
// value-tagged DepInteger subtype keeps reporting "Integer".
func dependentLeafFromType(t Type) string {
	if len(t.Parts) < 3 || t.Parts[0] != "Type" || t.Parts[1] != "Dependent" {
		return ""
	}
	if !strings.HasPrefix(t.Parts[2], "Dep") {
		return ""
	}
	return strings.TrimPrefix(t.Parts[2], "Dep")
}

// IsDepScalar reports whether the value is any dependent scalar type.
func (v Value) IsDepScalar() bool {
	return dependentLeafFromType(v.VType) != ""
}

// AsDepScalar extracts the DepScalarInfo payload.
func (v Value) AsDepScalar() (DepScalarInfo, error) {
	if di, ok := v.Data.(DepScalarInfo); ok {
		return di, nil
	}
	return DepScalarInfo{}, fmt.Errorf("AsDepScalar: not a DepScalar value (got %T)", v.Data)
}

// --- DepInteger compatibility wrappers ---
//
// The first slice exposed Integer-specific names. The general DepScalar
// machinery now drives them; these wrappers keep the public API stable.

// NewDepInteger builds a DepInteger from a comparison kind and an int64
// bound. Equivalent to NewDepScalar(kind, NewInteger(bound)).
func NewDepInteger(kind DepKind, bound int64) Value {
	return NewDepScalar(kind, NewInteger(bound))
}

// IsDepInteger reports whether the value is the DepInteger flavour of
// DepScalar.
func (v Value) IsDepInteger() bool {
	return dependentLeafFromType(v.VType) == "Integer"
}

// DepIntegerInfo is the legacy payload shape; today it's a synonym
// for the integer flavour of DepScalarInfo. Bound is the int64 value
// extracted from the underlying scalar bound.
type DepIntegerInfo struct {
	Kind  DepKind
	Bound int64
}

// AsDepInteger returns the legacy int64-typed payload for a DepInteger
// value. Errors for non-integer dependents.
func (v Value) AsDepInteger() (DepIntegerInfo, error) {
	di, err := v.AsDepScalar()
	if err != nil {
		return DepIntegerInfo{}, err
	}
	if !v.IsDepInteger() {
		return DepIntegerInfo{}, fmt.Errorf("AsDepInteger: value is %s, not DepInteger", v.VType)
	}
	n, err := di.Bound.AsInteger()
	if err != nil {
		return DepIntegerInfo{}, fmt.Errorf("AsDepInteger: bound: %w", err)
	}
	return DepIntegerInfo{Kind: di.Kind, Bound: n}, nil
}

// depScalarCheck returns true if `value` satisfies every comparison
// bit set in `info.Kind` against `info.Bound`. Comparisons go through
// compareValues so any orderable scalar type is supported (numbers,
// strings, booleans, atoms). Bits are AND-combined so a future range
// constraint (DepGTE | DepLTE) requires both halves.
func depScalarCheck(info DepScalarInfo, value Value) bool {
	if info.Kind == 0 {
		return false
	}
	cmp, err := compareValues(value, info.Bound)
	if err != nil {
		return false
	}
	if info.Kind&DepGT != 0 && !(cmp > 0) {
		return false
	}
	if info.Kind&DepGTE != 0 && !(cmp >= 0) {
		return false
	}
	if info.Kind&DepLT != 0 && !(cmp < 0) {
		return false
	}
	if info.Kind&DepLTE != 0 && !(cmp <= 0) {
		return false
	}
	return true
}

// depIntegerCheck is the integer-specific shim; kept for any caller
// using the int64 form directly.
func depIntegerCheck(info DepIntegerInfo, n int64) bool {
	return depScalarCheck(DepScalarInfo{Kind: info.Kind, Bound: NewInteger(info.Bound)}, NewInteger(n))
}
