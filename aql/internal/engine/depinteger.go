package engine

import "fmt"

// DepKind is a bit-field selector for the comparison encoded in a
// DepInteger value. One bit per primitive comparison; declaring it as
// a bit field (rather than a small enum) leaves room for combined
// constraints — a future range constraint can OR DepGTE | DepLTE into
// a single value with a low and a high bound. The first slice
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
	out := parts[0]
	for _, p := range parts[1:] {
		out += "|" + p
	}
	return out
}

// DepIntegerInfo is the payload carried by a Value of type
// Type/Dependent/DepInteger. The bit-field Kind selects which
// comparison to apply against the integer Bound.
type DepIntegerInfo struct {
	Kind  DepKind
	Bound int64
}

// NewDepInteger builds a DepInteger Value with the given comparison
// kind and bound.
func NewDepInteger(kind DepKind, bound int64) Value {
	return newValue(TDepInteger, DepIntegerInfo{Kind: kind, Bound: bound})
}

// IsDepInteger reports whether the value is a DepInteger.
func (v Value) IsDepInteger() bool {
	return v.VType.Equal(TDepInteger)
}

// AsDepInteger extracts the DepIntegerInfo payload.
func (v Value) AsDepInteger() (DepIntegerInfo, error) {
	if di, ok := v.Data.(DepIntegerInfo); ok {
		return di, nil
	}
	return DepIntegerInfo{}, fmt.Errorf("AsDepInteger: not a DepInteger value (got %T)", v.Data)
}

// depIntegerCheck returns true if `n` satisfies every comparison bit
// set in `info.Kind` against `info.Bound`. Bits are AND-combined so a
// future range constraint (DepGTE | DepLTE) requires both halves.
func depIntegerCheck(info DepIntegerInfo, n int64) bool {
	if info.Kind == 0 {
		return false
	}
	if info.Kind&DepGT != 0 && !(n > info.Bound) {
		return false
	}
	if info.Kind&DepGTE != 0 && !(n >= info.Bound) {
		return false
	}
	if info.Kind&DepLT != 0 && !(n < info.Bound) {
		return false
	}
	if info.Kind&DepLTE != 0 && !(n <= info.Bound) {
		return false
	}
	return true
}
