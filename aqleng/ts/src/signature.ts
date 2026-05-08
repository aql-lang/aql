// Signature describes one calling convention of a registered word.
// Mirrors aqleng/go/signature.go but trimmed to the spec subset:
// no checker mode, no Patterns, no FullStack, no Returns lists for
// type-check propagation.

import type { AqlType } from './type.ts'
import type { Value } from './value.ts'

/** A handler receives matched args and the registry, returns the values to push. */
export type Handler = (
  args: Value[],
  ctx: Map<string, Value> | null,
  stack: Value[],
  registry: Registry,
) => Value[] | Promise<Value[]>

export interface Signature {
  /** Argument types, deepest-first (Args[0] = deepest stack / first forward). */
  args: AqlType[]
  handler: Handler
  /**
   * Optional value-patterns indexed by arg position. A concrete-scalar
   * pattern fires the §1.1 literal-dispatch path: the matched arg
   * must have the same Data as the pattern. See match.ts for the
   * forward-vs-stack rules.
   */
  patterns?: Map<number, Value>
  /**
   * NoEvalArgs marks positions where list auto-evaluation is suppressed.
   * Unused in the spec subset; included for shape parity.
   */
  noEvalArgs?: Set<number>
  /** Fallback marker — true for the generic 0-arg fallback. */
  fallback?: boolean
}

export interface NativeSig {
  args: AqlType[]
  handler: Handler
  patterns?: Map<number, Value>
  noEvalArgs?: Set<number>
  fallback?: boolean
}

export interface NativeFunc {
  name: string
  forwardPrecedence?: boolean
  signatures: NativeSig[]
}

/**
 * Score a signature by sum of part-counts across its arg types
 * (argument specificity). Higher score = more specific. This mirrors
 * the Go engine's heuristic: a signature `[Integer, Integer]` scores
 * higher than `[Any, Any]`.
 */
export function signatureScore(sig: Signature): number {
  let s = 0
  for (const t of sig.args) s += t.specificity()
  // Concrete-value patterns make the sig more specific than one
  // with the same arg types but no pattern (parity with Go's
  // post-§1.1 score boost).
  if (sig.patterns) {
    for (const v of sig.patterns.values()) {
      if (v.data !== null) s += 10
    }
  }
  return s
}

/**
 * Sort signatures by descending specificity. The first matching sig
 * wins, so more-specific overloads must be tried first.
 */
export function sortSignatures(sigs: Signature[]): void {
  sigs.sort((a, b) => signatureScore(b) - signatureScore(a))
}

// Forward-declared type — the actual class lives in registry.ts.
import type { Registry } from './registry.ts'
