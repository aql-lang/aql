// Type-introspection words ported from eng/go/core_type.go:
// typeof, pathof, and the IsValueOfType conformance check behind `is`.
//
// PARITY NOTE: this is the subset the eng/spec corpus reaches through
// the current (no-parser) TS runner — bare type literals and concrete
// scalars/lists. The typed-list / typed-map / record-shape branches of
// IsValueOfType (which need ChildType payloads) and the Disjunct/Enum
// membership branches are added by their owning port increments.
import {
  BoruType,
  TAny,
  TBoolean,
  TInteger,
  TList,
  TMap,
  TNone,
  TNumber,
  TString,
  TType,
} from './type.ts'
import { newList, newTypeLiteral, OrderedMap, Value } from './value.ts'

/**
 * coerceBoolean reduces any value to a truthiness, mirroring
 * eng/go/core_helpers.go::CoerceBoolean: none/empty-string/empty-
 * container/zero are falsy; everything else is truthy.
 */
export function coerceBoolean(v: Value): boolean {
  if (v.isNone()) return false
  // A bare type literal (or other non-concrete value) is falsy.
  if (v.data === null) return false
  const t = v.vType
  if (t.matches(TBoolean)) return v.asBoolean()
  if (t.matches(TInteger)) return v.asInteger() !== 0n
  if (t.matches(TNumber)) return v.asFloat() !== 0
  if (t.equal(TList)) {
    if (Array.isArray(v.data)) return v.data.length > 0
    if (v.isTypedList()) return v.asChildType().elements.length > 0
    return false
  }
  if (t.equal(TMap)) {
    if (v.data instanceof OrderedMap) return v.data.size > 0
    return false
  }
  if (t.matches(TString)) return v.asString().length > 0
  return true
}

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
export function parentType(t: BoruType): BoruType | null {
  if (t.parts.length > 1) {
    return new BoruType(t.parts.slice(0, -1))
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
  // The none value's type is the None type literal.
  if (v.isNone()) return newTypeLiteral(TNone)
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
    elems.push(newTypeLiteral(new BoruType(parts.slice(0, i + 1))))
  }
  return newList(elems, { eval: false })
}

/** A bare type node: a type literal with no concrete payload (incl. None/Any/Never). */
function isBareTypeNode(v: Value): boolean {
  return v.data === null && !v.carrier
}

/**
 * isRecordShape: a non-empty concrete map whose every value is a type
 * body (type literal or nested record shape). Mirrors
 * core_type.go::IsRecordShape.
 */
export function isRecordShape(v: Value): boolean {
  if (!v.isMap()) return false
  const m = v.asMap()
  if (m.size === 0) return false
  return m.keys().every((k) => {
    const fv = m.get(k)!
    return fv.data === null || isRecordShape(fv)
  })
}

/** isTypeBody: a structural type value (typed container, disjunct, record shape, fn). */
export function isTypeBody(v: Value): boolean {
  if (v.isTypedList() || v.isTypedMap() || v.isDisjunct()) return true
  if (v.data !== null && v.vType.matches(TType)) return true
  return isRecordShape(v)
}

/**
 * IsValueOfType reports whether v satisfies type t. Subset of
 * core_type.go::IsValueOfType:
 *   - t is the bare metatype Type: v is itself a type;
 *   - t is any other bare type literal: lattice subtyping
 *     (v.VType conforms to t.VType);
 *   - otherwise: structural identity on the carried types (best effort).
 */
/** Structural value equality (scalars, atoms, type literals, lists, maps). */
function valuesEqual(a: Value, b: Value): boolean {
  // Bare type literals compare by their lattice node.
  if (a.data === null || b.data === null) {
    return a.data === null && b.data === null && a.vType.equal(b.vType)
  }
  // Lists (plain or typed) compare element-wise.
  const ae = listElements(a)
  const be = listElements(b)
  if (ae !== null && be !== null) {
    return ae.length === be.length && ae.every((x, i) => valuesEqual(x, be[i]!))
  }
  if (ae !== null || be !== null) return false
  // Maps compare by sorted keys then values.
  if (a.isMap() && b.isMap()) {
    const am = a.asMap()
    const bm = b.asMap()
    const ak = am.sortedKeys()
    const bk = bm.sortedKeys()
    return ak.length === bk.length && ak.every((k, i) => k === bk[i] && valuesEqual(am.get(k)!, bm.get(k)!))
  }
  // Scalars / atoms: compatible leaves and equal payload.
  if (!(a.vType.matches(b.vType) || b.vType.matches(a.vType))) return false
  if (typeof a.data === 'bigint' && typeof b.data === 'bigint') return a.data === b.data
  return a.data === b.data
}

/** Extract a list's elements whether it is a plain or a typed list. */
function listElements(v: Value): Value[] | null {
  if (Array.isArray(v.data)) return v.data as Value[]
  if (v.isTypedList()) return v.asChildType().elements
  return null
}

export function isValueOfType(v: Value, t: Value): boolean {
  // Typed list `[:T]`: v must be a CONCRETE list whose every element
  // satisfies T. A plain list (even empty) is concrete; a typed list is
  // concrete only when it carries retained elements — a bare typed-list
  // carrier (`[:Integer]`, no elements) is a type, not a value, so it
  // does not conform. Mirrors the Go IsConcrete guard.
  if (t.isTypedList()) {
    let elems: Value[] | null = null
    if (Array.isArray(v.data)) elems = v.data as Value[]
    else if (v.isTypedList() && v.asChildType().elements.length > 0) elems = v.asChildType().elements
    if (elems === null || !v.vType.equal(t.vType)) return false
    const child = t.asChildType().child
    return elems.every((e) => isValueOfType(e, child))
  }
  // Typed map `{:T}`: v must be a concrete map whose every value
  // satisfies T.
  if (t.isTypedMap()) {
    if (!v.isMap()) return false
    const child = t.asChildType().child
    return v.asMap().keys().every((k) => isValueOfType(v.asMap().get(k)!, child))
  }
  // Positional list type `[A B …]` (a concrete list of type/value
  // elements): v must be a same-length list whose elements satisfy the
  // corresponding template element.
  if (Array.isArray(t.data)) {
    const ve = listElements(v)
    if (ve === null) return false
    const te = t.data as Value[]
    return ve.length === te.length && te.every((tt, i) => isValueOfType(ve[i]!, tt))
  }
  // Record-shape / structural map type: every key declared in t must
  // be present in v with a conforming value (extra v keys are allowed).
  if (t.data instanceof OrderedMap) {
    if (!(v.data instanceof OrderedMap)) return false
    const tm = t.asMap()
    const vm = v.asMap()
    return tm.keys().every((k) => vm.has(k) && isValueOfType(vm.get(k)!, tm.get(k)!))
  }
  // Disjunct / Enum membership: v satisfies the union if it satisfies
  // any alternative (a type-literal alternative is a subtype check; a
  // concrete alternative — an enum member — is value equality).
  if (t.isDisjunct()) {
    for (const alt of t.asDisjunct().alternatives) {
      if (alt.data === null) {
        if (isValueOfType(v, alt)) return true
      } else if (valuesEqual(v, alt)) {
        return true
      }
    }
    return false
  }
  if (isBareTypeNode(t)) {
    if (t.vType.equal(TType)) {
      if (v.carrier) return false
      return isBareTypeNode(v) || isTypeBody(v)
    }
    return v.vType.matches(t.vType)
  }
  // Concrete RHS that is not a type: `is` is value equality
  // (`5 is 6` → false, `5 is 5` → true). Structural type-body RHS
  // (record-shape maps) is handled by a later increment.
  return valuesEqual(v, t)
}
