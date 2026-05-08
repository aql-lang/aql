// Capabilities are the integration seam between aqleng (the engine)
// and a host package that supplies real-world services like fileops,
// format encoders, or a SQL store.
//
// PARITY NOTE: in Go this lives on `Registry` as two methods plus a
// generic free function `Cap[T]`. TS uses `unknown` and the caller
// narrows with `instanceof` / a custom predicate — there is no
// runtime type assertion that's both type-safe and zero-overhead.
// The `cap<T>()` helper keeps the call site terse but the cast is
// unchecked: it trusts the host to install a value of the expected
// shape under the agreed-upon name.

import type { Registry } from './registry.js'

/**
 * Typed convenience accessor. Mirrors `aqleng.Cap[T]` from Go.
 * Returns the stored value cast to T (UNCHECKED CAST — see file
 * header) along with `true`, or `undefined` and `false` when the
 * capability is missing.
 */
export function cap<T>(r: Registry, name: string): [T, true] | [undefined, false] {
  const v = r.capability(name)
  if (v === undefined) return [undefined, false]
  return [v as T, true]
}
