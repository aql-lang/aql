package eng

// depScalarUnifier is the kernel-installed Unifier for user-defined
// refined-scalar types — types whose body is a DepScalar with a
// comparison constraint, e.g. `def Big (Integer gt 10)`.
//
// Before this Unifier existed, InstallType's generic "else" branch
// minted a lattice node parented at the base scalar (Integer) but
// left Behavior as DefaultBehavior. That meant `100.Is(Big)` did a
// plain lattice walk — 100's parent chain is Integer → Number → …,
// none of which is Big — so dispatch rejected every value, even ones
// that satisfied the predicate.
//
// The Unifier:
//   - Gate 1: v's type must satisfy the DepScalar's base type
//     (`Big` constrains Integers — 100 passes, "hi" fails).
//   - Gate 2: v's payload must satisfy the constraint
//     (depScalarCheck handles the bounds check).
//
// Bare type literals (Data==nil, !Carrier) pass through to the
// prev/DefaultBehavior walk — the type itself isn't an inhabitant.
type depScalarUnifier struct {
	prev     TypeBehavior
	baseType *Type
	depInfo  DepScalarInfo
	typeName string
}

func (d *depScalarUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(v) {
		if d.prev != nil {
			return d.prev.Match(v, t)
		}
		return DefaultBehavior.Match(v, t)
	}
	if !v.Parent.Matches(d.baseType) {
		return false
	}
	return depScalarCheck(d.depInfo, v)
}

func (d *depScalarUnifier) Format(v Value) string {
	if d.prev != nil {
		return d.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}

func (d *depScalarUnifier) Equal(a, b Value) bool {
	if d.prev != nil {
		return d.prev.Equal(a, b)
	}
	return DefaultBehavior.Equal(a, b)
}

// installDepScalarUnifier attaches a depScalarUnifier to def. Called
// by InstallType when minting a DepScalar-bodied user type so the
// constraint runs at every Is/Match call site (sig dispatch, the `is`
// word, options/record/Make slot checks).
func installDepScalarUnifier(def *Type, base *Type, info DepScalarInfo, name string) {
	def.Behavior = &depScalarUnifier{
		prev:     def.Behavior,
		baseType: base,
		depInfo:  info,
		typeName: name,
	}
}

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
func unifyDepScalar(a Value, sa ValueShape, b Value, sb ValueShape) (Value, *UnifyError) {
	// Both DepScalar: intersect constraints.
	if sa == ShapeDepScalar && sb == ShapeDepScalar {
		aType := denotedType(a)
		bType := denotedType(b)
		if !aType.Equal(bType) {
			return Value{}, unifyFail("DepScalar bases do not match", a, b)
		}
		aInfo, err := a.AsDepScalar()
		if err != nil {
			return Value{}, unifyFail("could not read DepScalar payload on left", a, b)
		}
		bInfo, err := b.AsDepScalar()
		if err != nil {
			return Value{}, unifyFail("could not read DepScalar payload on right", a, b)
		}
		combined, ok := combineDepScalars(aInfo, bInfo)
		if !ok {
			return Value{}, unifyFail("DepScalar constraint intersection is empty", a, b)
		}
		return NewValueRaw(aType, combined), nil
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
	if !IsConcrete(other) {
		return unifySameOrSubtype(dep, other)
	}

	// DepScalar vs concrete scalar over the same base.
	depType := denotedType(dep)
	otherType := denotedType(other)
	if !otherType.Matches(depType) {
		return Value{}, unifyFail("DepScalar base type does not match value's type", a, b)
	}
	info, err := dep.AsDepScalar()
	if err != nil {
		return Value{}, unifyFail("could not read DepScalar payload", a, b)
	}
	if depScalarCheck(info, other) {
		return other, nil
	}
	return Value{}, unifyFail("value does not satisfy DepScalar bounds", a, b)
}
