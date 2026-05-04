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
//
// Kind2/Bound2 form an optional second constraint, used for
// interval-style refinement (lower AND upper bound). When Kind2 is
// zero the value carries a single comparison and behaves exactly as
// the original implementation; when both are set, the value matches
// iff both comparisons hold against the same input. The two halves
// must be on opposite sides of the lattice (one lower-style — GT or
// GTE — and one upper-style — LT or LTE); same-side combinations are
// always tightened to a single bound during construction or
// unification, so they never reach the dual-storage form.
type DepScalarInfo struct {
	Kind   DepKind
	Bound  Value
	Kind2  DepKind
	Bound2 Value
}

// isLowerKind reports whether k bounds the value from below.
func isLowerKind(k DepKind) bool { return k == DepGT || k == DepGTE }

// isUpperKind reports whether k bounds the value from above.
func isUpperKind(k DepKind) bool { return k == DepLT || k == DepLTE }

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

// depScalarCheck returns true if `value` satisfies every comparison
// in info against the corresponding bound. The primary comparison
// (Kind/Bound) is required; the secondary (Kind2/Bound2) is also
// applied when Kind2 != 0. Both halves are AND-combined.
func depScalarCheck(info DepScalarInfo, value Value) bool {
	if info.Kind == 0 {
		return false
	}
	if !depCompareCheck(info.Kind, info.Bound, value) {
		return false
	}
	if info.Kind2 != 0 && !depCompareCheck(info.Kind2, info.Bound2, value) {
		return false
	}
	return true
}

// depCompareCheck applies a single comparison kind to value vs bound.
// Returns false on any compareValues error so cross-type comparisons
// (e.g. Integer DepScalar vs String value) reject cleanly.
func depCompareCheck(kind DepKind, bound, value Value) bool {
	cmp, err := compareValues(value, bound)
	if err != nil {
		return false
	}
	if kind&DepGT != 0 && !(cmp > 0) {
		return false
	}
	if kind&DepGTE != 0 && !(cmp >= 0) {
		return false
	}
	if kind&DepLT != 0 && !(cmp < 0) {
		return false
	}
	if kind&DepLTE != 0 && !(cmp <= 0) {
		return false
	}
	return true
}

// depScalarsEqual reports whether two DepScalar payloads describe the
// same constraint: same primary kind/bound and same secondary
// kind/bound (if any). Bound comparison delegates to valuesEqual so
// the underlying scalar payload is compared structurally.
func depScalarsEqual(a, b DepScalarInfo) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind != 0 {
		if !a.Bound.VType.Equal(b.Bound.VType) {
			return false
		}
		if !valuesEqual(a.Bound, b.Bound) {
			return false
		}
	}
	if a.Kind2 != b.Kind2 {
		return false
	}
	if a.Kind2 != 0 {
		if !a.Bound2.VType.Equal(b.Bound2.VType) {
			return false
		}
		if !valuesEqual(a.Bound2, b.Bound2) {
			return false
		}
	}
	return true
}

// formatDepScalar renders a DepScalar's display form, surfacing the
// secondary bound when present. Single-bound is "(Leaf op bound)";
// interval is "(Leaf op1 bound1 op2 bound2)".
func formatDepScalar(leaf string, info DepScalarInfo) string {
	if info.Kind2 == 0 {
		return fmt.Sprintf("(%s %s %s)", leaf, info.Kind, info.Bound.String())
	}
	return fmt.Sprintf("(%s %s %s %s %s)", leaf,
		info.Kind, info.Bound.String(),
		info.Kind2, info.Bound2.String())
}

// renderDepScalar is the canonical Value-shaped wrapper around
// formatDepScalar. Every display surface in the engine — Value.String,
// valToString, formatValueJSON, formatForPrint, aql_error stack
// rendering — funnels DepScalar values through here so the surface
// representation stays consistent across paths and the
// IsDepScalar→AsDepScalar dance happens in exactly one place.
//
// Returns the empty string if v isn't a DepScalar so callers can use
// `if s := renderDepScalar(v); s != ""` as a guarded alternative
// branch.
func renderDepScalar(v Value) string {
	if !v.IsDepScalar() {
		return ""
	}
	info, err := v.AsDepScalar()
	if err != nil {
		return ""
	}
	return formatDepScalar(dependentLeafFromType(v.VType), info)
}

// tightenSameSide combines two same-side comparisons (both lower or
// both upper) into the tighter single bound. The caller has verified
// k1 and k2 are both lower-style or both upper-style. Errors on the
// underlying compare propagate via ok=false (treated as Never).
func tightenSameSide(k1 DepKind, b1 Value, k2 DepKind, b2 Value) (DepKind, Value, bool) {
	cmp, err := compareValues(b1, b2)
	if err != nil {
		return 0, Value{}, false
	}
	// For lower bounds, the larger value is tighter; for upper bounds,
	// the smaller. When the bounds are equal, prefer the strict form
	// (GT over GTE, LT over LTE) — it's the narrower constraint.
	lower := isLowerKind(k1)
	if cmp == 0 {
		strictGT := k1 == DepGT || k2 == DepGT
		strictLT := k1 == DepLT || k2 == DepLT
		if lower {
			if strictGT {
				return DepGT, b1, true
			}
			return DepGTE, b1, true
		}
		if strictLT {
			return DepLT, b1, true
		}
		return DepLTE, b1, true
	}
	if (lower && cmp > 0) || (!lower && cmp < 0) {
		return k1, b1, true
	}
	return k2, b2, true
}

// combineDepScalars computes the intersection of two single- or
// dual-comparison DepScalar constraints over the same base type.
// Returns ok=false when the result is empty (no value satisfies both).
//
//	(Integer gt 5) tand (Integer lt 10) → interval (5, 10)
//	(Integer gte 10) tand (Integer gte 5) → gte 10 (tighter)
//	(Integer gt 10) tand (Integer lt 5) → empty (Never)
func combineDepScalars(a, b DepScalarInfo) (DepScalarInfo, bool) {
	// Split each side into lower and upper components.
	type comp struct {
		kind  DepKind
		bound Value
		set   bool
	}
	var lo, hi comp
	add := func(k DepKind, v Value) bool {
		if k == 0 {
			return true
		}
		c := &lo
		if isUpperKind(k) {
			c = &hi
		}
		if !c.set {
			c.kind, c.bound, c.set = k, v, true
			return true
		}
		nk, nv, ok := tightenSameSide(c.kind, c.bound, k, v)
		if !ok {
			return false
		}
		c.kind, c.bound = nk, nv
		return true
	}
	for _, p := range []struct {
		k DepKind
		v Value
	}{{a.Kind, a.Bound}, {a.Kind2, a.Bound2}, {b.Kind, b.Bound}, {b.Kind2, b.Bound2}} {
		if !add(p.k, p.v) {
			return DepScalarInfo{}, false
		}
	}
	// If both sides are present, verify the interval is non-empty.
	if lo.set && hi.set {
		cmp, err := compareValues(lo.bound, hi.bound)
		if err != nil {
			return DepScalarInfo{}, false
		}
		if cmp > 0 {
			return DepScalarInfo{}, false
		}
		// Equal bounds: empty unless both sides are inclusive.
		if cmp == 0 && (lo.kind == DepGT || hi.kind == DepLT) {
			return DepScalarInfo{}, false
		}
	}
	out := DepScalarInfo{}
	if lo.set {
		out.Kind, out.Bound = lo.kind, lo.bound
		if hi.set {
			out.Kind2, out.Bound2 = hi.kind, hi.bound
		}
	} else if hi.set {
		out.Kind, out.Bound = hi.kind, hi.bound
	}
	return out, true
}

// makeDepScalarSig builds the [TScalar, TScalarType] -> [TDependent]
// signature variant for a comparison op. `Integer gte 10`, `String lt
// "z"`, `Decimal gte 1.5` all hit this sig: arg0 is the bound, arg1 is
// the base-type literal. The result type path is Type/Dependent/Dep<X>
// where <X> is the leaf of the base type. This sig sorts ahead of the
// [Any, Any] boolean sig (because its types are more specific), so
// concrete `5 gte 10` still hits the boolean branch via the second
// match attempt.
//
// Used by RegisterComparison to wire the same single-bound DepScalar
// constructor onto each of `lt`, `gt`, `lte`, `gte`.
func makeDepScalarSig(opName string, kind DepKind) NativeSig {
	return NativeSig{
		Args: []Type{TScalar, TScalarType},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			// arg1 is the type-literal at the deep position. Reject
			// non-leaf bases — only the well-known scalar types map
			// to a Dependent leaf name.
			if args[1].Data != nil {
				return nil, fmt.Errorf("%s: dependent constructor needs a scalar type literal, got concrete %s",
					opName, args[1].VType.String())
			}
			leaf := dependentLeafFromBoundType(args[1].VType)
			if leaf == "" {
				return nil, fmt.Errorf("%s: dependent constructor does not support base type %s",
					opName, args[1].VType.String())
			}
			// Bound must be the same scalar base as the type literal.
			base, _ := dependentLeafBaseType(leaf)
			if !args[0].VType.Matches(base) {
				return nil, fmt.Errorf("%s: bound %s does not match dependent base %s",
					opName, args[0].VType.String(), base.String())
			}
			return []Value{NewDepScalar(kind, args[0])}, nil
		},
		Returns: []Type{TDependent},
	}
}

// RegisterBetween registers `between`: a closed-interval DepScalar
// constructor. `Integer between 10 20` ≡ `(Integer gte 10) tand
// (Integer lte 20)` but in one word. Sig follows the concatenative
// mirror pattern: sig[0]=lo (innermost forward), sig[1]=hi, sig[2]=
// type (deepest, taken from the stack when the type is prefixed).
//
// Inverted bounds (lo > hi) collapse to Never; equal bounds form a
// singleton interval.
func RegisterBetween(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "between",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TScalar, TScalar, TScalarType},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if args[2].Data != nil {
						return nil, fmt.Errorf("between: type arg must be a scalar type literal, got concrete %s",
							args[2].VType.String())
					}
					leaf := dependentLeafFromBoundType(args[2].VType)
					if leaf == "" {
						return nil, fmt.Errorf("between: unsupported base type %s",
							args[2].VType.String())
					}
					base, _ := dependentLeafBaseType(leaf)
					if !args[0].VType.Matches(base) {
						return nil, fmt.Errorf("between: low bound %s does not match base %s",
							args[0].VType.String(), base.String())
					}
					if !args[1].VType.Matches(base) {
						return nil, fmt.Errorf("between: high bound %s does not match base %s",
							args[1].VType.String(), base.String())
					}
					cmp, err := compareValues(args[0], args[1])
					if err != nil {
						return nil, fmt.Errorf("between: %w", err)
					}
					if cmp > 0 {
						return []Value{NewTypeLiteral(TNever)}, nil
					}
					info := DepScalarInfo{
						Kind: DepGTE, Bound: args[0],
						Kind2: DepLTE, Bound2: args[1],
					}
					t, err := NewType("Type/Dependent/Dep" + leaf)
					if err != nil {
						return nil, fmt.Errorf("between: %w", err)
					}
					return []Value{newValue(t, info)}, nil
				},
				Returns: []Type{TDependent},
			},
		},
	})
}
