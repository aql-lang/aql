// Bare-name type resolution — the canonical cascade of ADR-012 rule 4,
// mirroring eng/go/resolve.go. A capitalised name arriving as a Word
// resolves in exactly two arms, in order: (1) the registry's binding
// store (user types / dynamic bindings), then (2) the live builtin
// name table. Sites with a Registry run both arms; registry-free sites
// run arm 2 only. Do NOT re-implement either arm inline — that is how
// the Go per-site cascades drifted apart (NUR059).

import { canonValue } from './canon.ts'
import type { Registry } from './registry.ts'
import { TDisjunct, TList, TMap, typeNameTable } from './type.ts'
import {
  ChildType,
  OrderedMap,
  Value,
  newAtom,
  newBoolean,
  newList,
  newMap,
  newNone,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
} from './value.ts'

/**
 * Resolve a scalar Word to its semantic value: true/false/none
 * keywords, a bound def, a builtin type literal, else an Atom of the
 * same name. Mirrors Go resolveWordValue.
 */
export function resolveWordValue(v: Value, registry?: Registry): Value {
  if (!v.isWord()) return v
  const name = v.asWord().name
  if (name === 'true') return newBoolean(true)
  if (name === 'false') return newBoolean(false)
  if (name === 'none') return newNone()
  if (registry) {
    const top = registry.topOfDefStack(name)
    if (top !== undefined) return top
  }
  const t = typeNameTable().get(name)
  if (t !== undefined) return newTypeLiteral(t)
  return newAtom(name)
}

/**
 * Recursively resolve word values to their semantic form: lists
 * element-wise, map values, typed-container children, disjunct
 * alternatives. Mirrors Go ResolveWordsDeep / ResolveWordsDeepR.
 */
export function resolveWordsDeep(v: Value, registry?: Registry): Value {
  if (v.isWord()) return resolveWordValue(v, registry)
  if (v.data instanceof ChildType) {
    const ct = v.data
    const child = resolveWordsDeep(ct.child, registry)
    if (v.vType.equal(TMap)) {
      return newTypedMap(
        child,
        ct.entries.map((e) => ({ key: e.key, value: resolveWordsDeep(e.value, registry) })),
      )
    }
    return newTypedList(
      child,
      ct.elements.map((e) => resolveWordsDeep(e, registry)),
    )
  }
  if (v.vType.equal(TList) && Array.isArray(v.data)) {
    const resolved = (v.data as Value[]).map((e) => resolveWordsDeep(e, registry))
    return newList(resolved, { eval: v.eval, quoted: v.quoted })
  }
  if (v.vType.equal(TMap) && v.data instanceof OrderedMap) {
    const m = v.data
    const out = new OrderedMap()
    for (const k of m.keys()) {
      out.set(k, resolveWordsDeep(m.get(k)!, registry))
    }
    return newMap(out)
  }
  if (v.vType.matches(TDisjunct) && v.data !== null && typeof v.data === 'object' && 'alternatives' in (v.data as object)) {
    const d = v.data as { alternatives: Value[] }
    const alts = d.alternatives.map((a) => resolveWordsDeep(a, registry))
    return new Value(v.vType, { alternatives: simplifyDisjunctAlts(alts) })
  }
  return v
}

/**
 * Flatten nested disjuncts and deduplicate alternatives after
 * resolution — the Go SimplifyDisjunctAlts mirror. An alternative that
 * RESOLVED to a named disjunct (`{x?:Maybe}` where Maybe is
 * `Integer tor String`) must contribute its members, not ride as one
 * opaque value that membership then compares by equality (rejecting
 * every valid member). Order is insertion order; canonical re-ordering
 * stays with the `tor` builder, which is where rendered unions form.
 */
function simplifyDisjunctAlts(alts: Value[]): Value[] {
  const flat: Value[] = []
  const push = (a: Value): void => {
    if (a.vType.matches(TDisjunct) && a.data !== null && typeof a.data === 'object' && 'alternatives' in (a.data as object)) {
      for (const inner of (a.data as { alternatives: Value[] }).alternatives) push(inner)
      return
    }
    flat.push(a)
  }
  for (const a of alts) push(a)
  const seen = new Set<string>()
  const out: Value[] = []
  for (const a of flat) {
    const k = canonValue(a)
    if (seen.has(k)) continue
    seen.add(k)
    out.push(a)
  }
  return out
}
