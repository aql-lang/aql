package eng

// unifyDepScalar handles unification when at least one side is a
// DepScalar (refined scalar with bounds, e.g. `Integer ≥10`).
//
// Three cases:
//  1. DepScalar vs concrete scalar: succeeds iff the scalar's type
//     matches the base AND the value satisfies the comparison. Returns
//     the plain scalar (not the DepScalar) so downstream consumers see
//     a normal value.
//  2. DepScalar vs DepScalar over the same base: combine the
//     constraints (intersection) — same-side bounds tighten,
//     opposite-side bounds form an interval. Empty result fails.
//  3. DepScalar vs DepScalar over different bases: fails (incompatible
//     bases).
func unifyDepScalar(a Value, sa ValueShape, b Value, sb ValueShape) (Value, bool) {
	// Both DepScalar: intersect constraints.
	if sa == ShapeDepScalar && sb == ShapeDepScalar {
		aType := denotedType(a)
		bType := denotedType(b)
		if !aType.Equal(bType) {
			return Value{}, false
		}
		aInfo, err := a.AsDepScalar()
		if err != nil {
			return Value{}, false
		}
		bInfo, err := b.AsDepScalar()
		if err != nil {
			return Value{}, false
		}
		combined, ok := combineDepScalars(aInfo, bInfo)
		if !ok {
			return Value{}, false
		}
		return NewValueRaw(aType, combined), true
	}

	// Canonicalize: dep on the left, other on the right.
	var dep, other Value
	if sa == ShapeDepScalar {
		dep, other = a, b
	} else {
		dep, other = b, a
	}

	// DepScalar vs type literal / carrier: not a DepScalar concern —
	// fall through to general subtype narrowing. The DepScalar's
	// Parent is its base scalar, so the subtype walk handles e.g.
	// `Pos refine Integer` (Pos sub Integer) by returning the
	// narrower side.
	if other.Data == nil {
		return unifySameOrSubtype(dep, other)
	}

	// DepScalar vs concrete scalar over the same base.
	depType := denotedType(dep)
	otherType := denotedType(other)
	if !otherType.Matches(depType) {
		return Value{}, false
	}
	info, err := dep.AsDepScalar()
	if err != nil {
		return Value{}, false
	}
	if depScalarCheck(info, other) {
		return other, true
	}
	return Value{}, false
}
