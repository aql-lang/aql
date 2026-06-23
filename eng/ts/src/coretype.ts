// Type-introspection words ported from eng/go/core_type.go:
// typeof, pathof, and the IsValueOfType conformance check behind `is`.
//
// PARITY NOTE: this is the subset the eng/spec corpus reaches through
// the current (no-parser) TS runner — bare type literals and concrete
// scalars/lists. The typed-list / typed-map / record-shape branches of
// IsValueOfType (which need ChildType payloads) and the Disjunct/Enum
// membership branches are added by their owning port increments.
import { AqlType, TAny, TType } from './type.ts'
import { newList, newTypeLiteral, Value } from './value.ts'

// Degenerate roots saturate under typeof (they are their own parent).
const SATURATING_ROOTS = new Set(['Any', 'None', 'Never', 'Absent'])

/**
 * parentType returns the lattice parent of a type, mirroring the
 * Go Type.Parent chain:
 *   - a multi-part path drops its last segment (Scalar/Number/Integer →
 *     Scalar/Number);
 *   - a branch root (Scalar/Node/Ideal/Word/Type) has parent Any;
 *   - a degenerate root (Any/None/Never/Absent) has no parent (null).
 */
export function parentType(t: AqlType): AqlType | null {
  if (t.parts.length > 1) {
    return new AqlType(t.parts.slice(0, -1))
  }
  if (SATURATING_ROOTS.has(t.parts[0]!)) return null
  return TAny
}

/**
 * TypeOf returns the type of v as a type-literal Value — a single
 * parent hop up the unified lattice. A concrete value yields its own
 * VType (`typeof 5 → Integer`); a type literal yields the lattice
 * parent of its VType (`typeof Integer → Number`), with the degenerate
 * roots saturating (`typeof None → None`). Mirrors core_type.go::TypeOf
 * where, because Type = Value, a type literal's Parent IS its lattice
 * parent.
 */
export function typeOf(v: Value): Value {
  if (v.data === null) {
    const p = parentType(v.vType)
    return newTypeLiteral(p ?? v.vType)
  }
  return newTypeLiteral(v.vType)
}

/**
 * PathOf returns a type's ancestry as a List of type literals, root
 * first, leaf last: `pathof Integer → [Scalar Number Integer]`,
 * `pathof Type → [Type]`, `pathof None → [None]`. Mirrors
 * core_type.go::PathOf (Any is never an explicit ancestor segment
 * because the lattice paths here are already rooted at the branch
 * root, not at Any).
 */
export function pathOf(t: Value): Value {
  const parts = t.vType.parts
  const elems: Value[] = []
  for (let i = 0; i < parts.length; i++) {
    elems.push(newTypeLiteral(new AqlType(parts.slice(0, i + 1))))
  }
  return newList(elems, { eval: false })
}

/** A bare type node: a type literal with no concrete payload (incl. None/Any/Never). */
function isBareTypeNode(v: Value): boolean {
  return v.data === null && !v.carrier
}

/**
 * IsValueOfType reports whether v satisfies type t. Subset of
 * core_type.go::IsValueOfType:
 *   - t is the bare metatype Type: v is itself a type;
 *   - t is any other bare type literal: lattice subtyping
 *     (v.VType conforms to t.VType);
 *   - otherwise: structural identity on the carried types (best effort).
 */
export function isValueOfType(v: Value, t: Value): boolean {
  if (isBareTypeNode(t)) {
    if (t.vType.equal(TType)) {
      if (v.carrier) return false
      return isBareTypeNode(v) || v.vType.matches(TType)
    }
    return v.vType.matches(t.vType)
  }
  // Concrete RHS (structural type bodies) — handled by later increments;
  // fall back to type identity so simple cases still answer.
  return v.vType.matches(t.vType)
}
