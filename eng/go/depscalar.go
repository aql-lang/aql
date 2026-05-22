package eng

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

// DepBound is one side of a dependent-scalar constraint: a comparison
// against a concrete bound. Inclusive distinguishes weak (≤, ≥) from
// strict (<, >). The bound's Parent pins the base scalar type for the
// containing DepScalar.
type DepBound struct {
	Inclusive bool  // true → GTE/LTE, false → GT/LT
	Value     Value // the concrete bound being compared against
}

// DepScalarInfo is the payload carried by a Value of a Type/Dependent/
// Dep<X> type, where <X> is the leaf name of the base scalar type.
//
// A DepScalar carries up to two bounds:
//
//   - Lo (lower bound) — values must satisfy `v > Lo.Value` (strict)
//     or `v >= Lo.Value` (inclusive).
//   - Hi (upper bound) — values must satisfy `v < Hi.Value` (strict)
//     or `v <= Hi.Value` (inclusive).
//
// Either side may be nil meaning "unbounded on this side". The two
// bounds are AND-combined; intervals like `[10, 20]` set both Lo and
// Hi. Same-side combinations (e.g. `gte 10 tand gte 5`) tighten to
// the more restrictive single bound during construction or
// unification — the dual-bound shape is reserved for genuine
// intervals where Lo and Hi are on opposite sides of the lattice.
//
// Empty intervals (Lo > Hi, or equal with strict on either side)
// reduce to Never *before* a DepScalarInfo with that shape can be
// constructed, so a populated info always represents a non-empty
// constraint.
type DepScalarInfo struct {
	Lo *DepBound // nil = no lower bound
	Hi *DepBound // nil = no upper bound
}

// kindToBound translates a single DepKind into a (DepBound, isLower)
// pair. Returns (nil, false) for kinds that don't decompose into a
// single bound (zero, multi-bit, etc.). Used by NewDepScalar and
// combineDepScalars to feed the kind-tagged constructor APIs into
// the Lo/Hi storage.
func kindToBound(kind DepKind, value Value) (*DepBound, bool, bool) {
	switch kind {
	case DepGT:
		return &DepBound{Inclusive: false, Value: value}, true, true
	case DepGTE:
		return &DepBound{Inclusive: true, Value: value}, true, true
	case DepLT:
		return &DepBound{Inclusive: false, Value: value}, false, true
	case DepLTE:
		return &DepBound{Inclusive: true, Value: value}, false, true
	}
	return nil, false, false
}

// BoundToKind translates a (bound, isLower) pair back into the
// equivalent DepKind. Used by formatDepScalar so the surface form
// stays stable across the redesign (`(Integer gte 10 lte 20)`).
func BoundToKind(b *DepBound, lower bool) DepKind {
	if b == nil {
		return 0
	}
	if lower {
		if b.Inclusive {
			return DepGTE
		}
		return DepGT
	}
	if b.Inclusive {
		return DepLTE
	}
	return DepLT
}

// NewDepScalar builds a DepScalar Value from a comparison kind and a
// concrete bound. The bound's Parent determines the base type of the
// dependent constraint and selects the leaf name in the resulting
// Type/Dependent/Dep<Leaf> path. Returns a Value with a None Parent
// (and no payload) if the bound is not a scalar or kind doesn't
// decompose into a single bound — callers are expected to validate
// types before constructing.
func NewDepScalar(kind DepKind, bound Value) Value {
	leaf := dependentLeafFromBoundType(bound.Parent)
	if leaf == "" {
		return Value{Parent: TNone}
	}
	db, lower, ok := kindToBound(kind, bound)
	if !ok {
		return Value{Parent: TNone}
	}
	t, _ := NewType("Type/Dependent/Dep" + leaf)
	info := DepScalarInfo{}
	if lower {
		info.Lo = db
	} else {
		info.Hi = db
	}
	return NewValueRaw(t, info)
}

// dependentLeafFromBoundType returns the leaf name to use in a
// Type/Dependent/Dep<Leaf> path for a given bound type. The leaf is
// the last *named* part of the bound's lattice path, looked up
// against the well-known scalar bases. Returns "" for unsupported
// bound types.
func dependentLeafFromBoundType(t *Type) string {
	// Walk the ancestry from the most specific to root, returning the
	// first match against a well-known scalar base. Value-tagged
	// subtypes (e.g. Number/Integer/42) strip down to their last named
	// ancestor.
	for d := t; d != nil; d = d.Parent {
		switch d {
		case TInteger:
			return "Integer"
		case TDecimal:
			return "Decimal"
		case TNumber:
			return "Number"
		case TString:
			return "String"
		case TBoolean:
			return "Boolean"
		case TAtom:
			return "Atom"
		}
	}
	return ""
}

// DependentLeafBaseType returns the scalar base type for a given
// dependent leaf name, or (TNone, false) if the leaf is unknown.
//
// Post Step 9 of TYPE-DECOUPLING.0.md the lookup is no longer a
// hardcoded leaf-name switch — every dependent type's *Type carries
// its BaseType pointer directly, set at registration via
// builtinDecl.BasePath. This function walks the Builtin table to
// find the named DepXxx type and returns its BaseType. Callers
// holding a *Type should prefer `t.BaseType` for the same answer in
// one field access.
func DependentLeafBaseType(leaf string) (*Type, bool) {
	if leaf == "" {
		return TNone, false
	}
	depPath := "Type/Dependent/Dep" + leaf
	if dep := Builtin.bypath[depPath]; dep != nil && dep.BaseType != nil {
		return dep.BaseType, true
	}
	return TNone, false
}

// DependentLeafFromType extracts the leaf name from a Type/Dependent/
// Dep<Leaf> path, or "" if the type is not a dependent scalar path.
// Accepts trailing path components (forward-compat) so a future
// value-tagged DepInteger subtype keeps reporting "Integer".
func DependentLeafFromType(t *Type) string {
	// Walk up until the parent is Type/Dependent. Accepts deeper paths
	// (forward-compat) per the doc comment.
	for d := t; d != nil && d.Parent != nil; d = d.Parent {
		if d.Parent == TDependent {
			if !strings.HasPrefix(d.Name, "Dep") {
				return ""
			}
			return strings.TrimPrefix(d.Name, "Dep")
		}
	}
	return ""
}

// IsDepScalar reports whether the value is any dependent scalar type.
func (v Value) IsDepScalar() bool {
	return DependentLeafFromType(v.Parent) != ""
}

// AsDepScalar extracts the DepScalarInfo payload.
func (v Value) AsDepScalar() (DepScalarInfo, error) {
	if di, ok := v.Data.(DepScalarInfo); ok {
		return di, nil
	}
	return DepScalarInfo{}, fmt.Errorf("AsDepScalar: not a DepScalar value (got %T)", v.Data)
}

// depScalarCheck returns true if `value` satisfies every populated
// bound in info. Both Lo and Hi are AND-combined.
func depScalarCheck(info DepScalarInfo, value Value) bool {
	if info.Lo == nil && info.Hi == nil {
		return false
	}
	if info.Lo != nil && !depBoundCheck(info.Lo, true, value) {
		return false
	}
	if info.Hi != nil && !depBoundCheck(info.Hi, false, value) {
		return false
	}
	return true
}

// depBoundCheck applies a single-side bound to value. lower=true
// requires value > bound (or ≥ if Inclusive); lower=false requires
// value < bound (or ≤). Returns false on any CompareValues error so
// cross-type comparisons (e.g. Integer DepScalar vs String value)
// reject cleanly.
func depBoundCheck(b *DepBound, lower bool, value Value) bool {
	cmp, err := CompareValues(value, b.Value)
	if err != nil {
		return false
	}
	if lower {
		if b.Inclusive {
			return cmp >= 0
		}
		return cmp > 0
	}
	if b.Inclusive {
		return cmp <= 0
	}
	return cmp < 0
}

// boundsEqual reports whether two DepBound pointers represent the
// same constraint. Both nil → equal; one nil → not equal; otherwise
// Inclusive flag and structurally-equal Value.
func boundsEqual(a, b *DepBound) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Inclusive != b.Inclusive {
		return false
	}
	if !a.Value.Parent.Equal(b.Value.Parent) {
		return false
	}
	return ValuesEqual(a.Value, b.Value)
}

// depScalarsEqual reports whether two DepScalar payloads describe the
// same constraint: same Lo bound and same Hi bound (treating nil as
// "no bound on this side").
func depScalarsEqual(a, b DepScalarInfo) bool {
	return boundsEqual(a.Lo, b.Lo) && boundsEqual(a.Hi, b.Hi)
}

// formatDepScalar renders a DepScalar's display form. Single-bound is
// "(Leaf op bound)"; interval is "(Leaf op1 bound1 op2 bound2)" with
// the lower bound rendered first for stability. Each side is
// translated back through BoundToKind so the surface form matches
// the gt/gte/lt/lte vocabulary the user wrote.
func formatDepScalar(leaf string, info DepScalarInfo) string {
	switch {
	case info.Lo != nil && info.Hi != nil:
		return fmt.Sprintf("(%s %s %s %s %s)", leaf,
			BoundToKind(info.Lo, true), info.Lo.Value.String(),
			BoundToKind(info.Hi, false), info.Hi.Value.String())
	case info.Lo != nil:
		return fmt.Sprintf("(%s %s %s)", leaf,
			BoundToKind(info.Lo, true), info.Lo.Value.String())
	case info.Hi != nil:
		return fmt.Sprintf("(%s %s %s)", leaf,
			BoundToKind(info.Hi, false), info.Hi.Value.String())
	default:
		return fmt.Sprintf("(%s)", leaf)
	}
}

// renderDepScalar is the canonical Value-shaped wrapper around
// formatDepScalar. Every display surface in the engine — Value.String,
// ValToString, FormatValueJSON, FormatForPrint, aql_error stack
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
	return formatDepScalar(DependentLeafFromType(v.Parent), info)
}

// tightenSameSide combines two same-side bounds (both lower, or both
// upper) into the tighter single bound. The caller has verified
// lower matches both inputs. Errors on the underlying compare
// propagate via ok=false (treated as Never).
func tightenSameSide(a, b *DepBound, lower bool) (*DepBound, bool) {
	cmp, err := CompareValues(a.Value, b.Value)
	if err != nil {
		return nil, false
	}
	// For lower bounds, the larger value is tighter; for upper bounds,
	// the smaller. When the bounds are equal, prefer the strict form
	// (GT over GTE, LT over LTE) — it's the narrower constraint.
	if cmp == 0 {
		strict := !a.Inclusive || !b.Inclusive
		return &DepBound{Inclusive: !strict, Value: a.Value}, true
	}
	if (lower && cmp > 0) || (!lower && cmp < 0) {
		return a, true
	}
	return b, true
}

// combineDepScalars computes the intersection of two DepScalar
// constraints over the same base type. Returns ok=false when the
// result is empty (no value satisfies both).
//
//	(Integer gt 5) tand (Integer lt 10) → interval (5, 10)
//	(Integer gte 10) tand (Integer gte 5) → gte 10 (tighter)
//	(Integer gt 10) tand (Integer lt 5) → empty (Never)
func combineDepScalars(a, b DepScalarInfo) (DepScalarInfo, bool) {
	out := DepScalarInfo{}
	// Tighten lower bounds first.
	switch {
	case a.Lo == nil:
		out.Lo = b.Lo
	case b.Lo == nil:
		out.Lo = a.Lo
	default:
		t, ok := tightenSameSide(a.Lo, b.Lo, true)
		if !ok {
			return DepScalarInfo{}, false
		}
		out.Lo = t
	}
	// Then upper bounds.
	switch {
	case a.Hi == nil:
		out.Hi = b.Hi
	case b.Hi == nil:
		out.Hi = a.Hi
	default:
		t, ok := tightenSameSide(a.Hi, b.Hi, false)
		if !ok {
			return DepScalarInfo{}, false
		}
		out.Hi = t
	}
	// Verify the resulting interval is non-empty.
	if out.Lo != nil && out.Hi != nil {
		cmp, err := CompareValues(out.Lo.Value, out.Hi.Value)
		if err != nil {
			return DepScalarInfo{}, false
		}
		if cmp > 0 {
			return DepScalarInfo{}, false
		}
		// Equal bounds: empty unless both sides are inclusive.
		if cmp == 0 && (!out.Lo.Inclusive || !out.Hi.Inclusive) {
			return DepScalarInfo{}, false
		}
	}
	return out, true
}

// MakeDepScalarSig builds the [TScalar, TScalarType] -> [TDependent]
// signature variant for a comparison op. `Integer gte 10`, `String lt
// "z"`, `Decimal gte 1.5` all hit this sig: arg0 is the bound, arg1 is
// the base-type literal. The result type path is Type/Dependent/Dep<X>
// where <X> is the leaf of the base type. This sig sorts ahead of the
// [Any, Any] boolean sig (because its types are more specific), so
// concrete `5 gte 10` still hits the boolean branch via the second
// match attempt.
//
// Used by ComparisonNatives to wire the same single-bound DepScalar
// constructor onto each of `lt`, `gt`, `lte`, `gte`.
//
// RunInCheckMode=true so `type G10 (Integer gt 10)` produces a real
// DepScalar value under static analysis — without it the check-mode
// pipeline would push a TDependent carrier (Data=nil) and downstream
// `def x:G10 …` would have no per-leaf shape to reason about. The
// handler is a pure constructor with no registry side effects, so
// running it during check is safe.
func MakeDepScalarSig(opName string, kind DepKind) NativeSig {
	return NativeSig{
		Args: []*Type{TScalar, TScalarType},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			// arg1 is the type-literal at the deep position. Reject
			// non-leaf bases — only the well-known scalar types map
			// to a Dependent leaf name.
			if IsConcrete(args[1]) {
				return nil, fmt.Errorf("%s: dependent constructor needs a scalar type literal, got concrete %s",
					opName, args[1].Parent.String())
			}
			leaf := dependentLeafFromBoundType(args[1].Parent)
			if leaf == "" {
				return nil, fmt.Errorf("%s: dependent constructor does not support base type %s",
					opName, args[1].Parent.String())
			}
			// Bound must be the same scalar base as the type literal.
			base, _ := DependentLeafBaseType(leaf)
			if !args[0].Parent.Matches(base) {
				return nil, fmt.Errorf("%s: bound %s does not match dependent base %s",
					opName, args[0].Parent.String(), base.String())
			}
			return []Value{NewDepScalar(kind, args[0])}, nil
		},
		Returns:        []*Type{TDependent},
		RunInCheckMode: true,
	}
}

// The `between` word registration is defined in
// lang/go/engine/native_compare.go alongside the other DepScalar
// constructors (lt / gt / lte / gte). BetweenHandler below is the
// exported algorithm primitive.

func BetweenHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if IsConcrete(args[2]) {
		return nil, fmt.Errorf("between: type arg must be a scalar type literal, got concrete %s",
			args[2].Parent.String())
	}
	leaf := dependentLeafFromBoundType(args[2].Parent)
	if leaf == "" {
		return nil, fmt.Errorf("between: unsupported base type %s",
			args[2].Parent.String())
	}
	base, _ := DependentLeafBaseType(leaf)
	if !args[0].Parent.Matches(base) {
		return nil, fmt.Errorf("between: low bound %s does not match base %s",
			args[0].Parent.String(), base.String())
	}
	if !args[1].Parent.Matches(base) {
		return nil, fmt.Errorf("between: high bound %s does not match base %s",
			args[1].Parent.String(), base.String())
	}
	cmp, err := CompareValues(args[0], args[1])
	if err != nil {
		return nil, fmt.Errorf("between: %w", err)
	}
	if cmp > 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	info := DepScalarInfo{
		Lo: &DepBound{Inclusive: true, Value: args[0]},
		Hi: &DepBound{Inclusive: true, Value: args[1]},
	}
	t, err := NewType("Type/Dependent/Dep" + leaf)
	if err != nil {
		return nil, fmt.Errorf("between: %w", err)
	}
	return []Value{NewValueRaw(t, info)}, nil
}
